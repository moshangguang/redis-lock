package redislock

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type Allocator struct {
	timeout     time.Duration
	errHandler  func(option ErrorOption)
	redisClient *redis.Client
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
		redisClient: redisClient,
	}
	if len(options) != 0 {
		for _, opt := range options {
			if opt != nil {
				opt(allocator)
			}
		}
	}
	if allocator.timeout == 0 {
		allocator.timeout = defaultTimeout
	}
	if allocator.timeout < minTimeout {
		allocator.timeout = minTimeout
	}

	if allocator.errHandler == nil {
		allocator.errHandler = func(option ErrorOption) {
		}
	}

	return allocator
}

func (allocator *Allocator) NewLocker(key string) Locker {
	locker := &RLocker{
		key:         key,
		owner:       uuid.New().String(),
		timeout:     allocator.timeout,
		done:        make(chan struct{}),
		state:       new(uint32),
		redisClient: allocator.redisClient,
		errHandler:  allocator.errHandler,
	}
	return locker
}
