# barrier

[![Build Status](https://travis-ci.org/aQuaYi/barrier.svg?branch=master)](https://travis-ci.org/aQuaYi/barrier)
[![codecov](https://codecov.io/gh/aQuaYi/barrier/branch/master/graph/badge.svg)](https://codecov.io/gh/aQuaYi/barrier)
[![Go Report Card](https://goreportcard.com/badge/github.com/aQuaYi/barrier)](https://goreportcard.com/report/github.com/aQuaYi/barrier)
[![GoDoc](https://godoc.org/github.com/aQuaYi/barrier?status.svg)](https://godoc.org/github.com/aQuaYi/barrier)
[![License](https://img.shields.io/github/license/mashape/apistatus.svg?maxAge=2592000)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.13+-blue.svg)](https://golang.google.cn)

`barrier` 是一种基本的同步原语，当多个 `goroutine` 需要相互等待，同时到达同一个汇合点的时候，特别有用。

- [内容介绍](#%e5%86%85%e5%ae%b9%e4%bb%8b%e7%bb%8d)
- [使用方法](#%e4%bd%bf%e7%94%a8%e6%96%b9%e6%b3%95)
- [简单例子](#%e7%ae%80%e5%8d%95%e4%be%8b%e5%ad%90)

## 内容介绍

<!-- TODO: 修改一下 -->

由于 Barrier.SignalAndWait() 会进入临界区，所以，要求操作尽可能的少。

与 [marusama/cyclicbarrier](https://github.com/marusama/cyclicbarrier) 相比，做出了以下修改

1. barrier.action 的类型变成了 `func()`，取消返回 error 后，逻辑更简单。
2. 移除了 `Barrier` 接口中的 `Reset` 方法。由最后到达 barrier 的 goroutine 负责重置。因为如果对外暴露了 `Reset` 方法的话，会需要对所有的 goroutine 进行一次同步，可以看看 [Java 版 CyclicBarrier 的说明](https://docs.oracle.com/javase/9/docs/api/java/util/concurrent/CyclicBarrier.html#reset--)
3. 添加了 `break` 方法。理由是基于以下假设，barrier 可能存在以下使用情况，多个 goroutine 在为同一个东西准备不同的原材料，如果某个 goroutine 始终无法完成准备。为了结束此 round 的运行，它需要通知其他 goroutine。但它不能调用 `wait` 方法，因为 `wait` 隐含了`已准备好`的意思。调用 `Break` 就可以很恰当地表达 `我已到达汇合点，但很抱歉，没有做好准备` 的意思。
4. 取消了 `NewWithAction`，但增加了 `SetAction`。这样的话，利用 `闭包` 属性，在 `action` 可以调用 `Barrier` 接口的方法。这样的话， `Barrier.IsBroken` 才更有意义。
5. 移除了 `GetNumberWaiting` 和 `GetParities` 方法。我想了两天，也想不出来这两个方法，有存在的意义。

## 使用方法

初始化

```go
import "github.com/aQuaYi/barrier"
...
b1 := cyclicbarrier.New(10) // new cyclic barrier with parties = 10
...
b2 := cyclicbarrier.NewWithAction(10, func() error { return nil }) // new cyclic barrier with parties = 10 and with defined barrier action
```

Wait

```go
b.Wait(ctx)    // await other parties
```

## 简单例子

```go
// create a barrier for 10 parties with an action that increments counter
// this action will be called each time when all goroutines reach the barrier
cnt := 0
b := cyclicbarrier.NewWithAction(10, func() error {
    cnt++
    return nil
})

wg := sync.WaitGroup{}
for i := 0; i < 10; i++ {           // create 10 goroutines (the same count as barrier parties)
    wg.Add(1)
    go func() {
        for j := 0; j < 5; j++ {

            // do some hard work 5 times
            time.Sleep(100 * time.Millisecond)

            err := b.Await(context.TODO()) // ..and wait for other parties on the barrier.
                                           // Last arrived goroutine will do the barrier action
                                           // and then pass all other goroutines to the next round
            if err != nil {
                panic(err)
            }
        }
        wg.Done()
    }()
}

wg.Wait()
fmt.Println(cnt)                    // cnt = 5, it means that the barrier was passed 5 times
```

For more documentation see <https://godoc.org/github.com/aQuaYi/barrier>
