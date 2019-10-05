package barrier

import (
	"context"
	"sync"
	"testing"

	"github.com/marusama/cyclicbarrier"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAction(t *testing.T) {
	var b *barrier

	Convey("如果 Barrier 设置了 Action", t, func() {
		count := 0
		b = New(3).SetAction(func() {
			count = 1
		}).(*barrier)
		b.round.count = 2

		Convey("当最后一个参与值 Wait 的时候", func() {
			b.Wait(context.TODO())
			Convey("Action 会被执行", func() {
				So(count, ShouldEqual, 1)
			})
		})

		Convey("当最后一个参与者 Break 的时候", func() {
			b.Break()
			Convey("Action 会被执行", func() {
				So(count, ShouldEqual, 1)
			})

		})

	})

}

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

func TestBarrierStatus(t *testing.T) {
	Convey("假设 Barrier 有 3 个参与者", t, func() {
		b := New(3)
		status := 0
		b.SetAction(func() {
			if b.IsBroken() {
				status--
			} else {
				status++
			}
		})

		Convey("如果第 1 个参与者执行了 Wait", func() {
			goWait(b)
			So(status, ShouldEqual, 0)

			Convey("第 2 个参与者执行了 Wait", func() {
				goWait(b)
				So(status, ShouldEqual, 0)

				Convey("第 3 个参与者执行了 Wait", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldBeNil)
					So(status, ShouldEqual, 1)
				})

				Convey("第 3 个参与者执行了 Break", func() {
					b.Break()
					So(status, ShouldEqual, -1)
				})
			})

			Convey("第 2 个参与者执行了 Break", func() {
				b.Break()
				So(status, ShouldEqual, 0)

				Convey("第 3 个参与者执行了 Wait", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldEqual, ErrBroken)
					So(status, ShouldEqual, -1)
				})

				Convey("第 3 个参与者执行了 Break", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldEqual, ErrBroken)
					So(status, ShouldEqual, -1)
				})
			})
		})

		Convey("如果第 1 个参与者执行了 Break", func() {
			b.Break()
			So(status, ShouldEqual, 0)

			Convey("第 2 个参与者执行了 Wait", func() {
				err := b.Wait(context.TODO())
				So(err, ShouldEqual, ErrBroken)
				So(status, ShouldEqual, 0)

				Convey("第 3 个参与者执行了 Wait", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldEqual, ErrBroken)
					So(status, ShouldEqual, -1)
				})

				Convey("第 3 个参与者执行了 Break", func() {
					b.Break()
					So(status, ShouldEqual, -1)
				})
			})

			Convey("第 2 个参与者执行了 Break", func() {
				b.Break()
				So(status, ShouldEqual, 0)

				Convey("第 3 个参与者执行了 Wait", func() {
					err := b.Wait(context.TODO())
					So(err, ShouldEqual, ErrBroken)
					So(status, ShouldEqual, -1)
				})

				Convey("第 3 个参与者执行了 Break", func() {
					b.Break()
					So(status, ShouldEqual, -1)
				})
			})
		})
	})
}

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
