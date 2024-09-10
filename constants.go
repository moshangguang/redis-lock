package redislock

import (
	"errors"
	"time"
)

const (
	minRetryDelayMilliSec = 50
	maxRetryDelayMilliSec = 250
	minTimeout            = time.Millisecond * 100
	defaultTimeout        = time.Second * 10
	minKeepalive          = time.Second * 20
	maxKeepalive          = minKeepalive * 6
	stateLockNon          = 0 //未锁
	stateLockSuccess      = 1 //锁成功
)

var (
	LockerTimeout = errors.New("locker timeout")
	UnlockFail    = errors.New("unlock fail")
	LockerNonLock = errors.New("locker non lock")
)
