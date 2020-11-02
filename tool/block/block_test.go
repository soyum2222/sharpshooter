package block

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlock(t *testing.T) {

	b := NewBlocker()
	go func() {
		b.Pass()
	}()

	b.Block()
}

func TestMultiBlock(t *testing.T) {

	b := NewBlocker()

	wait := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		go func() {
			wait.Add(1)
			b.Block()
			wait.Done()
		}()
	}

	time.Sleep(time.Millisecond)

	for i := 0; i < 10; i++ {
		b.Pass()
	}
	wait.Wait()
}

func TestOrder(t *testing.T) {

	b := NewBlocker()

	var tag int64

	verification := func(i int) {
		if (atomic.AddInt64(&tag, 1) - 1) != int64(i) {
			t.Fail()
			return
		}
	}

	for i := 0; i < 100; i++ {

		go func(i int) {
			b.Block()
			verification(i)
		}(i)

		time.Sleep(time.Millisecond)
	}

	for i := 0; i < 100; i++ {
		b.Pass()
		time.Sleep(time.Millisecond)
	}
}

func TestClose(t *testing.T) {

	b := NewBlocker()

	wait := sync.WaitGroup{}
	for i := 0; i < 100; i++ {

		wait.Add(1)
		go func() {
			b.Block()
			wait.Done()
		}()

	}

	b.Close()
	wait.Wait()
}
