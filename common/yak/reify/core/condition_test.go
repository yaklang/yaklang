package core

import "testing"

// TestParseConditionCoversEveryFamily checks the mnemonics each loader prints.
// The x86 cases use the byte condition codes, the AArch64 cases the same
// comparison under its own spelling, and the s390x cases the names its mask
// decoder returns.
func TestParseConditionCoversEveryFamily(t *testing.T) {
	cases := []struct {
		name   string
		family string
		want   Condition
	}{
		{"e", "x86", Condition{Op: CompareEQ}},
		{"z", "x86", Condition{Op: CompareEQ}},
		{"ne", "x86", Condition{Op: CompareNE}},
		{"nz", "x86", Condition{Op: CompareNE}},
		{"l", "x86", Condition{Op: CompareLT, Signed: true}},
		{"nge", "x86", Condition{Op: CompareLT, Signed: true}},
		{"le", "x86", Condition{Op: CompareLE, Signed: true}},
		{"g", "x86", Condition{Op: CompareGT, Signed: true}},
		{"nle", "x86", Condition{Op: CompareGT, Signed: true}},
		{"ge", "x86", Condition{Op: CompareGE, Signed: true}},
		{"b", "x86", Condition{Op: CompareLT}},
		{"c", "x86", Condition{Op: CompareLT}},
		{"nae", "x86", Condition{Op: CompareLT}},
		{"be", "x86", Condition{Op: CompareLE}},
		{"na", "x86", Condition{Op: CompareLE}},
		{"a", "x86", Condition{Op: CompareGT}},
		{"nbe", "x86", Condition{Op: CompareGT}},
		{"ae", "x86", Condition{Op: CompareGE}},
		{"nb", "x86", Condition{Op: CompareGE}},
		{"nc", "x86", Condition{Op: CompareGE}},
		{"s", "x86", Condition{Op: CompareLT, Signed: true}},
		{"ns", "x86", Condition{Op: CompareGE, Signed: true}},

		{"eq", "aarch64", Condition{Op: CompareEQ}},
		{"lt", "aarch64", Condition{Op: CompareLT, Signed: true}},
		{"le", "aarch64", Condition{Op: CompareLE, Signed: true}},
		{"gt", "aarch64", Condition{Op: CompareGT, Signed: true}},
		{"ge", "aarch64", Condition{Op: CompareGE, Signed: true}},
		{"mi", "aarch64", Condition{Op: CompareLT, Signed: true}},
		{"pl", "aarch64", Condition{Op: CompareGE, Signed: true}},
		{"cs", "aarch64", Condition{Op: CompareGE}},
		{"hs", "aarch64", Condition{Op: CompareGE}},
		{"cc", "aarch64", Condition{Op: CompareLT}},
		{"lo", "aarch64", Condition{Op: CompareLT}},
		{"hi", "aarch64", Condition{Op: CompareGT}},
		{"ls", "aarch64", Condition{Op: CompareLE}},

		{"e", "s390x", Condition{Op: CompareEQ}},
		{"ne", "s390x", Condition{Op: CompareNE}},
		{"l", "s390x", Condition{Op: CompareLT, Signed: true}},
		{"le", "s390x", Condition{Op: CompareLE, Signed: true}},
		{"g", "s390x", Condition{Op: CompareGT, Signed: true}},
		{"ge", "s390x", Condition{Op: CompareGE, Signed: true}},
		{"lo", "s390x", Condition{Op: CompareLT}},
		{"hs", "s390x", Condition{Op: CompareGE}},
		{"hi", "s390x", Condition{Op: CompareGT}},
		{"ls", "s390x", Condition{Op: CompareLE}},
	}
	for _, tc := range cases {
		got, ok := ParseCondition(tc.name)
		if !ok {
			t.Fatalf("%s condition %q was rejected", tc.family, tc.name)
		}
		if got != tc.want {
			t.Fatalf("%s condition %q = %+v, want %+v", tc.family, tc.name, got, tc.want)
		}
	}
}

// TestParseConditionRejectsOverflow keeps the decoder honest: an overflow flag has
// no SSA value, so it must not become a comparison.
func TestParseConditionRejectsOverflow(t *testing.T) {
	for _, name := range []string{"o", "no", "vs", "vc", "", "always"} {
		if condition, ok := ParseCondition(name); ok {
			t.Fatalf("condition %q decoded as %+v", name, condition)
		}
	}
}

// TestParseConditionIgnoresCaseAndSpace keeps the loader free of normalization.
func TestParseConditionIgnoresCaseAndSpace(t *testing.T) {
	got, ok := ParseCondition(" EQ ")
	if !ok || got.Op != CompareEQ {
		t.Fatalf("padded condition = %+v", got)
	}
	if got, ok := ParseCondition("Mi"); !ok || got.Op != CompareLT || !got.Signed {
		t.Fatalf("mixed-case condition = %+v", got)
	}
}
