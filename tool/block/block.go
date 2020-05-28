package block

import (
	"errors"
	"sync"
)

var noblock = errors.New("no any block")

type Blocker struct {
	o sync.Once
	c chan struct{}
}

func (b *Blocker) init() {
	b.c = make(chan struct{}, 0)
}

func (b *Blocker) Block() {

	b.o.Do(b.init)

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
