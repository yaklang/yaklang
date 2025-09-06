package hnsw

import (
	"io"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

func pbWriteVarint(w io.Writer, value uint64) error {
	var i []byte
	i = protowire.AppendVarint(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteBytes(w io.Writer, value []byte) error {
	var i []byte
	i = protowire.AppendBytes(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteUint32(w io.Writer, value uint32) error {
	var i []byte
	i = protowire.AppendFixed32(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteFloat32(w io.Writer, value float32) error {
	var i []byte
	i = protowire.AppendFixed32(i, math.Float32bits(value))
	_, err := w.Write(i)
	return err
}

func pbWriteFloat64(w io.Writer, value float64) error {
	var i []byte
	i = protowire.AppendFixed64(i, math.Float64bits(value))
	_, err := w.Write(i)
	return err
}

func pbWriteBool(w io.Writer, value bool) error {
	var i []byte
	if value {
		i = protowire.AppendVarint(i, uint64(1))
	} else {
		i = protowire.AppendVarint(i, uint64(0))
	}
	_, err := w.Write(i)
	return err
}
