package textvalidate

import "testing"

// These outcomes were checked against govalidator f21760c49a8d before removal.
// Lexical recognition deliberately differs from numeric/base64 decoding.
func TestCompatibilityBoundaries(t *testing.T) {
	for _, tc := range []struct {
		s                                string
		integer, floating, base64, ascii bool
	}{
		{"", true, false, false, true}, {".", false, true, false, true},
		{"e3", false, true, false, true}, {".e3", false, true, false, true},
		{"-.1", false, false, false, true}, {"+.1", false, false, false, true},
		{"01", false, true, false, true}, {"+0", true, true, false, true}, {"-0", true, true, false, true},
		{"NaN", false, false, false, true}, {"Inf", false, false, false, true},
		{"1e9999", false, true, false, true}, {"Zg==", false, false, true, true},
		{"Zh==", false, false, true, true}, {"Zg", false, false, false, true},
		{"Zg==\n", false, false, false, false}, {"____", false, false, false, true},
		{"你好", false, false, false, false}, {"\x00", false, false, false, false}, {"\x7f", false, false, false, false},
	} {
		got := [4]bool{IsInt(tc.s), IsFloat(tc.s), IsBase64(tc.s), IsPrintableASCII(tc.s)}
		want := [4]bool{tc.integer, tc.floating, tc.base64, tc.ascii}
		if got != want {
			t.Errorf("%q: got %v, want %v", tc.s, got, want)
		}
	}
}
