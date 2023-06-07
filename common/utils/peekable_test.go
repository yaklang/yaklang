package utils

import (
	"bufio"
	"net"
	"testing"
	"time"
)

func TestPeekableConn(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		s, err := l.Accept()
		if err != nil {
			t.Error(err)
			return
		}
		s.Write([]byte("hello"))
		defer s.Close()
	}()
	time.Sleep(time.Second) // Wait for server to start
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	peekable := NewPeekableNetConn(conn)
	by, err := peekable.Peek(2) // Peek 2 bytes
	if err != nil {
		t.Fatal(err)
	}
	if string(by) != "he" {
		t.Fatal("peek error")
	}

	by, err = peekable.Peek(3) // Peek 3 bytes
	if err != nil {
		t.Fatal(err)
	}
	if string(by) != "hel" {
		t.Fatal("peek error")
	}

	data := make([]byte, 1)
	n, err := peekable.Read(data) // Read 1 byte
	if err != nil || n != 1 {
		t.Fatal(err)
	}
	if string(data) != "h" {
		t.Fatal("read error")
	}

	buf := bufio.NewReader(peekable)
	data = make([]byte, 4)
	n, err = buf.Read(data) // Read 5 bytes from the bufreader
	if err != nil || n != 4 {
		t.Fatal(err)
	}
	if string(data) != "ello" {
		t.Fatal("buf read error")
	}
}
