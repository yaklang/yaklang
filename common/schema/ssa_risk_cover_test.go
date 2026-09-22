package schema

import "testing"

func risk(mode, hash, file string, line int64, riskType, program string) *SSARisk {
	return &SSARisk{
		ScanMode:        mode,
		RiskFeatureHash: hash,
		CodeSourceUrl:   file,
		Line:            line,
		RiskType:        riskType,
		ProgramName:     program,
		Title:           mode + "-" + hash,
	}
}

func TestCoverSSARisks_LaterModeReplacesEarlier(t *testing.T) {
	program := "p"
	file := "a.php"
	source := risk("source", "same", file, 10, "sql-injection", program)
	source.Title = "source"
	ssa := risk("ssa", "same", file, 10, "sql-injection", program)
	ssa.Title = "ssa"

	got := CoverSSARisks([]*SSARisk{source, ssa})
	if len(got) != 1 || got[0].Title != "ssa" {
		t.Fatalf("ssa should cover source, got %#v", titles(got))
	}

	// A deeper result that already exists is not replaced by an earlier mode
	// that arrives late.
	got = CoverSSARisks([]*SSARisk{ssa, source})
	if len(got) != 1 || got[0].Title != "ssa" {
		t.Fatalf("source must not cover ssa, got %#v", titles(got))
	}
}

func TestCoverSSARisks_UnfinishedLaterStageKeepsFloor(t *testing.T) {
	only := risk("struct", "floor", "a.php", 4, "sql-injection", "p")
	got := CoverSSARisks([]*SSARisk{only})
	if len(got) != 1 || got[0] != only {
		t.Fatalf("struct floor should remain when ssa produced nothing")
	}
}

func TestCoverSSARisks_SameLineDifferentHash(t *testing.T) {
	program := "p"
	file := "a.php"
	source := risk("source", "text", file, 8, "sql-injection", program)
	ssa := risk("ssa", "ir", file, 8, "sql-injection", program)
	got := CoverSSARisks([]*SSARisk{source, ssa})
	if len(got) != 1 || got[0].ScanMode != "ssa" {
		t.Fatalf("same line and risk type should keep ssa, got %#v", titles(got))
	}

	other := risk("ssa", "other-bug", file, 8, "xss", program)
	got = CoverSSARisks([]*SSARisk{source, other})
	if len(got) != 2 {
		t.Fatalf("different risk type on the same line should both stay, got %d", len(got))
	}
}

func TestCoverSSARisks_SameHashDifferentFileStays(t *testing.T) {
	a := risk("struct", "same", "a.php", 1, "sql-injection", "p")
	b := risk("ssa", "same", "b.php", 1, "sql-injection", "p")
	got := CoverSSARisks([]*SSARisk{a, b})
	if len(got) != 2 {
		t.Fatalf("same feature in two files should both stay, got %d", len(got))
	}
}

func TestCoverSSARisks_SameModeSameHashKeepsLater(t *testing.T) {
	first := risk("ssa", "same", "a.php", 3, "sql-injection", "p")
	first.Title = "struct-stage"
	second := risk("ssa", "same", "a.php", 3, "sql-injection", "p")
	second.Title = "analyze-stage"
	got := CoverSSARisks([]*SSARisk{first, second})
	if len(got) != 1 || got[0].Title != "analyze-stage" {
		t.Fatalf("later stage should replace the same ssa finding, got %#v", titles(got))
	}
}

func TestCoverSSARisks_SameModeSameLineKeepsLater(t *testing.T) {
	a := risk("ssa", "hash-a", "a.php", 3, "sql-injection", "p")
	a.Title = "unit"
	b := risk("ssa", "hash-b", "a.php", 3, "sql-injection", "p")
	b.Title = "analyze"
	got := CoverSSARisks([]*SSARisk{a, b})
	if len(got) != 1 || got[0].Title != "analyze" {
		t.Fatalf("later ssa result on the same line should replace the earlier one, got %#v", titles(got))
	}

	otherLine := risk("ssa", "hash-c", "a.php", 9, "sql-injection", "p")
	got = CoverSSARisks([]*SSARisk{b, otherLine})
	if len(got) != 2 {
		t.Fatalf("different lines should both stay, got %d", len(got))
	}
}

func titles(risks []*SSARisk) []string {
	out := make([]string, 0, len(risks))
	for _, risk := range risks {
		if risk == nil {
			out = append(out, "<nil>")
			continue
		}
		out = append(out, risk.Title)
	}
	return out
}
