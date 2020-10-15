package sharpshooter

import (
	"time"
)

func (s *Sniper) Read(b []byte) (n int, err error) {

	var closeRet bool

	if len(b) == 0 {
		return 0, nil
	}

loop:

	if (!s.readDeadline.IsZero() && s.readDeadline.Before(time.Now())) && (!s.deadline.IsZero() || s.deadline.Before(time.Now())) {
		return 0, TIMEOUERROR
	}

	s.bemu.Lock()

	n += copy(b, s.rcvCache)
	s.rcvCache = s.rcvCache[n:]

	s.bemu.Unlock()

	if n <= 0 {

	recheck:
		var tick <-chan time.Time
		if !s.readDeadline.IsZero() || !s.deadline.IsZero() {
			if s.readDeadline.After(s.deadline) {
				tick = time.Tick(time.Duration(s.readDeadline.UnixNano()-time.Now().UnixNano()) * time.Nanosecond)
			} else {
				tick = time.Tick(time.Duration(s.deadline.UnixNano()-time.Now().UnixNano()) * time.Nanosecond)
			}
		} else {
			tick = time.Tick(time.Second)
		}

		select {
		case _, ok := <-s.readBlock:
			if !ok {
				return 0, CLOSEERROR
			}
			goto loop

		case _, _ = <-s.closeChan:
			if closeRet {
				return 0, CLOSEERROR
			}
			closeRet = true
			goto loop

		case <-s.errorSign:
			return 0, s.errorContainer.Load().(error)
		case <-tick:
			if (!s.readDeadline.IsZero() && s.readDeadline.Before(time.Now())) || (s.deadline.Before(time.Now()) && !s.deadline.IsZero()) {
				return 0, TIMEOUERROR
			}
		}
		goto recheck
	}

	return
}
