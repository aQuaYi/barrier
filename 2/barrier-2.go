package barrier

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ()

// Barrier is a synchronizer that allows a set of goroutines to wait for each other
// to reach a common execution point, also called a barrier.
// Barriers are useful in programs involving a fixed sized party of goroutines
// that must occasionally wait for each other.
// The barrier is called cyclic because it can be re-used after the waiting goroutines are released.
// A Barrier supports an optional Runnable command that is run once per barrier point,
// after the last goroutine in the party arrives, but before any goroutines are released.
// This barrier action is useful for updating shared-state before any of the parties continue.
// Barrier 同步原语用于多个 goroutine 相互等待。
// 即多个 goroutine 在同一个汇合点相互等待，直到全部到齐后，再一起继续执行的情况。
// NOTICE: 并发的 Barrier.SignalAndWait() 的 goroutine 数量一定要和 Barrier.GetParticipants() 相等。
type Barrier interface {
	// Await waits until all parties have invoked await on this barrier.
	// 1. If the barrier is reset while any goroutine is waiting, or if the barrier is broken when await is invoked,
	// 1. 如果 reset 的时候，有 goroutine 在 wait。或者，屏障 broke 时候，wait 已经被请求。
	// or while any goroutine is waiting, then ErrBrokenBarrier is returned.
	// 或 有 goroutine 正在 wait，会返回 ErrBrokenBarrier。
	// 2. If any goroutine is interrupted by ctx.Done() while waiting, then all other waiting goroutines
	// 2. 如果有 goroutine 被 ctx.Done 中断，所有其他等待中的 goroutine
	// will return ErrBrokenBarrier and the barrier is placed in the broken state.
	// 会返回 ErrBrokenBarrier 错误，barrier 会停在 broken 状态。
	// 3. If the current goroutine is the last goroutine to arrive, and a non-nil barrier action was supplied in the constructor,
	// 3. 如果本 goroutine 是最后到达的，且 action 不为空。
	// then the current goroutine runs the action before allowing the other goroutines to continue.
	// 那就由这个 goroutine 来执行 action，然后所有其他的 goroutine 才能继续。
	// 4. If an error occurs during the barrier action then that error will be returned and the barrier is placed in the broken state.
	// 4. 如果 action 执行过程中发生错误，那个 goroutine 会返回错误，
	// 5. 如果 action != nil, 最后一个 Await 的 goroutine 会执行 action。
	Await(ctx context.Context) error

	// Wait After this goroutine has got ready
	// In waiting, the goroutine can be canceled by context.Context
	Wait(ctx context.Context) error

	// BrokeAndSignal used when a goroutine can not prepare the need stuff finally
	// goroutine has to broke the barrier the tell the others.
	//
	// TODO: 添加此处内容
	// 对于一个 goroutine 来说， 要么 wait; 要么 broke
	Break()

	// IsBroken queries if this barrier is in a broken state.
	// Returns true if one or more parties broke out of this barrier due to interruption by ctx.Done() or the last reset,
	// or a barrier action failed due to an error;
	// false otherwise.
	IsBroken() bool

	SetAction(func() error) // TODO: 删除此处 error
}

var (
	// ErrBrokenBarrier error used when a goroutine tries to wait upon a barrier that is in a broken state,
	// or which enters the broken state while the goroutine is waiting.
	// TODO: 自定义错误类型
	ErrBrokenBarrier error

	// ErrBrokenByOther used when some goroutine wait early has canceled by its context.
	// the barrier is broken.
	// the goroutine wait lately, will return this error at once.
	ErrBrokenByOther = errors.New("barrier is broken by other goroutine")
)

// round is a cycle of using barrier
// every round has only two results: success or broken
// success means everything is ok
// otherwise the round is broken
// NOTICE: visit round need lock
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

// barrier implements Barrier interface
type barrier struct {
	participants int
	lock         sync.RWMutex
	action       func() error // TODO: 删除 error
	// every round has a new round
	round *round
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
func (b *barrier) SetAction(action func() error) {
	b.lock.Lock()
	b.action = action
	b.lock.Unlock()
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
			return ErrBrokenByOther
		case <-ctx.Done():
			// TODO: 修改逻辑
			b.breakRound()
			// return ctx.Err()
			return fmt.Errorf("barrier is broken: %w", ctx.Err())
		}
	} else {
		// TODO: 思考一下，需要改成 内部的方法吗？
		if b.IsBroken() {
			// 不能直接返回错误，需要下面还有 restRound 的工作要做
			err = ErrBrokenByOther
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

// TODO: 完成 Break 方法
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

func (b *barrier) Await(ctx context.Context) error {
	var doneCh <-chan struct{}
	if ctx != nil {
		doneCh = ctx.Done()
	}

	// check if context is done
	select {
	case <-doneCh:
		// TODO: 为什么这里不需要 breakBarrier（）
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
	waitCh := b.round.success
	brokeCh := b.round.broken
	count := b.round.count

	b.lock.Unlock()

	// TODO: 有可能 > 吗？
	// 有可能的，
	// 当并发的 Barrier.Await() 的 goroutine 数量大于 Barrier.GetParticipants() 时候，
	// 虽然 count++ 是在临界区内，但是 if 分支语句不在呀。
	// count = participants 刚刚 unlock 后，还没有到达 if 前。
	// 另一个 goroutine 进行了 count++ 运算
	// 就会导致 count > participants 成立
	if count > b.participants {
		panic("Barrier.Await is called more than count of parties")
	}

	// barrier 的核心逻辑
	if count < b.participants {
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
		// we are the last one,
		// run the barrier action
		// and
		// reset the barrier
		if b.action != nil {
			err := b.action()
			if err != nil {
				b.breakBarrier(true) // TODO: 简化此处逻辑
				// b.reset(true)
				return err
			}
		}
		b.reset(true)
		return nil
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
