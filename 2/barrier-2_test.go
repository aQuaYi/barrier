package barrier

import (
	"context"
	"sync"
	"testing"

	"github.com/marusama/cyclicbarrier"
)

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
