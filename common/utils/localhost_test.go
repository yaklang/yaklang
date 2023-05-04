package utils

import (
	"net"
	"testing"
)

func TestLoopback1(t *testing.T) {
	lis1, err := net.Listen("tcp", "127.0.0.1:8881")
	if err != nil {
		panic(err)
	}
	defer lis1.Close()
	lis2, err := net.Listen("tcp", "127.0.0.2:8882")
	if err != nil {
		panic(err)
	}
	lis2.Close()

}
