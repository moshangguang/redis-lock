package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"

	redislock "github.com/moshangguang/redis-lock"
)

func main() {
	//<1>创建Redis客户端
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       1,
	})

	allocator := redislock.NewAllocator(
		redisClient, //<3>指定Redis客户端，必传
		redislock.WithLockerTimeout(5*time.Second), //<4>指定默认超时时间为5s，不指定默认为10s
	)
	key := fmt.Sprintf("%d", time.Now().Unix()) //以时间戳作为key来抢锁
	wg := sync.WaitGroup{}
	wg.Add(4)
	go func() { //<6>
		defer wg.Done()
		locker := allocator.NewLocker(key)
		if err := locker.Lock(context.Background()); err != nil { //使用默认超时时间5s
			//处理错误...
			return
		}
		defer func() {
			if err := locker.Unlock(context.Background()); err != nil {
				//处理错误
			}
		}()
		fmt.Println("抢锁成功")
	}()
	go func() { //<7>
		defer wg.Done()
		locker := allocator.NewLocker(key)
		ctx, cancelFunc := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelFunc()
		if err := locker.Lock(ctx); err != nil { //ctx超时时间为3s，如果3s还未抢到锁则返回超时错误
			//处理错误...
			return
		}
		defer func() {
			if err := locker.Unlock(ctx); err != nil {
				fmt.Println("解锁失败,err:" + err.Error())
			}
		}()
		fmt.Println("抢锁成功")

	}()
	go func() { //<8>
		defer wg.Done()
		ctx, cancelFunc := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelFunc()
		locker := allocator.NewLocker(key) //ctx的指定超时时间为30s，但timeout指定15s，所以15s未抢锁成功则返回超时错误
		if err := locker.Lock(ctx, 15*time.Second); err != nil {
			//处理错误...
			return
		}
		defer func() {
			if err := locker.Unlock(ctx); err != nil {
				fmt.Println("解锁失败,err:" + err.Error())
			}
		}()
		fmt.Println("抢锁成功")
	}()
	go func() { //<9>
		defer wg.Done()
		ctx, cancelFunc := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancelFunc()
		locker := allocator.NewLocker(key) //ctx的指定超时时间为7s，但timeout指定20s，所以7s未抢锁成功则返回超时错误
		if err := locker.Lock(ctx, 20*time.Second); err != nil {
			//处理错误...
			return
		}
		defer func() {
			if err := locker.Unlock(ctx); err != nil {
				fmt.Println("解锁失败,err:" + err.Error())
			}
		}()
		fmt.Println("抢锁成功")
	}()
	wg.Wait()
}
