package sharpshooter

import (
	"fmt"
	"net"
	"testing"
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
