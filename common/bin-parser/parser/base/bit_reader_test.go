package base

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

func TestBitReader(t *testing.T) {
	reader := NewBitReader(bytes.NewReader([]byte("hello world")))
	err := reader.Backup()
	if err != nil {
		t.Fatal(err)
	}
	res, err := reader.ReadBits(40)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "hello" {
		t.Fatal("read bits error")
	}
	err = reader.Recovery()
	if err != nil {
		t.Fatal(err)
	}
	res, err = reader.ReadBits(88)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "hello world" {
		t.Fatal("read bits error")
	}
}
func TestMultiReader(t *testing.T) {
	reader1 := bytes.NewReader([]byte("hello"))
	reader2 := bytes.NewReader([]byte(" world"))
	multiReader := NewConcatReader(reader1, reader2)

	buf := make([]byte, 20)
	n, err := multiReader.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 11, n)
	assert.Equal(t, "hello world", string(buf[:n]))
	n, err = multiReader.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}
func TestBackup(t *testing.T) {
	reader := NewBitReader(bytes.NewReader([]byte("hello world")))
	err := reader.Backup()
	if err != nil {
		t.Fatal(err)
	}
	res, err := reader.ReadBits(40)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "hello" {
		t.Fatal("read bits error")
	}
	err = reader.Recovery()
	if err != nil {
		t.Fatal(err)
	}
	res, err = reader.ReadBits(88)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "hello world" {
		t.Fatal("read bits error")
	}
}
