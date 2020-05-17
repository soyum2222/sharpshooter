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
)

func TestShoot(t *testing.T) {

	go http.ListenAndServe(":8888", nil)
	addr1, err := net.ResolveUDPAddr("udp", "118.25.218.132:8890")
	if err != nil {
		panic(err)
	}

	conn, err := Dial(addr1, time.Time{})
	if err != nil {
		println(err)

	}
	for i := 0; i < 100; i++ {
		_, err := conn.Write([]byte("hello" + strconv.Itoa(i)))
		if err != nil {
			panic(err)
		}
	}
	time.Sleep(time.Hour)

}

func TestReceive(t *testing.T) {

	//conn, err := net.ListenUDP("udp", &net.UDPAddr{
	//	IP:   nil,
	//	Port: 8890,
	//	Zone: "",
	//})
	//
	//if err != nil {
	//	panic(err)
	//}
	//
	//h := headquarters{
	//	conn:      conn,
	//	Snipers:   map[string]*Sniper{},
	//	blocksign: make(chan struct{}, 0),
	//}
	//
	//go h.Monitor()
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
	for {
		n, err := sniper.Read(b)
		if err != nil {
			panic(err)
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

//func TestRand(t *testing.T) {
//
//	rand.Seed(time.Now().UnixNano())
//	for i:=0 ;i < 5000000 ; i++ {
//		nums := []int{}
//		find := func(num int)bool {
//			for _ , v := range nums{
//				if v == num {
//					return true
//				}
//			}
//			return false
//		}
//		for i:= 0 ; i < 6 ; i++{
//			for {
//				redNum := rand.Intn(33) + 1
//				if find(redNum) {
//					continue
//				}
//				nums = append(nums,redNum)
//				break
//			}
//		}
//		blueNum := rand.Intn(16)
//		nums = append(nums,blueNum +1)
//
//		log.Println(nums)
//	}
//}
