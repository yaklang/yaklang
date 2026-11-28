package ssa2source

import (
	"reflect"
	"testing"
)

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

func TestSourceLineScannersMatchExpectedSyntax(t *testing.T) {
	assignCases := []struct {
		line    string
		wantID  int64
		wantRHS string
		wantOK  bool
	}{
		{line: "    v12 = v3 + 0x4;", wantID: 12, wantRHS: "v3 + 0x4", wantOK: true},
		{line: "\tv1   =   ;   ", wantID: 1, wantRHS: "", wantOK: true},
		{line: "  v1x = 1;", wantOK: false},
		{line: "  v1 = 1", wantOK: false},
	}
	for _, tc := range assignCases {
		id, rhs, ok := parseSourceAssignment(tc.line)
		if ok != tc.wantOK || id != tc.wantID || rhs != tc.wantRHS {
			t.Fatalf("parseSourceAssignment(%q) = (%d, %q, %v), want (%d, %q, %v)", tc.line, id, rhs, ok, tc.wantID, tc.wantRHS, tc.wantOK)
		}
	}

	declCases := []struct {
		line   string
		wantID int64
		wantOK bool
	}{
		{line: "    uint64_t v7; /* forward declaration */", wantID: 7, wantOK: true},
		{line: "    void * v15;", wantID: 15, wantOK: true},
		{line: "  void *v15;", wantOK: false},
		{line: "  9int v1;", wantOK: false},
		{line: "  int v1; v2;", wantID: 1, wantOK: true},
	}
	for _, tc := range declCases {
		id, ok := parseSourceDeclaration(tc.line)
		if ok != tc.wantOK || id != tc.wantID {
			t.Fatalf("parseSourceDeclaration(%q) = (%d, %v), want (%d, %v)", tc.line, id, ok, tc.wantID, tc.wantOK)
		}
	}

	aliasCases := []struct {
		line    string
		wantID  string
		wantReg string
		wantOK  bool
	}{
		{line: "    v9 = entry_rsp - 0x28;", wantID: "9", wantReg: "entry_rsp", wantOK: true},
		{line: "  v4=entry_rbp-0x10;  ", wantID: "4", wantReg: "entry_rbp", wantOK: true},
		{line: "  v4 = entry_rax - 0x10;", wantOK: false},
		{line: "  v4 = entry_rsp - 16;", wantOK: false},
	}
	for _, tc := range aliasCases {
		idStr, reg, ok := parseFrameBaseAlias(tc.line)
		if ok != tc.wantOK || idStr != tc.wantID || reg != tc.wantReg {
			t.Fatalf("parseFrameBaseAlias(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.line, idStr, reg, ok, tc.wantID, tc.wantReg, tc.wantOK)
		}
	}

	var gotIDs []int64
	forEachSourceValue("v1 + v20 + v3x + xv4 + (v5)", func(id int64) bool {
		gotIDs = append(gotIDs, id)
		return true
	})
	wantIDs := []int64{1, 20, 5}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("forEachSourceValue = %v, want %v", gotIDs, wantIDs)
	}

	replaced := replaceSourceValues("  v9 = *(v9 + v10) + v90;", map[string]string{"v9": "entry_rsp"})
	if replaced != "  entry_rsp = *(entry_rsp + v10) + v90;" {
		t.Fatalf("replaceSourceValues = %q", replaced)
	}
	untouched := "  v10 = v11;"
	if replaceSourceValues(untouched, map[string]string{"v9": "entry_rsp"}) != untouched {
		t.Fatalf("expected untouched line")
	}
}

func TestPruneDeadAssignmentsWorklistChain(t *testing.T) {
	lines := []string{
		"void f(void) {",
		"    v1 = 0x1;",
		"    v2 = v1 + 0x2;",
		"    v3 = v2 + 0x3;",
		"    v4 = 0x4;",
		"    return v4;",
		"}",
	}
	got := pruneDeadAssignments(nil, lines)
	want := []string{
		"void f(void) {",
		"    v4 = 0x4;",
		"    return v4;",
		"}",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pruneDeadAssignments =\n%v\nwant\n%v", got, want)
	}
}
