package utils

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
)

func GzipCompress(i interface{}) ([]byte, error) {
	var buf bytes.Buffer
	var w = gzip.NewWriter(&buf)
	switch ret := i.(type) {
	case io.Reader:
		_, err := io.Copy(w, ret)
		w.Close()
		if err != nil && err != io.EOF {
			return buf.Bytes(), err
		}
		return buf.Bytes(), nil
	default:
		_, err := w.Write(InterfaceToBytes(ret))
		if err != nil {
			return nil, err
		}
		w.Flush()
		w.Close()
		return buf.Bytes(), nil
	}
}

func ZlibCompress(i interface{}) ([]byte, error) {
	var buf bytes.Buffer
	var w = zlib.NewWriter(&buf)
	switch ret := i.(type) {
	case io.Reader:
		_, err := io.Copy(w, ret)
		w.Close()
		if err != nil && err != io.EOF {
			return buf.Bytes(), err
		}
		return buf.Bytes(), nil
	default:
		_, err := w.Write(InterfaceToBytes(ret))
		if err != nil {
			return nil, err
		}
		w.Flush()
		w.Close()
		return buf.Bytes(), nil
	}
}

func GzipDeCompress(ret []byte) ([]byte, error) {
	var reader *gzip.Reader
	var err error
	reader, err = gzip.NewReader(bytes.NewBuffer(ret))
	if err != nil {
		return nil, Errorf("create gzip reader failed: %s", err)
	}
	var bufBytes bytes.Buffer
	_, err = io.Copy(&bufBytes, reader)
	reader.Close()
	if err != nil {
		return bufBytes.Bytes(), Errorf("ungzip failed: %s", err)
	}
	return bufBytes.Bytes(), nil
}

func ZlibDeCompress(ret []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewBuffer(ret))
	if err != nil {
		return nil, Errorf("create gzip reader failed: %s", err)
	}
	var bufBytes bytes.Buffer
	_, err = io.Copy(&bufBytes, reader)
	reader.Close()
	if err != nil {
		return bufBytes.Bytes(), Errorf("ungzip failed: %s", err)
	}
	return bufBytes.Bytes(), nil
}

func IsGzipBytes(i interface{}) bool {
	switch ret := i.(type) {
	case io.Reader:
		_, err := gzip.NewReader(ret)
		if err != nil {
			return false
		}
		return true
	default:
		var buf = bytes.NewBuffer(InterfaceToBytes(i))
		return IsGzipBytes(buf)
	}
}
