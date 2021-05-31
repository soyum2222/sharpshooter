package sharpshooter

import (
	"time"
)

func (s *Sniper) Read(b []byte) (n int, err error) {

	var closeRet bool

	var tick *time.Ticker
	defer func() {
		if tick != nil {
			tick.Stop()
		}
	}()

	if len(b) == 0 {
		return 0, nil
	}

loop:

	now := time.Now()
	if (!s.readDeadline.IsZero() && s.readDeadline.Before(now)) || (!s.deadline.IsZero() && s.deadline.Before(now)) {
		return 0, TIMEOUERROR
	}

	s.bemu.Lock()

	n += copy(b, s.rcvCache)
	s.rcvCache = removeByte(s.rcvCache, n)

	s.bemu.Unlock()

	if n <= 0 {

	recheck:

		if !s.readDeadline.IsZero() || !s.deadline.IsZero() {

			var remainTime int64

			switch {

			case s.readDeadline.IsZero():
				remainTime = s.deadline.UnixNano() - time.Now().UnixNano()
			case s.deadline.IsZero():
				remainTime = s.readDeadline.UnixNano() - time.Now().UnixNano()
			default:
				if s.readDeadline.After(s.deadline) {
					remainTime = s.deadline.UnixNano() - time.Now().UnixNano()
				} else {
					remainTime = s.readDeadline.UnixNano() - time.Now().UnixNano()
				}
			}

			if remainTime <= 0 {
				remainTime = 1
			}

			if tick == nil {
				tick = time.NewTicker(time.Duration(remainTime) * time.Nanosecond)
			} else {
				tick.Reset(time.Duration(remainTime) * time.Nanosecond)
			}
		}

		if tick != nil {

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
			case <-tick.C:
				now := time.Now()
				if (!s.readDeadline.IsZero() && s.readDeadline.Before(now)) || (s.deadline.Before(now) && !s.deadline.IsZero()) {
					return 0, TIMEOUERROR
				}
			}
		} else {

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
			}
		}
		goto recheck
	}

	return
}
