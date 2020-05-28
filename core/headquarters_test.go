package core

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strconv"
	"testing"
	"time"
	"os"
)

func TestShoot(t *testing.T) {

	go http.ListenAndServe(":8888", nil)
	addr1, err := net.ResolveUDPAddr("udp", "127.0.0.1:8890")
	//addr1, err := net.ResolveUDPAddr("udp", "118.25.218.132:8890")
	if err != nil {
		panic(err)
	}

	conn, err := Dial(addr1, time.Time{})
	if err != nil {
		println(err)
	}
	conn.OpenDeferSend()
	for i := 0; i < 10000; i++ {
		_, err := conn.Write([]byte("hello" + strconv.Itoa(i)))
		if err != nil {
			panic(err)
		}
	}
	conn.Close()

}

func TestReceive(t *testing.T) {

	go http.ListenAndServe(":9999", nil)

	addr := &net.UDPAddr{
		IP:   nil,
		Port: 8890,
		Zone: "",
	}
	h, err := License(addr)
	if err != nil {
		panic(err)
	}

	sniper := h.Accept()

	b := make([]byte, 1024)
	for i := 0; i < 10000; i++ {
		n, err := sniper.Read(b)
		if err != nil {
			fmt.Println(err)
			fmt.Println("return")
			os.Exit(0)
		}

		fmt.Println(string(b[:n]))
	}

}

func TestUdp(t *testing.T) {

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:18890")
	if err != nil {
		panic(err)

	}
	//addr1, _ := net.ResolveUDPAddr("udp", "127.0.0.1:1889")
	//if err != nil {
	//	panic(err)
	//
	//}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   nil,
		Port: 8890,
		Zone: "",
	})

	//raddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:28890")
	//if err != nil {
	//	panic(err)
	//}
	//
	//conn, err := net.DialUDP("udp", nil, addr)
	//if err != nil {
	//	panic(err)
	//}
	//conn, err := net.ListenUDP("udp", &net.UDPAddr{
	//	IP:   nil,
	//	Port: 50635,
	//	Zone: "",
	//})
	//if err != nil {
	//	panic(err)
	//}

	for {

		fmt.Println(conn.WriteTo([]byte("aaa"), addr))
		time.Sleep(time.Second)
	}

}
func TestReceiveUdp(t *testing.T) {

	//conn, err := net.ListenUDP("udp", &net.UDPAddr{
	//	IP:   nil,
	//	Port: 8890,
	//	Zone: "",
	//})
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:18890")
	if err != nil {
		panic(err)

	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}

	//if err != nil {
	//	panic(err)
	//}

	b := make([]byte, 1024)
	for {
		_, addr, err := conn.ReadFrom(b)
		if err != nil {
			panic(err)
		}
		fmt.Println(addr.String())

	}

}

func TestCopy(t *testing.T) {

	//b := make([]int, 10)

	boo := []int{0, 1, 2, 3, 4, 5}

	fmt.Println(boo[1:])
	//fmt.Println(copy(b, boo))
	//fmt.Println(b)
	//
	//copy(b, boo)
	//fmt.Println(b)
}

func TestSched(t *testing.T) {

	go func() {

		i := 0
		for i < 100 {
			fmt.Println(i)
			i++
			if i == 10 {
				runtime.Gosched()
			}
		}

	}()

	go func() {

		for i := 0; i < 20; i++ {
			fmt.Println("A", i)

		}
	}()

	time.Sleep(time.Hour)

}

func TestRand(t *testing.T) {

	conn, err := net.Dial("tcp", "118.25.218.132:1180")
	if err != nil {
		panic(err)
	}

	for i := 0; i < 10000; i++ {

		conn.Write([]byte(strconv.Itoa(i)))
	}

}

func TestTCPSend(t *testing.T) {

	conn, err := net.Dial("tcp", "127.0.0.1:9970")
	if err != nil {
		panic(err)
	}

	for i := 0; i < 100; i++ {
		go func(i int) {
			fmt.Println(i)
			_, err := conn.Write([]byte(strconv.Itoa(i)))
			if err != nil {
				panic(err)
			}
		}(i)
	}

	time.Sleep(time.Hour)
}

func TestTCPReceive(t *testing.T) {

	l, err := net.Listen("tcp", ":9970")
	if err != nil {
		panic(err)
	}
	conn, err := l.Accept()
	if err != nil {
		panic(err)
	}

	b := make([]byte, 1024)
	for {
		n, err := conn.Read(b)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(b[:n]))
		//if string(b[:n]) == "50" {
		//	conn.Close()
		//}
	}

}

func TestChan(t *testing.T) {
	c := make(chan int, 0)

	go func() {
		<-c
		fmt.Println(1)
	}()
	time.Sleep(time.Second)

	go func() {
		<-c
		fmt.Println(2)
	}()
	time.Sleep(time.Second)

	go func() {
		<-c
		fmt.Println(3)
	}()
	time.Sleep(time.Second)

	c <- 1
	c <- 1
	c <- 1

	time.Sleep(time.Second)
}
