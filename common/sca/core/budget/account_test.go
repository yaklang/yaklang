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
