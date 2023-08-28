package utils

import (
	"bufio"
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"io"
	"testing"
)

func TestHTTPRequestDumper_BodyIsLager(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if req.ContentLength == 3 {
		t.Fatal("ContentLength should be 10")
	}
}

func TestHTTPRequestDumper_BodyIsSmall(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 13` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if req.ContentLength == 13 {
		t.Fatal("ContentLength should be 10")
	}
}

func TestHTTPRequestDumper_Stream_BodyIsLager(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewBufferString(packet)))
	if err != nil {
		panic(err)
	}
	if req.ContentLength != 3 {
		t.Fatal("ContentLength should be 3")
	}
}

func TestHTTPRequestDumper_Stream_BodyIsSmall(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 13` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewBufferString(packet)))
	if err != nil {
		panic(err)
	}
	if req.ContentLength != 13 {
		t.Fatal("ContentLength should be 13")
	}
	raw, _ := io.ReadAll(req.Body)
	if string(raw) != "abccccddef   " && len(string(raw)) != 13 {
		spew.Dump(raw)
		t.Fatal("body should be abcccddef[SP][SP][SP]")
	}
}
