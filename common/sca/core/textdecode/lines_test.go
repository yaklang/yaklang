package textdecode

import (
	"bufio"
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"reflect"
	"strings"
	"testing"
)

func TestLinesMatchesScanLines(t *testing.T) {
	for _, input := range []string{"", "\n", "a\r\nb\n\n", "last\r", strings.Repeat("x", 16384) + "\nsmall\n", strings.Repeat("a\n", 10000)} {
		want := []string{}
		old := bufio.NewScanner(strings.NewReader(input))
		for old.Scan() {
			want = append(want, old.Text())
		}
		if old.Err() != nil {
			t.Fatal(old.Err())
		}
		got := []string{}
		s := NewLines(context.Background(), strings.NewReader(input))
		for s.Scan() {
			got = append(got, s.Text())
		}
		if s.Err() != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("len=%d got=%d want=%d err=%v", len(input), len(got), len(want), s.Err())
		}
	}
}
func TestLinesReserveBeforeReadAndGrowth(t *testing.T) {
	for _, limit := range []int64{1, 128} {
		r := &budgetReader{left: 8 << 20}
		s := NewLines(budget.Bind(context.Background(), budget.Limits{MaxResultBytes: limit}), r)
		if s.Scan() || !errors.Is(s.Err(), scanerr.ErrResourceLimit) || r.read != 0 {
			t.Fatalf("read %d err %v", r.read, s.Err())
		}
	}
	r := &budgetReader{left: 8 << 20}
	s := NewLines(budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 5000}), r)
	if s.Scan() || !errors.Is(s.Err(), scanerr.ErrResourceLimit) || r.read > 4096 {
		t.Fatalf("growth read %d err %v", r.read, s.Err())
	}
	s = NewLines(budget.Bind(context.Background(), budget.Limits{MaxFieldBytes: 32}), strings.NewReader(strings.Repeat("x", 33)))
	if s.Scan() || !errors.Is(s.Err(), scanerr.ErrResourceLimit) {
		t.Fatalf("line limit: %v", s.Err())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = &budgetReader{left: 100}
	s = NewLines(ctx, r)
	if s.Scan() || !errors.Is(s.Err(), context.Canceled) || r.read != 0 {
		t.Fatalf("cancel: %v", s.Err())
	}
}

func TestLinesTotalFileLimit(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := NewLines(budget.Bind(context.Background(), budget.Limits{MaxFileBytes: 6}), strings.NewReader(strings.Repeat("x\n", n)))
		lines := 0
		for s.Scan() {
			lines++
		}
		if lines != 3 || (n == 3 && s.Err() != nil) || (n == 4 && !errors.Is(s.Err(), scanerr.ErrResourceLimit)) {
			t.Fatalf("n=%d lines=%d err=%v", n, lines, s.Err())
		}
	}
}
