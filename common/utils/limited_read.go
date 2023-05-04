package utils

import (
	"bufio"
	"context"
	"io"
	"sync"
	"time"
)

func ReadWithLen(r io.Reader, length int) ([]byte, int) {
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanBytes)

	var (
		output []byte
		count  int
	)
	for scanner.Scan() {
		count += 1
		output = append(output, scanner.Bytes()...)

		if count >= length {
			break
		}
	}

	return output, len(output)
}

func ReadWithContextTickCallback(ctx context.Context, rc io.Reader, callback func([]byte) bool, interval time.Duration) {
	scanner := bufio.NewScanner(rc)
	scanner.Split(bufio.ScanBytes)
	ticker := time.Tick(interval)

	// one go routine to read
	var (
		mux = new(sync.Mutex)
		buf []byte
	)
	go func() {
		for scanner.Scan() {
			// 根据上下文退出
			if ctx.Err() != nil {
				break
			}

			// 临时读一下现有指纹信息
			mux.Lock()
			buf = append(buf, scanner.Bytes()...)
			mux.Unlock()
		}
	}()

	defer callback(buf)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
			mux.Lock()
			flag := callback(buf)
			mux.Unlock()
			if flag {
				continue
			} else {
				return
			}
		}
	}
}

func ReadWithContextCallback(ctx context.Context, rc io.Reader, callback func([]byte)) {
	// full 4700 http matches : 1024 - 17s
	//                          2048 - 34s
	ReadWithContextCallbackWithMaxLength(ctx, rc, callback, 4096)
}

func ReadWithContextCallbackWithMaxLength(ctx context.Context, rc io.Reader, callback func([]byte), length int) {
	scanner := bufio.NewScanner(rc)
	scanner.Split(bufio.ScanBytes)

	ctx, cancel := context.WithCancel(ctx)

	// one go routine to read
	var (
		mux = new(sync.Mutex)
		buf []byte
	)
	go func() {
		defer cancel()

		for scanner.Scan() {
			// 根据上下文退出
			if ctx.Err() != nil {
				break
			}

			// 临时读一下现有指纹信息
			flag := false
			mux.Lock()
			buf = append(buf, scanner.Bytes()...)
			if len(buf) > length {
				cancel()
				flag = true
			}
			mux.Unlock()
			if flag {
				break
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			mux.Lock()
			callback(buf)
			mux.Unlock()
			return
		}
	}
}
