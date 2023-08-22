package ssa4yak

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func TestTypePrediction_Int64(t *testing.T) {
	prog := ParseSSA(`var a = 1;`)
	result := prog.InspectVariable("a")
	if !utils.StringArrayContains(result.ProbablyTypes, "number") {
		t.Error("ProbablyTypes should contain int64")
	}
}

func TestTypePrediction_STR(t *testing.T) {
	prog := ParseSSA(`var a = 1; a = "abc"`)
	result := prog.InspectVariable("a")
	if !utils.StringArrayContains(result.ProbablyTypes, "string") {
		t.Error("ProbablyTypes should contain string")
	}
}

func TestTypePrediction_STR2(t *testing.T) {
	prog := ParseSSA(`var a = "abc"; b = 'aasdfasdfasdf'`)
	result := prog.InspectVariable("a")
	if !utils.StringArrayContains(result.ProbablyTypes, "string") {
		t.Error("ProbablyTypes should contain string")
	}
}

func TestTypePrediction_StaticPath(t *testing.T) {
	prog := ParseSSA(`c =5;;;;;;;;;;; var a=1; if c > 1 {a = 1} else {a = "123"};`)
	showProg(prog)
	t.Logf("a var types maybe: %v", prog.InspectVariable("a").ProbablyValues)
	typeVerbose := strings.Join(prog.InspectVariable("a").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(typeVerbose), "string", "number") {
		t.Fatalf("ProbablyTypes should contain string and int, but got %v", typeVerbose)
	}
}

func TestTypePrediction_StaticPath2(t *testing.T) {
	prog := ParseSSA(`ab = ["123", "bbb", "ccc"]; c = ab[1];`)
	typeVerbose := strings.Join(prog.InspectVariable("c").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(typeVerbose), "string") {
		t.Fatalf("ProbablyTypes should contain string and int, but got %v", typeVerbose)
	}
}

func TestTypePrediction_StaticPath3(t *testing.T) {
	prog := ParseSSA(`ab = ["123", 1]; c = ab[1];`)
	typeVerbose := strings.Join(prog.InspectVariable("c").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(typeVerbose), "number") {
		t.Fatalf("ProbablyTypes should contain string and int, but got %v", typeVerbose)
	}
}

func TestTypePrediction_StaticPath4(t *testing.T) {
	prog := ParseSSA(`ab = {"abc": 1}; c = ab["abc"];`)
	typeVerbose := strings.Join(prog.InspectVariable("c").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(typeVerbose), "number") {
		t.Fatalf("ProbablyTypes should contain string and int, but got %v", typeVerbose)
	}
}
