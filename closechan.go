package sharpshooter

import "sync"

//func closeChan(c chan struct{}) {
//
//	select {
//	case <-c:
//	default:
//		close(c)
//	}
//}

type chanCloser struct {
	mu sync.Mutex
}

func (c *chanCloser) closeChan(ch chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-ch:
	default:
		close(ch)
	}
}
