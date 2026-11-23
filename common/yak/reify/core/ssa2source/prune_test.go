package ssa2source

import "testing"

func TestPruneUnusedParametersKeepsParenthesizedName(t *testing.T) {
	for _, tc := range []struct {
		header string
		body   string
		want   string
	}{
		{
			header: "void main.(*Box[int]).Get(void * arg0, void * arg1) {",
			body:   "    return;",
			want:   "void main.(*Box[int]).Get(void) {",
		},
		{
			header: "void main.(*Box[int]).Get(void * arg0, void * arg1) {",
			body:   "    v1 = arg1;",
			want:   "void main.(*Box[int]).Get(void * arg1) {",
		},
		{
			header: "void pkg.(*T).M(int x, int y) {",
			body:   "    v1 = x + y;",
			want:   "void pkg.(*T).M(int x, int y) {",
		},
	} {
		lines := []string{tc.header, tc.body, "}"}
		got := pruneUnusedParameters(lines)
		if got[0] != tc.want {
			t.Fatalf("header = %q, want %q", got[0], tc.want)
		}
	}
}
