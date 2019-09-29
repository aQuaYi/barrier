package barrier

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marusama/cyclicbarrier"
)

func checkBarrier(t *testing.T, b Barrier,
	expectedParties int, expectedIsBroken bool) {

	isBroken := b.IsBroken()

	if isBroken != expectedIsBroken {
		t.Error("barrier must have isBroken = ", expectedIsBroken, ", but has ", isBroken)
	}
}

func TestNew(t *testing.T) {
	tests := []func(){
		func() {
			b := New(10)
			checkBarrier(t, b, 10, false)
			if b.(*barrier).action != nil {
				t.Error("barrier have unexpected barrierAction")
			}
		},
		func() {
			defer func() {
				if recover() == nil {
					t.Error("Panic expected")
				}
			}()
			_ = New(0)
		},
		func() {
			defer func() {
				if recover() == nil {
					t.Error("Panic expected")
				}
			}()
			_ = New(-1)
		},
	}
	for _, test := range tests {
		test()
	}
}

func TestNewWithAction(t *testing.T) {
	tests := []func(){
		func() {
			b := New(10)
			b.SetAction(func() error { return nil })
			checkBarrier(t, b, 10, false)
			if b.(*barrier).action == nil {
				t.Error("barrier doesn't have expected barrierAction")
			}
		},
		func() {
			b := New(10)
			checkBarrier(t, b, 10, false)
			if b.(*barrier).action != nil {
				t.Error("barrier have unexpected barrierAction")
			}
		},
		func() {
			defer func() {
				if recover() == nil {
					t.Error("Panic expected")
				}
			}()
			_ = New(0)
		},
		func() {
			defer func() {
				if recover() == nil {
					t.Error("Panic expected")
				}
			}()
			_ = New(-1)
		},
	}
	for _, test := range tests {
		test()
	}
}

func TestAwaitOnce(t *testing.T) {
	n := 100 // goroutines count
	b := New(n)
	ctx := context.Background()

	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			err := b.Await(ctx)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	checkBarrier(t, b, n, false)
}

func TestAwaitMany(t *testing.T) {
	n := 100  // goroutines count
	m := 1000 // inner cycle count
	b := New(n)
	ctx := context.Background()

	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(num int) {
			for j := 0; j < m; j++ {
				err := b.Await(ctx)
				if err != nil {
					panic(err)
				}
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
	checkBarrier(t, b, n, false)
}

func TestAwaitOnceCtxDone(t *testing.T) {
	n := 100        // goroutines count
	b := New(n + 1) // parties are more than goroutines count so all goroutines will wait
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var deadlineCount, brokenBarrierCount int32

	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(num int) {
			err := b.Await(ctx)
			if err == context.DeadlineExceeded {
				atomic.AddInt32(&deadlineCount, 1)
			} else if err == ErrBrokenBarrier {
				atomic.AddInt32(&brokenBarrierCount, 1)
			} else {
				panic("must be context.DeadlineExceeded or ErrBrokenBarrier error")
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
	checkBarrier(t, b, n+1, true)
	if deadlineCount == 0 {
		t.Error("must be more than 0 context.DeadlineExceeded errors, but found", deadlineCount)
	}
	if deadlineCount+brokenBarrierCount != int32(n) {
		t.Error("must be exactly", n, "context.DeadlineExceeded and ErrBrokenBarrier errors, but found", deadlineCount+brokenBarrierCount)
	}
}

func TestAwaitManyCtxDone(t *testing.T) {
	n := 100 // goroutines count
	b := New(n)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			for {
				err := b.Await(ctx)
				if err != nil {
					if err != context.DeadlineExceeded && err != ErrBrokenBarrier {
						panic("must be context.DeadlineExceeded or ErrBrokenBarrier error")
					}
					break
				}
			}
			wg.Done()
		}()
	}

	wg.Wait()
	checkBarrier(t, b, n, true)
}

func TestAwaitAction(t *testing.T) {
	n := 100  // goroutines count
	m := 1000 // inner cycle count
	ctx := context.Background()

	cnt := 0
	b := New(n)
	b.SetAction(func() error {
		cnt++
		return nil
	})

	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			for j := 0; j < m; j++ {
				err := b.Await(ctx)
				if err != nil {
					panic(err)
				}
			}
			wg.Done()
		}()
	}

	wg.Wait()
	checkBarrier(t, b, n, false)
	if cnt != m {
		t.Error("cnt must be equal to = ", m, ", but it's ", cnt)
	}
}

func TestAwaitErrorInActionThenReset(t *testing.T) {
	n := 100 // goroutines count
	ctx := context.Background()

	errExpected := errors.New("test error")

	isActionCalled := false
	var expectedErrCount, errBrokenBarrierCount int32

	b := New(10)
	b.SetAction(func() error {
		isActionCalled = true
		return errExpected
	})

	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			err := b.Await(ctx)
			if err == errExpected {
				atomic.AddInt32(&expectedErrCount, 1)
			} else if err == ErrBrokenBarrier {
				atomic.AddInt32(&errBrokenBarrierCount, 1)
			} else {
				panic(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	checkBarrier(t, b, n, true) // check that barrier is broken
	if !isActionCalled {
		t.Error("barrier action must be called")
	}
	if !b.IsBroken() {
		t.Error("barrier must be broken via action error")
	}
	if expectedErrCount != 1 {
		t.Error("expectedErrCount must be equal to", 1, ", but it equals to", expectedErrCount)
	}
	if errBrokenBarrierCount != int32(n-1) {
		t.Error("expectedErrCount must be equal to", n-1, ", but it equals to", errBrokenBarrierCount)
	}

	// call await on broken barrier must return ErrBrokenBarrier
	if b.Await(ctx) != ErrBrokenBarrier {
		t.Error("call await on broken barrier must return ErrBrokenBarrier")
	}
}

func TestAwaitTooMuchGoroutines(t *testing.T) {

	goroutines := 100 // goroutines count
	cycle := 1000     // inner cycle count
	b := New(1)
	ctx := context.Background()

	var panicCount int32

	wg := sync.WaitGroup{}
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(num int) {
			defer func() {
				r := recover()
				if r != nil {
					fmt.Println("too many: ", r)
					atomic.AddInt32(&panicCount, 1)
				}
				wg.Done()
			}()
			for j := 0; j < cycle; j++ {
				err := b.Await(ctx)
				if err != nil {
					fmt.Println("b.Await Err: ", err)
				}
			}
		}(i)
	}

	wg.Wait()
	checkBarrier(t, b, 1, false)

	if panicCount == 0 {
		t.Error("barrier must panic when await is called from too much goroutines")
	}

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
		oneRound(parties, cycles, cb.Await)
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
