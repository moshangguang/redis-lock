package redislock

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/go-redis/redis/v8"
)

type Locker interface {
	Lock(ctx context.Context, timeout ...time.Duration) error //如果传timeout，则在指定timeout时间内如果没抢到锁则返回抢锁失败，否则使用默认timeout
	TryLock(ctx context.Context) (bool, error)                //尝试抢锁，无阻塞
	Unlock(ctx context.Context) error                         //释放锁，由外部处理错误
	Expire(expire time.Duration) Locker
}
type RLocker struct {
	key         string
	value       string
	expire      time.Duration
	timeout     time.Duration
	redisClient *redis.Client
	cancelFunc  context.CancelFunc
	errHandler  func(option ErrorOption)
}

func (l *RLocker) Expire(expire time.Duration) Locker {
	l.expire = expire
	return l
}

var _ Locker = new(RLocker)

func (l *RLocker) Lock(ctx context.Context, timeout ...time.Duration) error {
	t := l.timeout
	if len(timeout) > 0 {
		t = timeout[0]
	}
	acquire, err := l.acquire(ctx, t, math.MaxInt)
	if err != nil {
		return err
	}
	if acquire {
		return nil
	}
	return LockerTimeout
}

func (l *RLocker) TryLock(ctx context.Context) (bool, error) {
	return l.acquire(ctx, 0, 1)
}

func (l *RLocker) Unlock(ctx context.Context) error {
	if l.cancelFunc != nil {
		l.cancelFunc()
	}
	res, err := l.redisClient.Eval(ctx, delScript, []string{l.key}, l.value).Int()
	if err != nil {
		return err
	}
	if res > 0 {
		return nil
	}
	return UnlockFail
}

func (l *RLocker) acquire(ctx context.Context, timeout time.Duration, tries int) (bool, error) {
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
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
				return false, nil
			case <-timer.C:
				timer.Reset(getDelayDuration())
			}
		}

		ok, err := l.tryAcquire(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func (l *RLocker) tryAcquire(ctx context.Context) (bool, error) {
	ok, err := l.redisClient.SetNX(ctx, l.key, l.value, l.expire).Result()
	if err != nil || !ok {
		return false, err
	}
	l.startAutoRenew()
	return true, nil
}

func (l *RLocker) startAutoRenew() {
	ctx, cancel := context.WithCancel(context.Background())
	l.cancelFunc = cancel

	go func() {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			l.errHandler(ErrorOption{
				Title: "Redis分布式锁续时抛出异常",
				Panic: v,
				Key:   l.key,
			})
		}()
		ticker := time.NewTicker(l.expire / 2)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := l.redisClient.Expire(ctx, l.key, l.expire).Err(); err != nil {
					l.errHandler(ErrorOption{
						Title: "Redis分布式锁续时出错",
						Error: err,
						Key:   l.key,
					})
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func getDelayDuration() time.Duration {
	return time.Duration(rand.Intn(maxRetryDelayMilliSec-minRetryDelayMilliSec)+minRetryDelayMilliSec) * time.Millisecond
}
