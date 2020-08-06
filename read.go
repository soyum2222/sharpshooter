package sharpshooter

func (s *Sniper) Read(b []byte) (n int, err error) {

loop:

	s.bemu.Lock()

	n += copy(b, s.rcvCache)
	s.rcvCache = s.rcvCache[n:]

	s.bemu.Unlock()

	if n <= 0 {
		select {

		case _, ok := <-s.readBlock:
			if !ok {
				return 0, CLOSEERROR
			}
			goto loop

		case _, _ = <-s.closeChan:
			return 0, CLOSEERROR

		case <-s.errorSign:
			return 0, s.errorContainer.Load().(error)

		}
	}
	return
}
