// Package testcheck provides standard-library-only assertions for preserved
// upstream test contracts. Production SCA packages do not import this package.
package testcheck

import (
	"reflect"
	"strings"
	"testing"
)

func Equal(t testing.TB, want, got any, _ ...any) bool {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("got %#v; want %#v", got, want)
		return false
	}
	return true
}
func NoError(t testing.TB, err error, _ ...any) bool {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
		return false
	}
	return true
}

type ErrorAssertionFunc func(testing.TB, error, ...any) bool

func Error(t testing.TB, err error, _ ...any) bool {
	t.Helper()
	if err == nil {
		t.Error("expected error")
		return false
	}
	return true
}
func Equalf(t testing.TB, want, got any, _ string, _ ...any) bool { return Equal(t, want, got) }
func ErrorContains(t testing.TB, err error, substring string, _ ...any) bool {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substring) {
		t.Errorf("error %v does not contain %q", err, substring)
		return false
	}
	return true
}
func Contains(t testing.TB, value any, item any, _ ...any) bool {
	t.Helper()
	found := false
	switch v := value.(type) {
	case string:
		needle, ok := item.(string)
		found = ok && strings.Contains(v, needle)
	default:
		r := reflect.ValueOf(value)
		if r.Kind() == reflect.Slice || r.Kind() == reflect.Array {
			for i := 0; i < r.Len(); i++ {
				if reflect.DeepEqual(r.Index(i).Interface(), item) {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Errorf("%#v does not contain %#v", value, item)
	}
	return found
}
func NotNil(t testing.TB, value any, _ ...any) bool {
	t.Helper()
	r := reflect.ValueOf(value)
	if !r.IsValid() || ((r.Kind() == reflect.Ptr || r.Kind() == reflect.Slice || r.Kind() == reflect.Map || r.Kind() == reflect.Interface) && r.IsNil()) {
		t.Error("unexpected nil")
		return false
	}
	return true
}
