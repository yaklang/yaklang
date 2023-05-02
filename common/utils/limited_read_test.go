package utils

import (
	"context"
	"io"
	"testing"
	"time"
	"yaklang/common/log"
)

func TestReadWithContextTickCallback(t *testing.T) {
	r, w := io.Pipe()
	go func() {
		tick := time.Tick(200 * time.Millisecond)
		for {
			select {
			case <-tick:
				log.Info("tick a")
				_, err := w.Write([]byte("a"))
				if err != nil {
					panic(err)
				}
			}
		}
	}()

	var (
		aCount, count int
	)
	ctx, _ := context.WithTimeout(context.Background(), 700*time.Millisecond)
	ReadWithContextTickCallback(ctx, r, func(i []byte) bool {
		log.Infof("read tick with %s", string(i))
		count += 1
		aCount = len(i)
		return true
	}, 110*time.Millisecond)

	if count == 6 || count == 7 {
		if aCount != 3 {
			t.Errorf("len of bytes should be 3 but %v", aCount)
			t.FailNow()
		}
	} else {
		t.Errorf("count should be 6/7 but %v", count)
		t.FailNow()
	}
}

func TestReadWithContextTickCallback2(t *testing.T) {
	r, w := io.Pipe()
	go func() {
		tick := time.Tick(200 * time.Millisecond)
		for {
			select {
			case <-tick:
				log.Info("tick a")
				_, err := w.Write([]byte("a"))
				if err != nil {
					panic(err)
				}
			}
		}
	}()

	var (
		aCount, count int
	)
	ctx, _ := context.WithTimeout(context.Background(), 700*time.Millisecond)
	ReadWithContextTickCallback(ctx, r, func(i []byte) bool {
		log.Infof("read tick with %s", string(i))
		count += 1
		aCount = len(i)
		if aCount == 2 {
			return false
		}
		return true
	}, 110*time.Millisecond)

	if count == 3 || count == 4 {
		if aCount != 2 {
			t.Errorf("len of bytes should be 2 but %v", aCount)
			t.FailNow()
		}
	} else {
		t.Errorf("count should be 3/4 but %v", count)
		t.FailNow()
	}
}
