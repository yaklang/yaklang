//go:build !cgo

package compat

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
)

// RE2 keeps priority, as in YakRegexpUtils. Non-RE2 expressions use the portable
// regexp2 engine; PCRE2-only extensions still require a cgo build.
type YakRegexpUtils struct {
	expr string
	once sync.Once
	re   *regexp.Regexp
	re2  *regexp2.Regexp
	err  error
}

func NewYakRegexpUtils(expr string) *YakRegexpUtils { return &YakRegexpUtils{expr: expr} }

func (r *YakRegexpUtils) init() {
	r.once.Do(func() {
		r.re, r.err = regexp.Compile(r.expr)
		if r.err == nil {
			return
		}
		r.re2, r.err = regexp2.Compile(r.expr, regexp2.None)
		if r.re2 != nil {
			r.re2.MatchTimeout = 500 * time.Millisecond
		}
	})
}

func (r *YakRegexpUtils) CanUse() bool { r.init(); return r.err == nil }
func (r *YakRegexpUtils) Match(b []byte) (bool, error) {
	r.init()
	if r.err != nil {
		return false, r.err
	}
	if r.re != nil {
		return r.re.Match(b), nil
	}
	return r.re2.MatchString(string(b))
}

func InterfaceToBytes(v interface{}) []byte {
	switch x := v.(type) {
	case nil:
		return []byte{}
	case []byte:
		return x
	case string:
		return []byte(x)
	case float64:
		return []byte(strconv.FormatFloat(x, 'f', -1, 64))
	case float32:
		return []byte(strconv.FormatFloat(float64(x), 'f', -1, 32))
	case fmt.Stringer:
		return []byte(x.String())
	case error:
		return []byte(x.Error())
	default:
		if b, err := json.Marshal(v); err == nil {
			return b
		}
		return []byte(fmt.Sprint(v))
	}
}

func InterfaceToStringSlice(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	if s, ok := v.([]string); ok {
		return s
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return []string{string(InterfaceToBytes(v))}
	}
	out := make([]string, rv.Len())
	for i := range out {
		out[i] = string(InterfaceToBytes(rv.Index(i).Interface()))
	}
	return out
}
