package redislock

import (
	"errors"
	"time"
)

const (
	minRetryDelayMilliSec = 50
	maxRetryDelayMilliSec = 250
	defaultTimeout        = time.Second * 10
	defaultExpire         = time.Second * 10
)

var (
	LockerTimeout = errors.New("locker timeout")
	UnlockFail    = errors.New("unlock fail")
)
