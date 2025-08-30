package redislock

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

type Allocator struct {
	timeout    time.Duration
	expire     time.Duration
	errHandler func(option ErrorOption)
	client     *redis.Client
}

func NewAllocator(redisClient *redis.Client, options ...AllocatorOption) *Allocator {
	if redisClient == nil {
		panic("redis client is nil")
	}
	err := redisClient.Ping(context.Background()).Err()
	if err != nil {
		panic(err)
	}
	allocator := &Allocator{
		timeout: defaultTimeout,
		expire:  defaultExpire,
		errHandler: func(option ErrorOption) {
			list := make([]interface{}, 0)
			if len(option.Title) != 0 {
				list = append(list, option.Title)
			}
			if len(option.Key) != 0 {
				list = append(list, fmt.Sprintf("key:%s", option.Key))
			}
			if option.Error != nil {
				list = append(list, fmt.Sprintf("err:%s", option.Error.Error()))
			}
			if option.Panic != nil {
				list = append(list, fmt.Sprintf("panic:%v", option.Panic))
			}
			log.Println(list...)
		},
		client: redisClient,
	}
	if len(options) != 0 {
		for _, opt := range options {
			if opt != nil {
				opt(allocator)
			}
		}
	}

	return allocator
}

func (allocator *Allocator) NewLocker(key string) Locker {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	locker := &RLocker{
		key:         key,
		value:       fmt.Sprintf("%d", time.Now().UnixNano()),
		timeout:     allocator.timeout,
		expire:      allocator.expire,
		redisClient: allocator.client,
		cancelFunc:  nil,
		errHandler:  allocator.errHandler,
	}
	return locker
}
