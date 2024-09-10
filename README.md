# redis-lock

## 背景需求

目前很多github上的Redis分布式锁的实现较为简单，需要外部设置key和在Redis的自动过期时间，但像一些需要分布式锁的长时间任务不好预计自动过期时间。因此需要有一个会自动续期的Redis锁，不需要外部关心key的超时时间，并且支持多种方式的抢锁API来满足不同过的业务需要。

## 接口说明

redis-lock的Locker是基于Redis实现的分布式锁，提供了多种抢锁、解锁方式。

```go
type Locker interface {
	Lock(ctx context.Context, timeout ...time.Duration) (err error) 
	TryLock(ctx context.Context) (bool, error)                      
	Unlock(ctx context.Context) error                              
}
```

1. Lock() ，允许外部传入超时时间timeout，如果在指定时间内还未抢到锁，则会返回抢锁超时的错误，如果不传入timeout，则会使用默认的超时时间，【用例说明】会讲解如何指定默认的超时时间。
2. TryLock()，尝试抢锁，无阻塞或等待，抢锁成功返回true，失败返回false。
3. Unlock() ，解锁。



## 用例说明

前面我们已经知道Locker接口的API使用方式，现在我们要来了解如何创建一个Locker实例。这里要引入redis-lock的一个概念—分配器Allocator。我们需要创建一个分配器Allocator来创建Locker实例，创建分配器时，会要求我们指定默认的超时时间和默认的错误处理器。

```go
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


```

创建生成器有以下几个步骤：

1. 先创建Redis客户端，见上述代码<1>。
2. 创建钉钉通知，钉钉通知用于后续通知错误，见<2>。
3. 创建分配器：
   1. 配置Redis客户端，必传，见<3>。
   2. 配置默认超时时间，见<4>，非必传，如果不指定超时时间默认为10s。
   3. 配置错误处理器，见<5>，要求传入钉钉通知，如果出现错误时会通知到钉钉群，该选项如果不传则会忽略错误。

至此，我们已经创建好一个分配器，分配器的作用仅仅是为了存储一些默认参数，比如：超时时间、错误处理器，以便在创建Locker实例的时候，为Locker实例分配默认的超时、错误处理器。

接下去我们可以使用分配器来生成Locker实例，见上述代码<6>、<7>、<8>、<9>，这四处我们分别启动一个协程，并调用allocator实例的NewLocker()方法分别在各自的协程创建一个Locker实例，通常一个项目使用一个Allocator分配器实例。

这四个Locker实例都是抢同一个key的锁。这四处分别使用不同的方式来进行超时抢锁：

1. 协程<6>使用分配器默认配置的超时时间，即为5s。
2. 协程<7>我们创建一个超时时间为3s的ctx来抢锁，索引超时时间为3s。
3. 协程<8>我们创建一个超时时间为30s的ctx，但指定timeout为15s，因此超时时间为15s。
4. 协程<9>我们创建一个超时时间为7s的ctx，但指定timeout为20s，因此超时时间为7s。





