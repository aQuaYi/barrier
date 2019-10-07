package barrier

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/marusama/cyclicbarrier"
	. "github.com/smartystreets/goconvey/convey"
)

// goWait make sure b.Wait is waiting
func goWait(b Barrier) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		b.Wait(context.TODO())
	}()
	wg.Wait()
	return
}

// count arriver
func count(b Barrier) int {
	// NOTICE: 访问 Barrier 的原始数据结构，不是一个好行为
	bp := b.(*barrier)
	bp.lock.RLock()
	res := bp.round.count
	bp.lock.RUnlock()
	return res
}

func TestNew(t *testing.T) {
	Convey("如果想要新建一个 Barrier", t, func() {

		Convey("当 participants 是 0 的时候", func() {
			Convey("就会 panic", func() {
				So(func() {
					New(0)
				}, ShouldPanicWith, nonPositiveParticipants)
			})
		})

		Convey("当 participants 是 -1 的时候", func() {
			Convey("也会 panic", func() {
				So(func() {
					New(-1)
				}, ShouldPanicWith, nonPositiveParticipants)
			})
		})

		Convey("当 participants 是 2 的时候", func() {
			Convey("就不会 panic", func() {
				So(func() {
					New(2)
				}, ShouldNotPanicWith, nonPositiveParticipants)
			})
		})

		Convey("当 participants 是 1 的时候", func() {
			Convey("也不会 panic", func() {
				So(func() {
					New(1)
				}, ShouldNotPanicWith, nonPositiveParticipants)
			})
		})
	})
}

func TestAction(t *testing.T) {
	participants := 5
	Convey("如果 Barrier 设置了 Action", t, func() {
		status := 0
		b := New(participants).SetAction(func() {
			status = 1
		})

		Convey("除了最后一个参与者，都已经 Wait 了", func() {
			for i := 1; i < participants; i++ {
				goWait(b)
				s := fmt.Sprintf("已经执行了 %d 个 Wait， ", i)
				Convey(s+"Status 依然应该为 0", func() {
					So(status, ShouldEqual, 0)
					So(count(b), ShouldEqual, i) // TODO: 这里出现过报错
				})
			}

			Convey("当最后一个参与值 Wait 的时候", func() {
				b.Wait(context.TODO())
				Convey("Action 会被执行, status 成为了 1", func() {
					So(status, ShouldEqual, 1)
				})
			})

			Convey("当最后一个参与者 Break 的时候", func() {
				b.Break()
				Convey("Action 会被执行, status 成为了 1", func() {
					So(status, ShouldEqual, 1)
				})
			})
		})
	})
}

func TestBarrierStatus(t *testing.T) {
	Convey("假设 Barrier 有 3 个参与者，其中", t, func() {
		b := New(3)
		status := 0
		statusCh := make(chan int, 1)

		b.SetAction(func() {
			if b.IsBroken() {
				status--
			} else {
				status++
			}
			statusCh <- status
		})

		Convey("第 1 个参与者执行了 Wait", func() {
			goWait(b)
			So(status, ShouldEqual, 0)

			Convey("第 2 个参与者执行了 Wait", func() {
				goWait(b)
				So(status, ShouldEqual, 0)

				Convey("第 3 个参与者执行了 Wait", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldBeNil)
					So(<-statusCh, ShouldEqual, 1)
				})

				Convey("第 3 个参与者执行了 Break", func() {
					b.Break()
					So(<-statusCh, ShouldEqual, -1)
				})

			})

			Convey("第 2 个参与者执行了 Break", func() {
				b.Break()
				So(status, ShouldEqual, 0)
				So(b.IsBroken(), ShouldBeTrue)

				Convey("第 3 个参与者执行了 Wait", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldEqual, ErrBroken)
					So(<-statusCh, ShouldEqual, -1)
				})

				Convey("第 3 个参与者执行了 Break", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldEqual, ErrBroken)
					So(<-statusCh, ShouldEqual, -1)
				})
			})
		})

		Convey("第 1 个参与者执行了 Break", func() {
			b.Break()
			So(status, ShouldEqual, 0)
			So(b.IsBroken(), ShouldBeTrue)

			Convey("第 2 个参与者执行了 Wait", func() {
				err := b.Wait(context.TODO())
				So(err, ShouldEqual, ErrBroken)
				So(b.IsBroken(), ShouldBeTrue)
				So(status, ShouldEqual, 0)

				Convey("第 3 个参与者执行了 Wait", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldEqual, ErrBroken)
					So(<-statusCh, ShouldEqual, -1)
				})

				Convey("第 3 个参与者执行了 Break", func() {
					b.Break()
					So(<-statusCh, ShouldEqual, -1)
				})
			})

			Convey("第 2 个参与者执行了 Break", func() {
				b.Break()
				So(b.IsBroken(), ShouldBeTrue)
				So(status, ShouldEqual, 0)

				Convey("第 3 个参与者执行了 Wait", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldEqual, ErrBroken)
					So(<-statusCh, ShouldEqual, -1)
				})

				Convey("第 3 个参与者执行了 Break", func() {
					b.Break()
					So(<-statusCh, ShouldEqual, -1)
				})
			})
		})
	})
}

