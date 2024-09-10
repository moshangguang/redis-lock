package redislock

import (
	"time"
)

type UnlockMessage struct {
	Key   string
	Owner string
	Done  chan<- UnlockDone
}

type UnlockDone struct {
	IsUnlock bool
	Error    error
}

type ErrorOption struct {
	Title string //出错消息
	Error error  //错误内容
	Panic interface{}
	Key   string
}

type AllocatorOption func(allocator *Allocator)

func WithLockerTimeout(timeout time.Duration) AllocatorOption {
	return func(allocator *Allocator) {
		allocator.timeout = timeout
	}
}
func WithErrHandler(errHandler func(option ErrorOption)) AllocatorOption {
	if errHandler == nil {
		panic("err handler is nil")
	}
	return func(allocator *Allocator) {
		allocator.errHandler = errHandler
	}
}

var deleteScript = `
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	else
		return 0
	end
`
