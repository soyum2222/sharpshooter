package block

import (
	"errors"
	"time"
)

var noblock = errors.New("without any block")
var closed = errors.New("blocker be closed")

type Blocker struct {
	c     chan struct{}
	close chan struct{}
}

func NewBlocker() *Blocker {
	b := &Blocker{}
	b.Init()
	return b
}

func (b *Blocker) Init() {
	b.c = make(chan struct{}, 0)
	b.close = make(chan struct{}, 0)
}

func (b *Blocker) Block() (err error) {

	select {
	case b.c <- struct{}{}:

	case _, ok := <-b.close:
		if !ok {
			return closed
		}
	}
	return
}

func (b *Blocker) Pass() error {

	select {
	case <-b.c:
		return nil
	default:
		return noblock
	}
}

func (b *Blocker) PassBT(duration time.Duration) error {

	ticker := time.NewTicker(duration)
	select {
	case <-ticker.C:
		ticker.Stop()
		return nil
	case <-b.c:

	}
	return nil
}

func (b *Blocker) Close() {
	select {
	case <-b.close:
		return
	default:
		close(b.close)
	}
}
