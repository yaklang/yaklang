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
