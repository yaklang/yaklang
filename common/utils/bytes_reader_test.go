package utils

import (
	"github.com/akutz/memconn"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestReadConnWithTimeout(t *testing.T) {
	token := uuid.New().String()
	lis, err := memconn.Listen("memu", token)
	if err != nil {
		t.Logf("listen failed: %s", err)
		t.FailNow()
	}
	defer func() {
		_ = lis.Close()
	}()

	go func() {
		conn, err := lis.Accept()
		if err != nil {
			t.Logf("accept failed: %s", err)
			t.FailNow()
		}

		time.Sleep(500 * time.Millisecond)
		conn.Write([]byte("hello"))
	}()

	c, err := memconn.Dial("memu", token)
	if err != nil {
		t.Logf("failed dail memu abc: %s", err)
		t.FailNow()
	}

	data, err := ReadConnWithTimeout(c, 200*time.Millisecond)
	if err == nil {
		t.Logf("BUG: should not read data: %s", string(data))
		t.FailNow()
	}

	data, err = ReadConnWithTimeout(c, 500*time.Millisecond)
	if err != nil {
		t.Logf("BUG: should have read data: %s", err)
		t.FailNow()
	}

	if string(data) != "hello" {
		t.Logf("read data is not hello: %s", data)
		t.FailNow()
	}
}
