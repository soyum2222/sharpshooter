package sharpshooter

func closeChan(c chan struct{}) {

	select {
	case <-c:
	default:
		close(c)
	}
}
