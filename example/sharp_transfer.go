package main

import (
	"encoding/binary"
	"flag"
	"github.com/soyum2222/sharpshooter"
	"io"
	"log"
	"net"
	"os"
	"strconv"
)

var l int
var o string

var addr string
var i string

var c bool

func main() {
	flag.IntVar(&l, "l", 0, "listen port")
	flag.BoolVar(&c, "c", false, "breakpoint continue")
	flag.StringVar(&o, "o", "", "output file path")
	flag.StringVar(&addr, "addr", "", "remote ip addr IP:PORT")
	flag.StringVar(&i, "i", "", "input file path")
	flag.Parse()

	var conn net.Conn
	var err error
	if addr != "" {
		conn, err = sharpshooter.Dial(addr)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
	} else {
		listen, err := sharpshooter.Listen(":" + strconv.Itoa(l))
		if err != nil {
			log.Println(err)
			os.Exit(1)
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

	_, err = file.Seek(off, 0)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	_, err = io.Copy(conn, file)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	_ = conn.Close()

	os.Exit(1)
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

	size := binary.BigEndian.Uint64(remain)

	n, err := io.Copy(file, conn)
	if err != nil && n != int64(size) {
		log.Println(err)
		os.Exit(1)
	}
}
