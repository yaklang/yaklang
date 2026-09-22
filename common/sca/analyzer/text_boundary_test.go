package analyzer

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
)

func TestTextBlockFinalLineAndBudget(t *testing.T) {
	for _, input := range []string{"Name: one", "\n\r\nName: one", "Name: one\n"} {
		b, err := ReadBlock(bufio.NewReader(strings.NewReader(input)))
		if err != io.EOF || strings.TrimSpace(string(b)) != "Name: one" {
			t.Fatalf("lost final field: %q, %v", b, err)
		}
	}
	l, _ := (budget.Limits{MaxFieldBytes: 5}).Normalize()
	_, err := readBlock(budget.Bind(context.Background(), l), bufio.NewReaderSize(strings.NewReader("Name: too long"), 16))
	if err == nil || !strings.Contains(err.Error(), "resource_limit") {
		t.Fatalf("unbounded field: %v", err)
	}
}

func TestDpkgFinalLineAndMissingIdentity(t *testing.T) {
	a := &dpkgAnalyzer{}
	for _, ending := range []string{"", "\n", "\n\n"} {
		pkgs, err := a.analyzeStatus(strings.NewReader("\nPackage: example\nStatus: install ok installed\nVersion: 1.2.3" + ending))
		if err != nil || len(pkgs) != 1 || pkgs[0].Version != "1.2.3" {
			t.Fatalf("final package lost: %+v, %v", pkgs, err)
		}
	}
	for _, input := range []string{"Package: example\nVersion: 1\n", "Package: example\nStatus: install ok installed\n"} {
		if _, err := a.analyzeStatus(strings.NewReader(input)); err == nil {
			t.Fatal("incomplete record silently accepted")
		}
	}
}
