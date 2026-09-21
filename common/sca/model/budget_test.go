package model

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
)

func TestNormalizeReservationPrecedesMutation(t *testing.T) {
	input := Report{Components: []Component{{Key: ComponentKey{Name: "two"}}, {Key: ComponentKey{Name: "one"}}}, Observations: []Observation{{NativeID: "one"}, {NativeID: "one"}}, Complete: true}
	before := input
	st := budget.From(budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 1}))
	if err := input.NormalizeBudget(st); err == nil {
		t.Fatal("accepted insufficient memory")
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("mutated before complete reservation")
	}
	want := before
	want.Normalize()
	if err := input.NormalizeBudget(budget.From(budget.Ensure(context.Background()))); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, want) {
		t.Fatal("budget path changed normalization semantics")
	}
}
