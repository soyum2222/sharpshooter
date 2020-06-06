package block

import (
	"errors"
)

var noblock = errors.New("no any block")
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

	case <-b.close:
		return closed
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

func (b *Blocker) PassB() error {

	<-b.c
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
