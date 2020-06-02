package block

import (
	"errors"
)

var noblock = errors.New("no any block")

type Blocker struct {
	c chan struct{}
}

func (b *Blocker) Init() {
	b.c = make(chan struct{}, 0)
}

func (b *Blocker) Block() {
	b.c <- struct{}{}
}

func (b *Blocker) Pass() error {
	select {
	case <-b.c:
		return nil
	default:
		return noblock
	}
}

func (b *Blocker) Close() {
	close(b.c)
}
