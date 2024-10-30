package yakit

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func generateLargePayloadFile(lines int) (filename string, clean func(), err error) {
	fd, err := os.CreateTemp("", "large_payload_file")
	if err != nil {
		return "", nil, err
	}

	linesM := make([]string, 0, lines)
	for i := 0; i < lines; i++ {
		linesM = append(linesM, utils.RandAlphaNumStringBytes(16))
	}
	fd.WriteString(strings.Join(linesM, "\n"))

	return fd.Name(), func() {
		fd.Close()
		os.Remove(fd.Name())
	}, nil
}

func TestReadLargePayloadFileLineWithCallBack(t *testing.T) {
	t.Skip("skip this test because time consuming")

	filename, clean, err := generateLargePayloadFile(1e6)
	require.NoError(t, err)
	defer clean()

	rawBytes, err := os.ReadFile(filename)
	t.Logf("raw lines: \n%s", rawBytes)
	require.NoError(t, err)
	rawLines := strings.Split(strings.TrimSpace(string(rawBytes)), "\n")

	var lock sync.Mutex
	gotLines := make([]string, 0, len(rawLines))
	ReadLargeFileLineWithCallBack(context.Background(), filename, nil, func(s string) error {
		lock.Lock()
		gotLines = append(gotLines, strconv.Quote(s))
		lock.Unlock()
		return nil
	})
	t.Logf("got lines: \n%s", strings.Join(gotLines, "\n"))

	require.Lenf(t, gotLines, len(rawLines), "line count mismatch, got %d, want %d", len(gotLines), len(rawLines))
}

func TestReadLargePayloadFileLineWithCallBack2(t *testing.T) {
	t.Skip("skip this test because time consuming")

	fd, err := os.OpenFile("D:\\new.txt", os.O_CREATE|os.O_RDWR, 0o644)
	require.NoError(t, err)
	defer fd.Close()
	w := bufio.NewWriter(fd)
	defer w.Flush()

	ch := make(chan string, 128)
	once := utils.NewAtomicBool()
	go func(ch chan string) {
		ReadLargeFileLineWithCallBack(context.Background(), "D:\\rockyou.txt", nil, func(s string) error {
			ch <- strconv.Quote(s)
			return nil
		})
		close(ch)
	}(ch)

	for s := range ch {
		if once.SetToIf(false, true) {
			w.WriteString(s)
		} else {
			w.WriteString("\n" + s)
		}
	}
}
