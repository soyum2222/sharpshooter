package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/soyum2222/sharpshooter"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"time"
)

var l int
var o string

var addr string
var i string

var c bool
var t bool

func main() {
	flag.IntVar(&l, "l", 0, "listen port")
	flag.BoolVar(&c, "c", false, "breakpoint continue")
	flag.StringVar(&o, "o", "", "output file path")
	flag.StringVar(&addr, "addr", "", "remote ip addr IP:PORT")
	flag.StringVar(&i, "i", "", "input file path")
	flag.BoolVar(&t, "t", false, "use tcp")
	flag.Parse()

	log.SetFlags(log.Lshortfile)
	go func() { http.ListenAndServe(":45671", nil) }()
	var conn net.Conn
	var err error
	if addr != "" {
		if t {
			conn, err = net.Dial("tcp", addr)
			if err != nil {
				log.Println(err)
				os.Exit(1)
			}
		} else {
			conn, err = sharpshooter.Dial(addr)
			if err != nil {
				log.Println(err)
				os.Exit(1)
			}
		}

	} else {
		var listen net.Listener
		if t {
			listen, err = net.Listen("tcp", ":"+strconv.Itoa(l))
			if err != nil {
				log.Println(err)
				os.Exit(1)
			}
		} else {
			listen, err = sharpshooter.Listen(":" + strconv.Itoa(l))
			if err != nil {
				log.Println(err)
				os.Exit(1)
			}
		}

		conn, err = listen.Accept()
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
	}

	if i != "" {
		send(conn, i)
	} else {
		receive(conn, o, c)
	}
}

func send(conn net.Conn, i string) {

	info, err := os.Stat(i)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	file, err := os.Open(i)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	offset := make([]byte, 8)
	_, err = io.ReadFull(conn, offset)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	remain := make([]byte, 8)
	off := int64(binary.BigEndian.Uint64(offset))
	binary.BigEndian.PutUint64(remain, uint64(info.Size()-off))

	_, err = conn.Write(remain)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	var totalSize int64
	totalSize = info.Size()

	_, err = file.Seek(off, 0)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	b := make([]byte, 1024*32)
	var size int64

	go func() {
		var old int64
		for {
			time.Sleep(time.Millisecond * 500)
			printBar(totalSize, size)
			fmt.Printf("%d KB/s \n", ((size-old)/1024)*2)
			old = size
		}
	}()

	for {
		n, err := file.Read(b)
		if err != nil && err != io.EOF {
			log.Println(err)
			break
		}

		if n == 0 {
			break
		}

		_, err = conn.Write(b[:n])
		if err != nil {
			log.Println(err)
			break
		}
		size += int64(n)
	}

	printBar(totalSize, size)

	_ = conn.Close()

	os.Exit(0)
}

func receive(conn net.Conn, o string, c bool) {

	var offset int64
	var exist bool
	var err error
	if c {
		info, err := os.Stat(o)
		if err == nil {
			exist = true
			offset = info.Size()
		}
	}

	var file *os.File
	off := make([]byte, 8)
	if exist {

		file, err = os.OpenFile(o, os.O_APPEND|os.O_WRONLY, 777)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		binary.BigEndian.PutUint64(off, uint64(offset))
		_, err = conn.Write(off)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}

	} else {

		file, err = os.Create(o)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}

		_, err = conn.Write(off)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
	}

	remain := make([]byte, 8)

	_, err = io.ReadFull(conn, remain)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	var total int64
	total = int64(binary.BigEndian.Uint64(remain)) + offset

	b := make([]byte, 1024*32)
	var size int64

	go func() {
		var old int64
		for {
			time.Sleep(time.Millisecond * 500)
			printBar(total, size)
			fmt.Printf("%d KB/s \n", ((size-old)/1024)*2)
			old = size
		}
	}()

	for {
		n, err := conn.Read(b)
		if err != nil {
			log.Println(err)
			break
		}

		if n == 0 {
			break
		}

		_, err = file.Write(b[:n])
		if err != nil {
			log.Println(err)
			break
		}

		size += int64(n)

	}

	printBar(total, size)
	conn.Close()
	os.Exit(0)

}

func progressBar(total, size int64) [100]bool {

	p := float64(size) / float64(total) * 100

	pro := [100]bool{}
	for i := 0; i < int(p); i++ {
		pro[i] = true
	}
	return pro
}

func printBar(total, size int64, ) {

	bar := progressBar(total, size)

	fmt.Print("[")
	n := 0
	for i := 0; i < 100; i++ {
		if bar[i] {
			fmt.Print("*")
			n++
		} else {
			fmt.Printf("=")
		}
	}

	fmt.Printf("]	%d %% \n", n)
	fmt.Printf("		total: %d byte		transfer: %d byte \n", total, size)
}
