package sharpshooter

import (
	"fmt"
	"github.com/soyum2222/sharpshooter/protocol"
	"net"
	"testing"
	"time"
)

var client *Sniper
var server *Sniper

func makeSniper(t *testing.T) {

	go func() {
		conn, err := Dial("127.0.0.1:7842")
		if err != nil {
			fmt.Println(err)
			t.Fail()
		}
		client = conn
	}()

	l, err := Listen(&net.UDPAddr{
		Port: 7842,
	})
	if err != nil {
		fmt.Println(err)
		t.Fail()
	}
	conn, err := l.Accept()
	if err != nil {
		fmt.Println(err)
		t.Fail()
	}
	server = conn

}

func TestSendAndReceive(t *testing.T) {

	t.Run("make", makeSniper)
	go func() {
		_, err := client.Write([]byte("hello"))
		if err != nil {
			fmt.Println(err)
			t.Fail()
		}
		client.Close()
	}()

	b := make([]byte, 1024)
	n, err := server.Read(b)
	if err != nil {
		fmt.Println(err)
		t.Fail()
	}

	if string(b[:n]) != "hello" {
		fmt.Println(err)
		t.Fail()
	}

	server.Close()
}

func TestSniper_DeferSend(t *testing.T) {

	sn := NewSniper(nil, nil)

	go func() {

		for {

			time.Sleep(time.Second)
			sn.mu.Lock()
			fmt.Println(sn.sendCache)
			sn.sendCache = sn.sendCache[len(sn.sendCache):]
			sn.mu.Unlock()
			sn.writerBlocker.Pass()

		}

	}()

	sn.delaySend([]byte{uint8(1), uint8(2), uint8(3)})

	fmt.Println(sn.sendCache)

}

func TestWarp(t *testing.T) {

	sn := NewSniper(nil, nil)

	for i := 0; i < 1000; i++ {
		sn.delaySend([]byte{uint8(i)})
	}

	sn.packageSize = 10

	sn.wrap()

	fmt.Println(len(sn.ammoBag))
	if len(sn.ammoBag) != 100 {
		t.Fail()
	}
	for _, v := range sn.ammoBag {

		fmt.Println(v.Body)
	}

}

func TestBeShot(t *testing.T) {

	sn := NewSniper(nil, nil)

	for i := 0; i < 100000; i++ {

		ammo := protocol.Ammo{
			Id: uint32(i),
		}
		sn.beShot(&ammo)
	}
}

func BenchmarkBeshot(b *testing.B) {
	sn := NewSniper(nil, nil)

	go func() {

		b := make([]byte, 1<<20)
		for {
			sn.Read(b)
		}
	}()

	for i := 0; i < b.N; i++ {

		ammo := protocol.Ammo{
			Id: uint32(i),
		}
		sn.beShot(&ammo)
	}
}

func BenchmarkWrap(b *testing.B) {

	for i := 0; i < b.N; i++ {
		sn := NewSniper(nil, nil)

		b := make([]byte, 1<<20)

		sn.sendCache = b

		for k := range b {
			b[k] = uint8(k)
		}
		sn.maxWin = (1 << 20) / 1024

		sn.wrap()

	}

}

func BenchmarkShot(b *testing.B) {

	h := NewHeadquarters()
	var addr = net.UDPAddr{
		IP:   net.ParseIP("192.168.1.2"),
		Port: 9888,
		Zone: "",
	}

	conn, err := net.DialUDP("udp", nil, &addr)
	if err != nil {
		panic(err)
	}

	h.Snipers[addr.String()] = NewSniper(conn, &addr)

	bb := make([]byte, 1<<20)
	h.Snipers[addr.String()].timeoutTimer = time.NewTimer(1)
	h.Snipers[addr.String()].sendCache = bb
	for i := 0; i < b.N; i++ {

		h.Snipers[addr.String()].shoot()
	}
}
