package parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
)

type runtimeIsolationBoundedReader struct {
	*bytes.Reader
}

func (r *runtimeIsolationBoundedReader) InputBitLength() uint64 {
	return uint64(r.Len() * 8)
}

func TestParseBinaryParallelRuntimeIsolation(t *testing.T) {
	messages := [][]byte{
		newRuntimeIsolationNTPMessage(0x23, 0x10), // LI=0, version=4, mode=3
		newRuntimeIsolationNTPMessage(0xdc, 0x90), // LI=3, version=3, mode=4
	}

	const workers = 24
	const iterations = 12
	runParallelParserChecks(t, workers, func(worker int) error {
		for iteration := 0; iteration < iterations; iteration++ {
			wire := messages[(worker+iteration)%len(messages)]
			if err := parseAndValidateRuntimeIsolationNTP(wire); err != nil {
				return fmt.Errorf("iteration %d: %w", iteration, err)
			}
		}
		return nil
	})
}

func TestGenerateBinaryParallelNestedImportIsolation(t *testing.T) {
	type generationCase struct {
		data map[string]any
		wire []byte
	}
	cases := []generationCase{
		{
			data: map[string]any{
				"Flags And Version": 0, "Protocol Type": 0x880b,
				"Payload": map[string]any{
					"PPP": map[string]any{
						"Address": 0xff, "Control": 0x03, "Protocol": 0xc023,
						"PAP": map[string]any{
							"Code": 1, "Identifier": 0, "Length": 14,
							"Request": map[string]any{
								"Peer ID Length": 4, "Peer ID": "ixia",
								"Password Length": 4, "Password": "ixia",
							},
						},
					},
				},
			},
			wire: []byte{0x00, 0x00, 0x88, 0x0b, 0xff, 0x03, 0xc0, 0x23, 0x01, 0x00, 0x00, 0x0e, 0x04, 'i', 'x', 'i', 'a', 0x04, 'i', 'x', 'i', 'a'},
		},
		{
			data: map[string]any{
				"Flags And Version": 0, "Protocol Type": 0x880b,
				"Payload": map[string]any{
					"PPP": map[string]any{
						"Address": 0xff, "Control": 0x03, "Protocol": 0xc023,
						"PAP": map[string]any{
							"Code": 2, "Identifier": 1, "Length": 9,
							"Response": map[string]any{"Message Length": 4, "Message": "good"},
						},
					},
				},
			},
			wire: []byte{0x00, 0x00, 0x88, 0x0b, 0xff, 0x03, 0xc0, 0x23, 0x02, 0x01, 0x00, 0x09, 0x04, 'g', 'o', 'o', 'd'},
		},
	}

	const workers = 16
	const iterations = 8
	runParallelParserChecks(t, workers, func(worker int) error {
		for iteration := 0; iteration < iterations; iteration++ {
			item := cases[(worker+iteration)%len(cases)]
			node, err := GenerateBinary(item.data, "generic_routing_encapsulation", "GRE")
			if err != nil {
				return fmt.Errorf("iteration %d generate: %w", iteration, err)
			}
			if _, err := node.Result(); err != nil {
				return fmt.Errorf("iteration %d result: %w", iteration, err)
			}
			buffer, ok := node.Ctx.GetItem("buffer").(*bytes.Buffer)
			if !ok {
				return fmt.Errorf("iteration %d generated tree has no byte buffer", iteration)
			}
			if got := buffer.Bytes(); !bytes.Equal(got, item.wire) {
				return fmt.Errorf("iteration %d generated %x, want %x", iteration, got, item.wire)
			}
		}
		return nil
	})
}

func runParallelParserChecks(t *testing.T, workers int, check func(worker int) error) {
	t.Helper()
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		worker := worker
		go func() {
			defer wait.Done()
			<-start
			if err := check(worker); err != nil {
				errs <- fmt.Errorf("worker %d: %w", worker, err)
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func newRuntimeIsolationNTPMessage(header byte, seed byte) []byte {
	wire := make([]byte, 48)
	wire[0] = header
	for index := 1; index < len(wire); index++ {
		wire[index] = seed + byte(index*3)
	}
	return wire
}

func parseAndValidateRuntimeIsolationNTP(wire []byte) error {
	reader := &runtimeIsolationBoundedReader{Reader: bytes.NewReader(wire)}
	node, err := ParseBinary(reader, "application-layer.ntp", "NTP")
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if reader.Len() != 0 {
		return fmt.Errorf("left %d of %d bytes unread", reader.Len(), len(wire))
	}
	value, err := node.Result()
	if err != nil {
		return fmt.Errorf("result: %w", err)
	}
	wants := map[string]any{
		"Leap Indicator":      uint64(wire[0] >> 6),
		"Version":             uint64((wire[0] >> 3) & 0x07),
		"Mode":                uint64(wire[0] & 0x07),
		"Stratum":             uint64(wire[1]),
		"Poll":                int64(int8(wire[2])),
		"Precision":           int64(int8(wire[3])),
		"Root Delay":          uint64(binary.BigEndian.Uint32(wire[4:8])),
		"Root Dispersion":     uint64(binary.BigEndian.Uint32(wire[8:12])),
		"Reference ID":        append([]byte(nil), wire[12:16]...),
		"Reference Timestamp": append([]byte(nil), wire[16:24]...),
		"Origin Timestamp":    append([]byte(nil), wire[24:32]...),
		"Receive Timestamp":   append([]byte(nil), wire[32:40]...),
		"Transmit Timestamp":  append([]byte(nil), wire[40:48]...),
	}
	if len(value.Children()) != len(wants) {
		return fmt.Errorf("result has %d fields, want %d", len(value.Children()), len(wants))
	}
	for name, want := range wants {
		child := value.Child(name)
		if child == nil {
			return fmt.Errorf("result is missing %q", name)
		}
		if !runtimeIsolationValuesEqual(child.Value, want) {
			return fmt.Errorf("field %q=%#v, want %#v", name, child.Value, want)
		}
	}
	return nil
}

func runtimeIsolationValuesEqual(got, want any) bool {
	switch want := want.(type) {
	case uint64:
		got, ok := runtimeIsolationUint64(got)
		return ok && got == want
	case int64:
		got, ok := runtimeIsolationInt64(got)
		return ok && got == want
	case []byte:
		got, ok := got.([]byte)
		return ok && bytes.Equal(got, want)
	default:
		return false
	}
}

func runtimeIsolationUint64(value any) (uint64, bool) {
	switch value := value.(type) {
	case uint:
		return uint64(value), true
	case uint8:
		return uint64(value), true
	case uint16:
		return uint64(value), true
	case uint32:
		return uint64(value), true
	case uint64:
		return value, true
	default:
		return 0, false
	}
}

func runtimeIsolationInt64(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	default:
		return 0, false
	}
}