func TestTooMuchWaiting(t *testing.T) {
	noSend := make(chan struct{})
	Convey("如果所有的 participants 已经到齐了", t, func() {
		b := New(2).SetAction(func() {
			<-noSend
		})
		goWait(b)
		goWait(b)
		Convey("再次调用 b.Wait，会触发 panic", func() {
			So(func() {
				b.Wait(context.TODO())
			}, ShouldPanicWith, tooMuchWaiting)
		})
	})
}

func TestContextCancel(t *testing.T) {
	Convey("Barrier 中有一个 goroutine 已经 waiting", t, func() {
		b := New(2)
		ctx, cancel := context.WithCancel(context.Background())
		var err error
		var waitBefore, waitAfter sync.WaitGroup
		waitBefore.Add(1)
		waitAfter.Add(1)
		go func() {
			waitBefore.Done()
			err = b.Wait(ctx)
			waitAfter.Done()
		}()

		waitBefore.Wait()

		Convey("在 Cancel 之前，b 不是 broken", func() {
			So(b.IsBroken(), ShouldBeFalse)
			So(err, ShouldBeNil)
			So(count(b), ShouldEqual, 1)
		})

		cancel()
		waitAfter.Wait()

		Convey("在 Cancel 之后，b 是 broken", func() {
			So(b.IsBroken(), ShouldBeTrue)
			So(err.Error(), ShouldEqual, "barrier is broken: context canceled")
			So(count(b), ShouldEqual, 1)
		})
	})
}

func TestBarrierCyclic(t *testing.T) {
	round := 5
	participants := 7
	roundCount := 0
	roundCh := make(chan int, 1)
	b := New(participants).SetAction(func() {
		roundCount++
		roundCh <- roundCount
	})

	Convey("循环使用同一个 Barrier", t, func() {
		for r := 1; r <= round; r++ {
			for p := 1; p < participants; p++ {
				goWait(b)
				So(count(b), ShouldEqual, p)
			}
			// err := b.Wait(context.TODO())
			// So(err, ShouldBeNil)
			err := b.Wait(context.TODO())
			So(err, ShouldBeNil)
			So(<-roundCh, ShouldEqual, r)
		}
	})
}

// below is benchmark

func oneRound(parties, cycles int, wait func(context.Context) error) {
	var wg sync.WaitGroup
	wg.Add(parties)
	for i := 0; i < parties; i++ {
		go func() {
			for c := 0; c < cycles; c++ {
				wait(context.TODO())
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func Benchmark_CyclicBarrier(b *testing.B) {
	parties := 10
	cycles := 10
	cb := cyclicbarrier.New(parties)
	//
	for i := 1; i < b.N; i++ {
		oneRound(parties, cycles, cb.Await)
	}
}

func Benchmark_Barrier(b *testing.B) {
	parties := 10
	cycles := 10
	cb := New(parties)
	//
	for i := 1; i < b.N; i++ {
		oneRound(parties, cycles, cb.Wait)
	}
}

type boc struct {
	isOk bool
	l    sync.RWMutex
	ch   chan struct{}
}

var g = 100

func Benchmark_boc_readlock(b *testing.B) {
	bb := &boc{}
	var wg sync.WaitGroup
	b.ResetTimer()
	for i := 1; i < b.N; i++ {
		wg.Add(g)
		for j := 0; j < g; j++ {
			go func() {
				bb.l.RLock()
				if bb.isOk {
				} else {
				}
				bb.l.RUnlock()
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func Benchmark_boc_lock(b *testing.B) {
	bb := &boc{}
	var wg sync.WaitGroup
	b.ResetTimer()
	for i := 1; i < b.N; i++ {
		wg.Add(g)
		for j := 0; j < g; j++ {
			go func() {
				bb.l.Lock()
				if bb.isOk {
				} else {
				}
				bb.l.Unlock()
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func Benchmark_boc_readclosedChannel(b *testing.B) {
	bb := &boc{
		ch: make(chan struct{}),
	}
	close(bb.ch)
	var wg sync.WaitGroup
	b.ResetTimer()
	for i := 1; i < b.N; i++ {
		wg.Add(g)
		for j := 0; j < g; j++ {
			go func() {
				bb.l.Lock()
				select {
				case <-bb.ch:
				default:
				}
				bb.l.Unlock()
				wg.Done()
			}()
		}
		wg.Wait()
	}
}
