package barrier

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrBroken will be returned by all goroutines called Barrier.Wait() if a
	// goroutine called Barrier.Break()
	// The goroutine wait lately, will return this error at once.
	ErrBroken = errors.New("barrier is broken by other goroutine")
)

// Barrier is a synchronizer that allows a set of goroutines to wait for each other
// to reach a common execution point, also called a barrier.
// Barriers are useful in programs involving a fixed sized party of goroutines
// that must occasionally wait for each other.
// The barrier is cyclic because it can be re-used after the waiting goroutines are released.
// A Barrier supports an optional Runnable command that is run once per barrier point,
// after the last goroutine in the party arrives, but before any goroutines are released.
// This barrier action is useful for updating shared-state before any of the parties continue.
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

	// IsBroken returns true if this barrier is broken.
	IsBroken() bool

	// SetAction set an action will be execute after all participants
	// arrived the barrier.
	// Even the barrier is broken, the action will also be executed.
	SetAction(func()) Barrier
}

// barrier implements Barrier interface
type barrier struct {
	participants int
	lock         sync.RWMutex
	action       func()
	// every round has a new round
	round *round
}

// round is a cycle of using barrier
// every round has only two results: success or broken
// success means everything is ok
// otherwise the round is broken
type round struct {
	// this barrier round has broken
	isBroken bool
	// count of goroutines has arrived barrier
	count int
	// broadcast success result when close(success)
	success chan struct{}
	// broadcast broken status to waiting goroutine when close(borken)
	// this round is broken status after close(broken)
	broken chan struct{}
}

func newRound() *round {
	return &round{
		success: make(chan struct{}),
		broken:  make(chan struct{}),
	}
}

// NOTICE: used in lock
// save returns in local variables to prevent race
func (r *round) meetNewComer() (count int, success, broken chan struct{}) {
	r.count++
	count = r.count
	success = r.success
	broken = r.broken
	return
}

// New initializes a new instance of the Barrier, specifying the number of parties.
func New(participants int) Barrier {
	if participants <= 0 {
		panic("parties must be positive number")
	}
	return &barrier{
		participants: participants,
		lock:         sync.RWMutex{},
		round:        newRound(),
	}
}

// SetAction if you need
// action will be execute by
// the last **Wait** goroutine
// OR the first **Break** goroutine
func (b *barrier) SetAction(action func()) Barrier {
	b.lock.Lock()
	b.action = action
	b.lock.Unlock()
	return b
}

func (b *barrier) Wait(ctx context.Context) (err error) {
	b.lock.Lock()
	count, success, broken := b.round.meetNewComer()
	b.lock.Unlock()

	// 如果并发的 b.Wait() 的 goroutines 的数量
	// 大于 b.participants 的话，
	// 虽然 count++ 是在临界区内，但是 if 分支语句不在呀。
	// count = participants 刚刚 unlock 后，还没有到达 if 前。
	// 另一个 goroutine 进行了 count++ 运算
	// 就会导致 count > participants 成立
	if count > b.participants {
		panic("goroutines calling b.Wait() is more than b.participants. Make sure they are equal.")
	}

	if count < b.participants {
		// wait other parties
		select {
		case <-success:
			return nil
		case <-broken:
			return ErrBroken
		case <-ctx.Done():
			// TODO: 修改逻辑
			b.breakRound()
			// return ctx.Err()
			return fmt.Errorf("barrier is broken: %w", ctx.Err())
		}
	} else {
		if b.IsBroken() {
			// 不能直接返回错误，需要下面还有 restRound 的工作要做
			err = ErrBroken
		}
		if b.action != nil {
			b.action()
		}
		// 无论成功与否
		// 最后达到的 goroutine 负责重置 barrier
		b.resetRound()
		return
	}
}

func (b *barrier) Break() {
	// TODO: 把 round.meetNewComer 改写成 b.meetNewComer
	b.lock.Lock()
	count, _, _ := b.round.meetNewComer()
	b.lock.Unlock()

	if count > b.participants {
		panic("goroutines calling b.Wait() is more than b.participants. Make sure they are equal.")
	}

	b.breakRound()

	if count == b.participants {
		if b.action != nil {
			b.action()
		}
		// 无论成功与否
		// 最后达到的 goroutine 负责重置 barrier
		b.resetRound()
	}
}

func (b *barrier) breakRound() {
	b.lock.Lock()
	defer b.lock.Unlock()
	if !b.round.isBroken {
		b.round.isBroken = true
		// broadcast
		close(b.round.broken)
	}
}

// TODO: 删除此处内容
func (b *barrier) breakBarrier(needLock bool) {
	if needLock {
		b.lock.Lock()
		defer b.lock.Unlock()
	}
	if !b.round.isBroken {
		b.round.isBroken = true
		// broadcast
		close(b.round.broken)
	}
}

func (b *barrier) resetRound() {
	b.lock.Lock()
	defer b.lock.Unlock()
	if !b.round.isBroken {
		// broadcast to pass waiting goroutines
		close(b.round.success)
		// round.success 和 round.broken
		// 这两个 channel 总是成对出现的
		// ！isBroken 才需要 close（success） 来通知大家。
	}
	b.round = newRound()
}

// broadcast all that everyone is waiting.
func (b *barrier) broadcast() {
	close(b.round.success)
}

// TODO: 删除此处内容
func (b *barrier) reset(safe bool) {
	b.lock.Lock()
	defer b.lock.Unlock()
	if safe {
		// broadcast to pass waiting goroutines
		close(b.round.success)
	} else if b.round.count > 0 {
		b.breakBarrier(false)
	}
	// create new round
	b.round = newRound()
}

func (b *barrier) IsBroken() bool {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.round.isBroken
}
