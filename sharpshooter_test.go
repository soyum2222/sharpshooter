package sharpshooter

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"testing"
	"time"
)

func server() (net.Conn, net.Listener) {

	listen, err := Listen(":9090")
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
	conn, err := Dial("127.0.0.1:9090")
	if err != nil {
		panic(err)
	}
	return conn
}

func TestDial(t *testing.T) {

	go func() { http.ListenAndServe(":11233", nil) }()
	group := sync.WaitGroup{}
	group.Add(2)

	synchronization := make(chan struct{}, 1)
	server := func() {
		defer group.Done()
		synchronization <- struct{}{}
		conn, listen := server()
		defer func() { _ = listen.Close() }()
		defer func() { fmt.Println("server close") }()
		defer func() { fmt.Println(conn.Close()) }()

		b := make([]byte, 4)

		_, err := conn.Read(b)
		if err != nil {
			t.Fail()
			panic(err)
		}
	}

	dial := func() {
		defer group.Done()
		<-synchronization

		conn := dial()
		defer func() { fmt.Println("client close") }()
		defer func() { fmt.Println(conn.Close()) }()
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

func TestSniper_Close(t *testing.T) {
	group := sync.WaitGroup{}
	group.Add(2)
	s := func() {
		defer group.Done()
		conn, listen := server()
		defer func() { fmt.Println("server close") }()
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()
		b := make([]byte, 1)
		_, err := conn.Read(b)
		if err != CLOSEERROR {
			t.Fail()
		}
	}

	d := func() {
		defer group.Done()
		conn := dial()
		_ = conn.Close()
		fmt.Println("client close")
	}

	go s()
	go d()

	group.Wait()
}

func TestSniper_Close2(t *testing.T) {
	group := sync.WaitGroup{}
	group.Add(2)

	s := func() {
		defer group.Done()
		conn, listen := server()
		defer func() { fmt.Println("server close") }()
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()
		b := make([]byte, 1)
		_, err := conn.Read(b)
		if err != nil {
			if err != CLOSEERROR {
				t.Fail()
				return
			}
		}
	}

	d := func() {
		defer group.Done()
		conn := dial()
		_, err := conn.Write([]byte("ping"))
		if err != nil {
			t.Fail()
			panic(err)
		}

		_ = conn.Close()
		fmt.Println("client close")
	}

	go s()
	go d()

	group.Wait()
}

func TestSniper_ClientClose(t *testing.T) {

	go func() { http.ListenAndServe(":9444", nil) }()
	group := sync.WaitGroup{}
	group.Add(2)

	s := func() {
		defer group.Done()
		conn, listen := server()
		defer func() { fmt.Println("server close") }()
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()
		b := make([]byte, 1)
		for {
			_, err := conn.Read(b)
			if err != nil {
				if err != CLOSEERROR {
					t.Fail()
					return
				} else {
					return
				}
			}
		}
	}

	d := func() {
		defer group.Done()
		conn := dial()
		_, err := conn.Write([]byte("ping"))
		if err != nil {
			t.Fail()
			panic(err)
		}

		err = conn.Close()
		if err != nil {
			panic(err)
		}
		fmt.Println("client close")
	}

	go s()
	go d()

	group.Wait()
}

func TestSniper_ServerClose(t *testing.T) {

	group := sync.WaitGroup{}
	group.Add(2)

	s := func() {
		defer group.Done()
		conn, listen := server()
		defer func() { _ = listen.Close() }()
		b := make([]byte, 1)
		for {
			_, err := conn.Read(b)
			if err != nil {
				if err != CLOSEERROR {
					t.Fail()
					return
				} else {
					return
				}
			}
			if b[0] == 'g' {
				fmt.Println("begin close")
				err = conn.Close()
				if err != nil {
					panic(err)
				}
				fmt.Println("server close")
				return
			}
		}
	}

	d := func() {
		defer group.Done()
		conn := dial()
		_, err := conn.Write([]byte("ping"))
		if err != nil {
			t.Fail()
			panic(err)
		}

		time.Sleep(time.Second)

		_, err = conn.Write([]byte("ping"))
		if err != nil && err != CLOSEERROR {
			t.Fail()
			return
		}
		fmt.Println("client return")
	}

	go s()
	go d()

	group.Wait()

}

func TestSniper_WriteCloseConn(t *testing.T) {

	group := sync.WaitGroup{}
	group.Add(2)

	s := func() {
		defer group.Done()
		conn, listen := server()
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()
		b := make([]byte, 4)
		for {
			_, err := conn.Read(b)
			if err != nil {
				if err != CLOSEERROR {
					t.Fail()
					return
				} else {
					return
				}
			}
		}
	}

	d := func() {
		defer group.Done()
		conn := dial()
		_, err := conn.Write([]byte("ping"))
		if err != nil {
			t.Fail()
			panic(err)
		}

		err = conn.Close()
		if err != nil {
			panic(err)
		}

		_, err = conn.Write([]byte("ping"))
		if err != nil {
			if err != CLOSEERROR {
				t.Fail()
				return
			}
		} else {
			t.Fail()
		}

	}

	go s()
	go d()

	group.Wait()
}

func TestSniper_ReadCloseConn(t *testing.T) {

	group := sync.WaitGroup{}
	group.Add(2)

	s := func() {
		defer group.Done()
		conn, listen := server()
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()
		b := make([]byte, 4)
		for {
			_, err := conn.Read(b)
			if err != nil {
				if err != CLOSEERROR {
					t.Fail()
					return
				} else {
					return
				}
			}

			_, _ = conn.Write([]byte("pong"))
		}
	}

	d := func() {
		defer group.Done()
		conn := dial()
		_, err := conn.Write([]byte("ping"))
		if err != nil {
			t.Fail()
			panic(err)
		}

		err = conn.Close()
		if err != nil {
			panic(err)
		}

		b := make([]byte, 4)

		for {
			_, err = conn.Read(b)
			if err != nil {
				if err != CLOSEERROR {
					t.Fail()
					return
				} else {
					return
				}
			} else if string(b) != "pong" {
				t.Fail()
			}
		}
	}

	go s()
	go d()

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
		fmt.Println("timeout")
	}

	dial := func() {
		defer group.Done()

		<-synchronization
		conn := dial()
		defer func() { _ = conn.Close() }()
		time.Sleep(time.Second * 3)
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

func TestSniper_SetWriteDeadline(t *testing.T) {

	group := sync.WaitGroup{}
	group.Add(2)

	synchronization := make(chan struct{}, 1)
	server := func() {
		defer group.Done()

		synchronization <- struct{}{}
		conn, listen := server()
		fmt.Println("accept")
		defer func() { _ = listen.Close() }()
		defer func() { _ = conn.Close() }()

		b := make([]byte, 10)
		fmt.Println("reading")
		_, err := conn.Read(b)
		if err != CLOSEERROR {
			fmt.Println("server fail ", err)
			t.Fail()
		}
	}

	dial := func() {
		defer group.Done()

		<-synchronization
		conn := dial()
		fmt.Println("dial")
		defer func() { _ = conn.Close() }()
		err := conn.SetWriteDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Fail()
			panic(err)
		}
		time.Sleep(time.Second * 2)
		fmt.Println("write")
		_, err = conn.Write([]byte("ping"))
		if err != TIMEOUERROR {
			fmt.Println("client fail ", err)
			t.Fail()
		}
	}

	go server()
	go dial()

	group.Wait()
}

func Test_CreateLargeConnection(t *testing.T) {

	go http.ListenAndServe(":7777", nil)
	group := sync.WaitGroup{}
	group.Add(2)

	server := func() {

		defer group.Done()

		listen, err := Listen(":9090")
		if err != nil {
			panic(err)
		}

		for i := 0; i < 1<<10; i++ {
			conn, err := listen.Accept()
			go func() {
				b := make([]byte, 10)
				if err != nil {
					panic(err)
				}

				defer conn.Close()

				n, err := conn.Read(b)
				if err != nil {
					t.Fail()
					panic(err)
				}
				fmt.Println(string(b[:n]))

			}()
		}
	}

	dial := func() {

		defer group.Done()

		for i := 0; i < 1<<10; i++ {

			go func() {
				conn, err := Dial("127.0.0.1:9090")
				if err != nil {
					panic(err)
				}

				_, err = conn.Write([]byte("ping"))
				if err != nil {
					t.Fail()
					panic(err)
				}
			}()
		}
	}

	go server()
	go dial()

	group.Wait()
}
