package utils

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestBufPipeConcurrent(t *testing.T) {
	// 测试并发读写
	reader, writer := NewPipe()
	done := make(chan bool)
	content := []byte("test data")

	// 启动多个写协程
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := writer.Write(content)
			if err != nil {
				t.Errorf("Write error: %v", err)
			}
		}()
	}

	// 启动多个读协程
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, len(content))
			_, err := reader.Read(buf)
			if err != nil {
				t.Errorf("Read error: %v", err)
			}
			if !bytes.Equal(buf, content) {
				t.Errorf("Read content mismatch")
			}
		}()
	}

	// 等待所有协程完成
	go func() {
		wg.Wait()
		close(done)
	}()

	// 设置2秒超时
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out")
	}
}

func TestBufPipeReadBeforeWrite(t *testing.T) {
	reader, writer := NewPipe()
	done := make(chan bool)
	content := []byte("test data")

	// 先启动读协程
	go func() {
		buf := make([]byte, len(content))
		_, err := reader.Read(buf)
		if err != nil {
			t.Errorf("Read error: %v", err)
		}
		if !bytes.Equal(buf, content) {
			t.Errorf("Read content mismatch")
		}
		done <- true
	}()

	// 稍后启动写协程
	time.Sleep(100 * time.Millisecond)
	go func() {
		_, err := writer.Write(content)
		if err != nil {
			t.Errorf("Write error: %v", err)
		}
	}()

	// 设置2秒超时
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out")
	}
}

func TestBufPipeClose(t *testing.T) {
	reader, writer := NewPipe()
	done := make(chan bool)

	// 测试关闭写端
	go func() {
		err := writer.Close()
		if err != nil {
			t.Errorf("Close error: %v", err)
		}
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out")
	}

	// 测试关闭后读取
	buf := make([]byte, 10)
	_, err := reader.Read(buf)
	if err == nil {
		t.Error("Expected error after writer close")
	}
}

func TestBufPipeWithInitialData(t *testing.T) {
	initialData := []byte("initial data")
	reader, _ := NewBufPipe(initialData)
	done := make(chan bool)

	// 测试读取初始数据
	go func() {
		buf := make([]byte, len(initialData))
		_, err := reader.Read(buf)
		if err != nil {
			t.Errorf("Read error: %v", err)
		}
		if !bytes.Equal(buf, initialData) {
			t.Errorf("Read content mismatch")
		}
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out")
	}
}
