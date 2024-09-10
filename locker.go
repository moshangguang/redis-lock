package redislock

import (
	"context"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
)

type Locker interface {
	Lock(ctx context.Context, timeout ...time.Duration) (err error) //如果传timeout，则在指定timeout时间内如果没抢到锁则返回抢锁失败，否则使用默认timeout
	TryLock(ctx context.Context) (bool, error)                      //尝试抢锁，无阻塞
	Unlock(ctx context.Context) error                               //释放锁，由外部处理错误
}
type RLocker struct {
	key         string
	owner       string
	timeout     time.Duration
	done        chan struct{}
	state       *uint32
	redisClient *redis.Client
	errHandler  func(option ErrorOption)
}

func (locker *RLocker) Lock(ctx context.Context, timeout ...time.Duration) (err error) {
	_, ok := ctx.Deadline()
	if ok && len(timeout) == 0 { //如果ctx有过期时间，且没有指定timeout，则使用ctx的过期时间
		return locker.lock(ctx, math.MaxInt)
	}
	//如果
	t := locker.timeout
	if len(timeout) != 0 && timeout[0] > minTimeout {
		t = timeout[0]
	}
	//如果ctx有过期时间，或有指定timeout，则再次创建带timeout的ctx
	ctx, cancelFunc := context.WithTimeout(ctx, t)
	defer cancelFunc()
	return locker.lock(ctx, math.MaxInt)
}

func (locker *RLocker) TryLock(ctx context.Context) (bool, error) {
	err := locker.lock(ctx, 1)
	return err == nil, err
}

func (locker *RLocker) Unlock(ctx context.Context) error {
	if atomic.LoadUint32(locker.state) != stateLockSuccess {
		return LockerNonLock
	}
	if !atomic.CompareAndSwapUint32(locker.state, stateLockSuccess, stateLockNon) {
		return LockerNonLock
	}
	locker.done <- struct{}{}
	eval := locker.redisClient.Eval(ctx, deleteScript, []string{locker.key}, locker.owner)
	res, err := eval.Int()
	if err != nil {
		return err
	}
	if res > 0 {
		return nil
	}
	return UnlockFail
}

func (locker *RLocker) lock(ctx context.Context, tries int) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	isAcquire := false
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for i := 0; i < tries; i++ {
		if i != 0 {
			if timer == nil {
				timer = time.NewTimer(getDelayDuration())
			}
			select {
			case <-ctx.Done():
				return LockerTimeout
			case <-timer.C:
				timer.Reset(getDelayDuration())
			}
		}
		isAcquire, err = locker.redisClient.SetNX(ctx, locker.key, locker.owner, maxKeepalive).Result()
		if err != nil {
			return err
		}
		if isAcquire {
			break
		}
	}
	if !isAcquire {
		return LockerTimeout
	}
	go func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			locker.errHandler(ErrorOption{
				Title: "【Redis分布式锁】锁续时出现异常",
				Panic: r,
				Key:   locker.key,
			})
		}()
		ticker := time.NewTicker(minKeepalive)
		defer ticker.Stop()
	Loop:
		for {
			select {
			case <-ticker.C:
				if err := locker.redisClient.Expire(ctx, locker.key, maxKeepalive).Err(); err != nil {
					locker.errHandler(ErrorOption{
						Title: "【Redis分布式锁】锁续时出错",
						Error: err,
						Key:   locker.key,
					})
				}
			case <-locker.done:
				break Loop
			}
		}
	}()
	atomic.StoreUint32(locker.state, stateLockSuccess)
	return
}

func getDelayDuration() time.Duration {
	return time.Duration(rand.Intn(maxRetryDelayMilliSec-minRetryDelayMilliSec)+minRetryDelayMilliSec) * time.Millisecond
}
