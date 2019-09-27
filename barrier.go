package barrier

import (
	"context"
	"sync"
)

// Barrier is a synchronizer that allows a set of goroutines to wait for each other
// to reach a common execution point, also called a barrier.
// CyclicBarriers are useful in programs involving a fixed sized party of goroutines
// that must occasionally wait for each other.
// The barrier is called cyclic because it can be re-used after the waiting goroutines are released.
// A Barrier supports an optional Runnable command that is run once per barrier point,
// after the last goroutine in the party arrives, but before any goroutines are released.
// This barrier action is useful for updating shared-state before any of the parties continue.
// Barrier 同步原语用于多个 goroutine 相互等待。
// 即多个 goroutine 在同一个汇合点相互等待，直到全部到齐后，再一起继续执行的情况。
type Barrier interface {
	// Await waits until all parties have invoked await on this barrier.
	// 1. If the barrier is reset while any goroutine is waiting, or if the barrier is broken when await is invoked,
	// or while any goroutine is waiting, then ErrBrokenBarrier is returned.
	// 2. If any goroutine is interrupted by ctx.Done() while waiting, then all other waiting goroutines
	// will return ErrBrokenBarrier and the barrier is placed in the broken state.
	// 3. If the current goroutine is the last goroutine to arrive, and a non-nil barrier action was supplied in the constructor,
	// then the current goroutine runs the action before allowing the other goroutines to continue.
	// 4. If an error occurs during the barrier action then that error will be returned and the barrier is placed in the broken state.
	// 5. 如果 action != nil, 最后一个 Await 的 goroutine 会执行 action。
	Await(ctx context.Context) error

	// Reset resets the barrier to its initial state.
	// If any parties are currently waiting at the barrier, they will return with a ErrBrokenBarrier.
	Reset()

	// GetNumberWaiting returns the number of parties currently waiting at the barrier.
	GetNumberWaiting() int

	// GetParties returns the number of parties required to trip this barrier.
	GetParties() int

	// IsBroken queries if this barrier is in a broken state.
	// Returns true if one or more parties broke out of this barrier due to interruption by ctx.Done() or the last reset,
	// or a barrier action failed due to an error; false otherwise.
	IsBroken() bool
}

var (
	// ErrBrokenBarrier error used when a goroutine tries to wait upon a barrier that is in a broken state,
	// or which enters the broken state while the goroutine is waiting.
	// TODO: 自定义错误类型
	ErrBrokenBarrier error
)

// round
type round struct {
	count    int           // count of goroutines for this roundtrip
	waitCh   chan struct{} // wait channel for this roundtrip
	brokeCh  chan struct{} // channel for isBroken broadcast
	isBroken bool          // is barrier broken
}

func newRound() *round {
	return &round{
		waitCh:  make(chan struct{}),
		brokeCh: make(chan struct{}),
	}
}

// barrier implement Barrier interface
type barrier struct {
	parties int
	lock    sync.RWMutex
	action  func() error
	// every round has a new round
	round *round
}

// NewWithAction initializes a new instance of the CyclicBarrier,
// specifying the number of parties and the barrier action.
func NewWithAction(parties int, action func() error) Barrier {
	if parties <= 0 {
		panic("parties must be positive number")
	}
	return &barrier{
		parties: parties,
		lock:    sync.RWMutex{},
		action:  action,
		round:   newRound(),
	}
}

// New initializes a new instance of the Barrier, specifying the number of parties.
func New(parties int) Barrier {
	return NewWithAction(parties, nil)
}

func (b *barrier) Await(ctx context.Context) error {
	var doneCh <-chan struct{}
	if ctx != nil {
		doneCh = ctx.Done()
	}

	// check if context is done
	select {
	case <-doneCh:
		return ctx.Err()
	default:
	}

	b.lock.Lock()

	// check if broken
	if b.round.isBroken {
		b.lock.Unlock()
		return ErrBrokenBarrier
	}

	// increment count of waiters
	b.round.count++

	// saving in local variables to prevent race
	waitCh := b.round.waitCh
	brokeCh := b.round.brokeCh
	count := b.round.count

	b.lock.Unlock()

	// TODO: 有可能 > 吗？
	if count > b.parties {
		panic("CyclicBarrier.Await is called more than count of parties")
	}

	if count < b.parties {
		// wait other parties
		select {
		case <-waitCh:
			return nil
		case <-brokeCh:
			return ErrBrokenBarrier
		case <-doneCh:
			b.breakBarrier(true)
			return ctx.Err()
		}
	} else {
		// we are last, run the barrier action and reset the barrier
		if b.action != nil {
			err := b.action()
			if err != nil {
				b.breakBarrier(true) // TODO: 简化此处逻辑
				return err
			}
		}
		b.reset(true)
		return nil
	}
}

func (b *barrier) breakBarrier(needLock bool) {
	if needLock {
		b.lock.Lock()
		defer b.lock.Unlock()
	}

	if !b.round.isBroken {
		b.round.isBroken = true

		// broadcast
		close(b.round.brokeCh)
	}
}

func (b *barrier) reset(safe bool) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if safe {
		// broadcast to pass waiting goroutines
		close(b.round.waitCh)

	} else if b.round.count > 0 {
		b.breakBarrier(false)
	}

	// create new round
	b.round = newRound()
}

func (b *barrier) Reset() {
	b.reset(false)
}

func (b *barrier) GetParties() int {
	return b.parties
}

func (b *barrier) GetNumberWaiting() int {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.round.count
}

func (b *barrier) IsBroken() bool {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.round.isBroken
}
