package sharpshooter

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

func server() (net.Conn, net.Listener) {

	listen, err := Listen(":8080")
	if err != nil {
		panic(err)
	}

	conn, err := listen.Accept()
	if err != nil {
		panic(err)
	}
	return conn, listen
}

func dial() net.Conn {
	conn, err := Dial("127.0.0.1:8080")
	if err != nil {
		panic(err)
	}

	_, err = conn.Write([]byte("ping"))
	if err != nil {
		panic(err)
	}
	return conn
}

func TestDial(t *testing.T) {

	group := sync.WaitGroup{}
	group.Add(2)

	synchronization := make(chan struct{}, 1)
	server := func() {
		defer group.Done()
		synchronization <- struct{}{}
		conn, listen := server()
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()

		b := make([]byte, 4)

		_, err := conn.Read(b)
		if err != nil {
			t.Fail()
			panic(err)
		}

		if string(b) == "ping" {
			fmt.Println(string(b))
			return
		} else {
			t.Fail()
		}
	}

	dial := func() {
		defer group.Done()
		<-synchronization

		conn := dial()
		defer func() { _ = conn.Close() }()
		_, err := conn.Write([]byte("ping"))
		if err != nil {
			t.Fail()
			panic(err)
		}
	}

	go server()
	go dial()

	group.Wait()
}

func TestSniper_SetDeadline(t *testing.T) {

	group := sync.WaitGroup{}
	group.Add(2)

	synchronization := make(chan struct{}, 1)
	server := func() {
		defer group.Done()

		synchronization <- struct{}{}
		conn, listen := server()
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()
		err := conn.SetDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Fail()
			panic(err)
		}

		b := make([]byte, 1)
		_, err = conn.Read(b)
		if err != TIMEOUERROR {
			t.Fail()
		}
	}

	dial := func() {
		defer group.Done()

		<-synchronization
		conn := dial()
		defer func() { _ = conn.Close() }()
		time.Sleep(time.Second * 2)
		_, err := conn.Write([]byte("ping"))
		if err != CLOSEERROR {
			t.Fail()
		}
	}

	go server()
	go dial()

	group.Wait()
}

func TestSniper_SetReadDeadline(t *testing.T) {

	group := sync.WaitGroup{}
	group.Add(2)

	synchronization := make(chan struct{}, 1)
	server := func() {
		defer group.Done()

		synchronization <- struct{}{}
		conn, listen := server()
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()
		err := conn.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Fail()
			panic(err)
		}

		b := make([]byte, 1)
		_, err = conn.Read(b)
		if err != TIMEOUERROR {
			t.Fail()
		}
	}

	dial := func() {
		defer group.Done()

		<-synchronization
		conn := dial()
		defer func() { _ = conn.Close() }()
		time.Sleep(time.Second * 2)
		_, err := conn.Write([]byte("ping"))
		if err != CLOSEERROR {
			t.Fail()
		}
	}

	go server()
	go dial()

	group.Wait()
}
