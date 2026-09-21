package budget

import (
	"context"
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func TestZeroDefaultAndNegativeCharge(t *testing.T) {
	st := From(Bind(context.Background(), Limits{}))
	if st.Limits.MaxResultBytes != 256<<20 {
		t.Fatalf("Bind did not apply default MaxResultBytes: %d", st.Limits.MaxResultBytes)
	}
	unbound := &State{}
	if err := unbound.Add(1, 1); err != nil {
		t.Fatal(err)
	}
	if unbound.ResultBytes() != 1 {
		t.Fatalf("zero MaxResultBytes must use the 256MiB default, not unlimited skip")
	}
	if err := unbound.Add(-1, 0); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("negative objects: %v", err)
	}
}

func TestSizeAddRejectsNegativeAndOverflow(t *testing.T) {
	if _, err := SizeAdd(10, -1); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatal("negative part must not become a positive total")
	}
	if v := SizeOfSortIndex(int(^uint(0) >> 1)); v != -1 {
		t.Fatalf("sort index overflow sentinel: %d", v)
	} else if _, err := SizeAdd(v, 48); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatal("SizeOfSortIndex overflow sentinel must not add")
	}
	n, err := SizeAdd(8, 16, 24)
	if err != nil || n != 48 {
		t.Fatalf("add: %d %v", n, err)
	}
	if _, err = SizeAdd(1<<62, 1<<62); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("overflow: %v", err)
	}
	js, err := SizeOfJSONString("\x00\n\"")
	if err != nil || js < 2+6*3 {
		t.Fatalf("json string charge: %d %v", js, err)
	}
}

func TestSizeMulOverflow(t *testing.T) {
	if _, err := SizeMul(-1, 8); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatal("negative n")
	}
	if _, err := SizeMul(2, -1); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatal("negative unit")
	}
	n, err := SizeMul(0, 8)
	if err != nil || n != 0 {
		t.Fatalf("zero: %d %v", n, err)
	}
	n, err = SizeMul(3, 8)
	if err != nil || n != 24 {
		t.Fatalf("3*8: %d %v", n, err)
	}
	if _, err = SizeMul(4, 1<<62); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("overflow: %v", err)
	}
}

func TestOnceChargesSharedMaterialOnce(t *testing.T) {
	l, err := (Limits{MaxResultBytes: 4096}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	st := From(Bind(context.Background(), l))
	if err := st.Once("k", 1, 100); err != nil {
		t.Fatal(err)
	}
	before := st.ResultBytes()
	if err := st.Once("k", 1, 1000); err != nil {
		t.Fatal(err)
	}
	if st.ResultBytes() != before {
		t.Fatalf("shared id charged twice")
	}
	if err := st.Once("other", 1, 5000); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("oversize unique material: %v", err)
	}
	if !st.Exhausted() {
		t.Fatal("exhausted not sticky")
	}
}
