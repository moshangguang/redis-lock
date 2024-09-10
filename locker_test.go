package redislock

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

func GetRedisClient() (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       1,
	})
	ping := client.Ping(context.Background())
	if err := ping.Err(); err != nil {
		return nil, err
	}
	return client, nil
}

// 测试同一个key在多个协程会正常抢锁和释放锁
func TestRLocker_Lock(t *testing.T) {
	client, err := GetRedisClient()
	if err != nil {
		t.Errorf("连接Redis失败,err:%s", err.Error())
		return
	}
	allocator := NewAllocator(
		client,
	)
	key := fmt.Sprintf("%d", time.Now().Unix())
	goCount := rand.Intn(10) + 10
	wg := sync.WaitGroup{}
	wg.Add(goCount)
	for i := 0; i < goCount; i++ {
		go func(i int) {
			defer wg.Done()
			locker := allocator.NewLocker(key)
			if err := locker.Lock(context.Background()); err != nil {
				t.Logf("协程%d出现抢锁出错,err:%s", i, err.Error())
				return
			}
			defer func() {
				if err := locker.Unlock(context.Background()); err != nil {
					t.Errorf("协程%d解锁失败,err:%s", i, err.Error())
				}
			}()
			t.Logf("%s 协程%d抢锁成功", time.Now().Format("2006-01-02 15:04:05.000"), i)
			time.Sleep(time.Duration(rand.Intn(100)+100) * time.Millisecond)
			defer t.Logf("%s 协程%d使用锁完毕", time.Now().Format("2006-01-02 15:04:05.000"), i)

		}(i)
	}
	wg.Wait()
}

// 测试第一个尝试抢锁成功后，第二个尝试抢锁失败
func TestRLocker_TryLock(t *testing.T) {
	client, err := GetRedisClient()
	if err != nil {
		t.Errorf("连接Redis失败,err:%s", err.Error())
		return
	}
	allocator := NewAllocator(
		client)
	key := fmt.Sprintf("%d", time.Now().Unix())
	locker1 := allocator.NewLocker(key)
	ok, err := locker1.TryLock(context.Background())
	if ok {
		t.Logf("主协程抢锁成功")
	} else {
		t.Errorf("主协程抢锁失败")
	}
	defer func() {
		if err := locker1.Unlock(context.Background()); err != nil {
			t.Errorf("主协程,err:%s", err.Error())
		}
	}()
	ch := make(chan struct{}, 1)
	go func() {
		defer func() {
			ch <- struct{}{}
		}()
		locker2 := allocator.NewLocker(key)
		tryLock, _ := locker2.TryLock(context.Background())
		if tryLock {
			t.Errorf("次协程预期抢锁失败，实际抢锁成功")
		} else {
			t.Logf("次协程预期抢锁失败，实际抢锁失败")
		}
	}()
	<-ch
}

// 测试超时
func TestRLocker_Lock2(t *testing.T) {
	client, err := GetRedisClient()
	if err != nil {
		t.Errorf("连接Redis失败,err:%s", err.Error())
		return
	}
	//默认5s超时
	allocator := NewAllocator(
		client,
		WithLockerTimeout(5*time.Second))
	key := fmt.Sprintf("%d", time.Now().Unix())
	locker1 := allocator.NewLocker(key)
	err = locker1.Lock(context.Background())
	if err != nil {
		t.Errorf("主协程抢锁失败,err:%s", err.Error())
		return
	}
	defer func() {
		err := locker1.Unlock(context.Background())
		if err != nil {
			t.Errorf("主协程解锁失败,err:%s", err.Error())
		}
	}()
	ch := make(chan struct{}, 1)
	//这个协程测试ctx的超时时间比指定的timeout长，超过timeout就抢锁失败
	go func() {
		defer func() {
			ch <- struct{}{}
		}()
		locker2 := allocator.NewLocker(key)
		start := time.Now()
		maxTimeout := time.Second * 30
		miniTimeout := time.Second * 2
		ctx, cancelFunc := context.WithTimeout(context.Background(), maxTimeout)
		defer cancelFunc()
		err := locker2.Lock(ctx, miniTimeout)
		if err == LockerTimeout {
			delta := time.Now().Sub(start)
			if delta >= miniTimeout && delta < miniTimeout+time.Second {
				t.Logf("次协程抢锁出现预期抢锁超时,耗时：%fs", delta.Seconds())
			} else {
				t.Errorf("次协程抢锁出现预期抢锁超时,耗时超过预期：%fs", delta.Seconds())
			}
		} else {
			t.Errorf("次协程抢锁没有出现预期抢锁超时")
		}
	}()
	<-ch
	//这个协程测试ctx的超时时间比指定的timeout短，超过ctx的超时就抢锁失败
	go func() {
		defer func() {
			ch <- struct{}{}
		}()
		locker3 := allocator.NewLocker(key)
		start := time.Now()
		maxTimeout := time.Second * 30
		miniTimeout := time.Second * 2
		ctx, cancelFunc := context.WithTimeout(context.Background(), miniTimeout)
		defer cancelFunc()
		err := locker3.Lock(ctx, maxTimeout)
		if err == LockerTimeout {
			delta := time.Now().Sub(start)
			if delta >= miniTimeout && delta < miniTimeout+time.Second {
				t.Logf("次协程抢锁出现预期抢锁超时,耗时：%fs", delta.Seconds())
			} else {
				t.Errorf("次协程抢锁出现预期抢锁超时,耗时超过预期：%fs", delta.Seconds())
			}
		} else {
			t.Errorf("次协程抢锁没有出现预期抢锁超时")
		}
	}()
	<-ch
}
