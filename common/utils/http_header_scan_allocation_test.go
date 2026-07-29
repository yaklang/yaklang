package utils

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/pkg/errors"
)

func TestScanHTTPHeaderWithHeaderFoldingRetainedSlices(t *testing.T) {
	packet := []byte("X-One: first\r\n second\r\nX-Two: value\r\n\r\nbody")
	reader := bufio.NewReader(bytes.NewReader(packet))
	var headers [][]byte
	if err := ScanHTTPHeaderWithHeaderFolding(reader, func(raw []byte) {
		headers = append(headers, raw)
	}, nil); err != nil {
		t.Fatalf("scan headers: %v", err)
	}

	want := [][]byte{
		[]byte("X-One: first\r\n second"),
		[]byte("X-Two: value"),
		nil,
	}
	if len(headers) != len(want) {
		t.Fatalf("header count: got %d want %d", len(headers), len(want))
	}
	for index := range want {
		if !bytes.Equal(headers[index], want[index]) {
			t.Fatalf("header %d: got %q want %q", index, headers[index], want[index])
		}
	}

	copy(packet, bytes.Repeat([]byte{'x'}, len(packet)))
	for index := range want {
		if !bytes.Equal(headers[index], want[index]) {
			t.Fatalf("retained header %d changed with input: got %q want %q", index, headers[index], want[index])
		}
	}
}

func scanHTTPHeaderWithHeaderFoldingLegacy(
	reader io.Reader,
	headerCallback func(rawHeader []byte),
	prefix []byte,
) error {
	var headerRawCache []byte
	currentState := CommonHeaderStat
	headerFoldingPrefix := make([]byte, 0)

	pushHeaderRawData := func(raw []byte) {
		headerRawCache = append(headerRawCache, raw...)
	}
	emitHeaderRaw := func() {
		if headerCallback != nil {
			headerCallback(headerRawCache)
		}
		headerRawCache = make([]byte, 0)
	}
	defer emitHeaderRaw()

	trimPrefix := func(raw []byte) []byte {
		minLen := Min(len(prefix), len(raw))
		index := 0
		for ; index < minLen; index++ {
			if raw[index] != prefix[index] {
				break
			}
		}
		return raw[index:]
	}

	for {
		lineBytes, err := ReadLine(reader)
		if err != nil && err != io.EOF {
			return errors.Wrap(err, "read HTTPResponse header failed")
		}
		lineBytes = trimPrefix(lineBytes)
	Retry:
		switch currentState {
		case CommonHeaderStat:
			if len(lineBytes) == 0 {
				return nil
			}
			for index, value := range lineBytes {
				if value != ' ' && value != '\t' {
					headerFoldingPrefix = lineBytes[:index]
					break
				}
			}
			pushHeaderRawData(lineBytes)
			currentState = HeaderCheckStat
		case HeaderCheckStat:
			checkLine := bytes.TrimPrefix(lineBytes, headerFoldingPrefix)
			if len(checkLine) > 0 && (checkLine[0] == ' ' || checkLine[0] == '\t') {
				pushHeaderRawData(append([]byte(CRLF), checkLine...))
			} else {
				emitHeaderRaw()
				currentState = CommonHeaderStat
				goto Retry
			}
		}
	}
}

func collectScannedHTTPHeaders(
	scan func(io.Reader, func([]byte), []byte) error,
	packet []byte,
	prefix []byte,
) ([][]byte, error) {
	var headers [][]byte
	err := scan(bufio.NewReader(bytes.NewReader(packet)), func(raw []byte) {
		if raw == nil {
			headers = append(headers, nil)
			return
		}
		headers = append(headers, append([]byte{}, raw...))
	}, prefix)
	return headers, err
}

func FuzzScanHTTPHeaderWithHeaderFoldingMatchesLegacy(f *testing.F) {
	for _, seed := range []struct {
		packet string
		prefix string
	}{
		{packet: "Content-Type: text/plain\r\nContent-Length: 7\r\n\r\npayload"},
		{packet: "X-Folded: first\r\n second\r\nContent-Length: 7\r\n\r\npayload"},
		{packet: "  X-Prefixed: value\r\n  \r\n", prefix: "  "},
		{packet: "X-LF: value\n\n"},
		{packet: ""},
	} {
		f.Add([]byte(seed.packet), []byte(seed.prefix))
	}

	f.Fuzz(func(t *testing.T, packet, prefix []byte) {
		if len(packet) > 64*1024 || len(prefix) > 256 {
			t.Skip()
		}
		want, wantErr := collectScannedHTTPHeaders(scanHTTPHeaderWithHeaderFoldingLegacy, packet, prefix)
		got, gotErr := collectScannedHTTPHeaders(ScanHTTPHeaderWithHeaderFolding, packet, prefix)
		if (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("error mismatch: got %v want %v", gotErr, wantErr)
		}
		if len(got) != len(want) {
			t.Fatalf("header count: got %d want %d", len(got), len(want))
		}
		for index := range want {
			if (got[index] == nil) != (want[index] == nil) || !bytes.Equal(got[index], want[index]) {
				t.Fatalf("header %d: got %#v want %#v", index, got[index], want[index])
			}
		}
	})
}

var benchmarkScannedHTTPHeaderBytes int

func BenchmarkScanHTTPHeaderWithHeaderFolding(b *testing.B) {
	for _, test := range []struct {
		name   string
		packet []byte
	}{
		{
			name: "canonical",
			packet: []byte(
				"Date: today\r\n" +
					"Content-Type: text/plain; charset=utf-8\r\n" +
					"Content-Length: 4096\r\n" +
					"Cache-Control: no-cache\r\n" +
					"Connection: keep-alive\r\n\r\n",
			),
		},
		{
			name: "folded",
			packet: []byte(
				"X-Folded: first\r\n second\r\n" +
					"Content-Type: text/plain\r\n" +
					"Content-Length: 4096\r\n\r\n",
			),
		},
	} {
		b.Run(test.name, func(b *testing.B) {
			packetReader := bytes.NewReader(test.packet)
			reader := bufio.NewReader(packetReader)
			callback := func(raw []byte) {
				benchmarkScannedHTTPHeaderBytes += len(raw)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				packetReader.Reset(test.packet)
				reader.Reset(packetReader)
				if err := ScanHTTPHeaderWithHeaderFolding(reader, callback, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
