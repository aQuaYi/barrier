package barrier

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const (
	nonpositiveParticipants = "participants is NOT positive"
	tooMuchWaiting          = "goroutines calling b.Wait() is more than b.participants. Make sure they are equal."
)

var (
	// ErrBroken will be returned by all goroutines called Barrier.Wait() if a
	// goroutine called Barrier.Break()
	// The goroutine wait lately, will return this error at once.
	ErrBroken = errors.New("barrier is broken by other goroutine")
)

// Barrier is a synchronizer that allows a set of goroutines
// to wait for each other to reach a common execution point,
// also called a barrier.
// Barriers are useful in programs involving a fixed sized party of
// goroutines that must occasionally wait for each other.
// The barrier can be re-used after the waiting goroutines are released.
// A Barrier supports an optional Runnable command
// that is run once per barrier point, after the last goroutine in the party arrives,
// but before any goroutines are released.
// This barrier action is useful for updating
// shared-state before any of the parties continue.
type Barrier interface {
	// Wait until all participants have invoked wait on this barrier.
	// If another goroutine breaks the barrier, it will return ErrBroken
	// else return nil.
	Wait(ctx context.Context) error

	// Break is `Wait` with unfinished job.
	// The code of use `Break` is like
	// if ok := doJob(); ok {
	//     b.Wait()
	// } else {
	//     b.Break()
	// }
	Break()

	// IsBroken returns true if this round barrier is broken.
	IsBroken() bool

	// SetAction set an action will be execute after all participants
	// arrived the barrier.
	// Even the barrier is broken, the action will also be executed.
	SetAction(func()) Barrier
}

// New initializes a new instance of the Barrier, specifying the number of parties.
func New(participants int) Barrier {
	if participants <= 0 {
		panic(nonpositiveParticipants)
	}
	return &barrier{
		participants: participants,
		lock:         sync.RWMutex{},
		round:        newRound(),
	}
}

// barrier implements Barrier interface
type barrier struct {
	participants int
	lock         sync.RWMutex
	action       func()
	round        *round // every round has a new round
}

// round is a cycle of using barrier
// if any goroutine call Barrier.Break, this round is Broken
type round struct {
	isBroken bool
	count    int           // count of goroutines has arrived barrier
	success  chan struct{} // broadcast success result using close(success)
	broken   chan struct{} // broadcast broken status using close(borken)
}

func newRound() *round {
	return &round{
		success: make(chan struct{}),
		broken:  make(chan struct{}),
	}
}

func (b *barrier) Wait(ctx context.Context) (err error) {
	count, success, broken := b.newComer()
	if count < b.participants {
		// wait other participants
		select {
		case <-success:
			return nil
		case <-broken:
			return ErrBroken
		case <-ctx.Done():
			b.breakRound()
			return fmt.Errorf("barrier is broken: %w", ctx.Err())
		}
	}
	if count == b.participants {
		if b.IsBroken() {
			err = ErrBroken
		}
		b.lastArrived()
	}
	return
}

func (b *barrier) Break() {
	count, _, _ := b.newComer()
	b.breakRound()
	if count == b.participants {
		b.lastArrived()
	}
}

// lastArrived to do action and reset
func (b *barrier) lastArrived() {
	if b.action != nil {
		b.action()
	}
	b.resetRound()
}

func (b *barrier) IsBroken() (res bool) {
	b.lock.RLock()
	res = b.round.isBroken
	b.lock.RUnlock()
	return
}

// SetAction if you need
// action will be execute by
// the last **arrived** goroutine
func (b *barrier) SetAction(action func()) Barrier {
	b.lock.Lock()
	b.action = action
	b.lock.Unlock()
	return b
}

// meetNewComer save returns in local variables to prevent race
func (b *barrier) newComer() (count int, success, broken chan struct{}) {
	b.lock.Lock()
	b.round.count++
	count = b.round.count
	success = b.round.success
	broken = b.round.broken
	b.lock.Unlock()
	// 如果并发的 b.Wait() 的 goroutines 的数量
	// 大于 b.participants 的话，
	// 虽然 count++ 是在临界区内，但是 if 分支语句不在呀。
	// count = participants 刚刚 unlock 后，还没有到达 if 前。
	// 另一个 goroutine 进行了 count++ 运算
	// 就会导致 count > participants 成立
	if count > b.participants {
		panic(tooMuchWaiting)
	}
	return
}

func (b *barrier) breakRound() {
	b.lock.Lock()
	if !b.round.isBroken {
		b.round.isBroken = true
		close(b.round.broken) // broadcast to waiting goroutines
	}
	b.lock.Unlock()
}

func (b *barrier) resetRound() {
	b.lock.Lock()
	if !b.round.isBroken {
		close(b.round.success) // broadcast to waiting goroutines
	}
	b.round = newRound()
	b.lock.Unlock()
}
