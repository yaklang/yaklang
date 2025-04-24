package fuzztagx

import (
	"bytes"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/utils"
)

func TestReader(t *testing.T) {
	gener, err := NewTagReader("aaa\n{{sleep(1)}}sdfa", map[string]*parser.TagMethod{
		"sleep": {
			Fun: func(s string) ([]*parser.FuzzResult, error) {
				sleepTime, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return nil, err
				}
				return []*parser.FuzzResult{parser.NewFuzzResultWithData(func() ([]byte, error) {
					time.Sleep(utils.FloatSecondDuration(sleepTime))
					return nil, nil
				})}, nil
			},
		},
	}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	gener.Next()
	reader, err := gener.Result()
	if err != nil {
		t.Fatal(err)
	}
	calcDu := func(f func()) time.Duration {
		start := time.Now()
		f()
		return time.Now().Sub(start)
	}
	du := calcDu(func() {
		buf := make([]byte, 3)
		n, err := reader.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, 3, n)
		assert.Equal(t, "aaa", string(buf))
	})
	assert.Equal(t, 0, int(du.Seconds()))
	du = calcDu(func() {
		buf := make([]byte, 4)
		n, err := reader.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, 4, n)
		assert.Equal(t, "\nsdf", string(buf))
	})
	assert.Equal(t, 1, int(du.Seconds()))
	du = calcDu(func() {
		buf := make([]byte, 2)
		n, err := reader.Read(buf)
		assert.Equal(t, 1, n)
		assert.Equal(t, "a", string(buf[:n]))
		assert.Equal(t, io.EOF, err)
	})
	assert.Equal(t, 0, int(du.Seconds()))

	reader, err = gener.Result()
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := utils.RealTimeCopy(buf, reader)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, int64(8), n)
	}()
	time.Sleep(time.Millisecond * 500)
	assert.Equal(t, "aaa\n", buf.String())
	time.Sleep(time.Second)
	assert.Equal(t, "aaa\nsdfa", buf.String())
	wg.Wait()
}
