package fuzztagx

import (
	"errors"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"io"
)

type ReaderGenerator struct {
	generator *parser.Generator
}

func (r *ReaderGenerator) Next() bool {
	return r.generator.Next()
}
func (r *ReaderGenerator) Result() (io.Reader, error) {
	rawRes := r.generator.RawResult()
	reader := &TagReader{
		data: []any{},
	}
	for _, result := range rawRes {
		switch ret := result.Data.(type) {
		case []byte, stepDataGetter, string, func() ([]byte, error):
			reader.data = append(reader.data, ret)
		default:
			return nil, errors.New("unsupported data type")
		}
	}
	return reader, nil
}

func NewTagReader(code string, table map[string]*parser.TagMethod, isSimple, syncTag bool) (*ReaderGenerator, error) {
	reader := &ReaderGenerator{}
	gen, err := NewGenerator(code, table, isSimple, syncTag)
	if err != nil {
		return nil, err
	}
	reader.generator = gen
	return reader, nil
}

type TagReader struct {
	data    []any
	current int
	preData []byte
}

func (t *TagReader) Read(p []byte) (n int, err error) {
	l := len(p)
	for {
		if len(t.preData) >= l {
			copy(p[n:], t.preData[:l])
			n += l
			t.preData = t.preData[l:]
			return n, nil
		}
		copy(p[n:n+len(t.preData)], t.preData)
		l -= len(t.preData)
		n += len(t.preData)
		if t.current >= len(t.data) {
			break
		}
		switch ret := t.data[t.current].(type) {
		case string:
			t.preData = []byte(ret)
		case []byte:
			t.preData = ret
		case func() ([]byte, error):
			res, err := ret()
			if err != nil {
				return 0, err
			}
			t.preData = res
		case stepDataGetter:
			res, err := ret()
			if err != nil {
				return 0, err
			}
			t.preData = res
		default:
			return 0, errors.New("unsupported data type")
		}
		t.current++
	}
	return n, io.EOF
}
