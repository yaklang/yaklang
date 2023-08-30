package ssa4yak

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func TestTypePrediction_Int64(t *testing.T) {
	prog := ParseSSA(`var a = 1 + 1;`)
	result := prog.InspectVariable("a")
	if !utils.StringArrayContains(result.ProbablyTypes, "number") {
		t.Error("ProbablyTypes should contain int64")
	}
	if !utils.StringArrayContains(result.ProbablyValues, "2") {
		t.Error("ProbablyValue should contain 2")
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
	result := prog.InspectVariable("a")
	t.Logf("a var types maybe: %v", result.ProbablyValues)
	typeVerbose := strings.Join(result.ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(typeVerbose), "string", "number") {
		t.Fatalf("ProbablyTypes should contain string and int, but got %v", typeVerbose)
	}
}

func TestTypePrediction_slice(t *testing.T) {
	prog := ParseSSA(`ab = ["123", "bbb", "ccc"]; c = ab[1];`)
	typeVerbose := strings.Join(prog.InspectVariable("c").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(typeVerbose), "string") {
		t.Fatalf("ProbablyTypes should contain string and int, but got %v", typeVerbose)
	}
}

func TestTypePrediction_Struct(t *testing.T) {
	prog := ParseSSA(`ab = ["123", 1]; c = ab[1];`)
	prog.Show()
	varCProbablyType := strings.Join(prog.InspectVariable("c").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(varCProbablyType), "number") {
		t.Fatalf("ProbablyTypes should contain number, but got %v", varCProbablyType)
	}
	varABProbablyType := strings.Join(prog.InspectVariable("ab").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(varABProbablyType), "struct {string,number}") {
		t.Fatalf("ProbablyTypes should contain struct {string,number} , but got %v", varCProbablyType)
	}
}

func TestTypePrediction_Map2(t *testing.T) {
	prog := ParseSSA(`ab = {"abc": 1}; c = ab["abc"];`)
	typeVerbose := strings.Join(prog.InspectVariable("c").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(typeVerbose), "number") {
		t.Fatalf("ProbablyTypes should contain number, but got %v", typeVerbose)
	}
	typeABVerbose := strings.Join(prog.InspectVariable("ab").ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(typeABVerbose), "map[string]number") {
		t.Fatalf("ProbablyTypes should contain map[string]number, but got %v", typeVerbose)
	}
}

func TestTypePrediction_Static_PhiAndSccp(t *testing.T) {
	prog := ParseSSA(`a = 1;b=1; if a>2{b = "arst"};print(b)`)
	varB := prog.InspectVariable("b")
	varBProbablyType := strings.Join(varB.ProbablyTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(varBProbablyType), "string", "number") {
		t.Fatalf("ProbablyTypes should contain string and number, but got %v", varBProbablyType)
	}
	varBMustType := strings.Join(varB.MustTypes, ",")
	if !utils.MatchAllOfSubString(strings.ToLower(varBMustType), "number") {
		t.Fatalf("musttype should contain number, but got %v", varBProbablyType)
	}
}
