package c

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func TestDefaultLayer(t *testing.T) {
	layer := C{}
	if layer.Kind() != core.LanguageC {
		t.Fatalf("kind = %s", layer.Kind())
	}
	report := layer.Detect(&core.Artifact{Kind: core.KindNative})
	if report == nil || report.Language != core.LanguageC || report.Confidence != core.ConfidenceNone {
		t.Fatalf("report = %+v", report)
	}
	if report.ABI != nil {
		t.Fatalf("default layer must not override the platform ABI: %+v", report.ABI)
	}
	if report.Strings == nil || report.Strings.Kind != core.StringC || !report.Strings.NulTerminated || report.Strings.Header != "pointer" {
		t.Fatalf("string model = %+v", report.Strings)
	}
	if len(report.Notes) != 0 {
		t.Fatalf("default layer must not add limitation notes: %v", report.Notes)
	}
	symbols, err := layer.Recover(&core.Artifact{Kind: core.KindNative})
	if err != nil || len(symbols) != 0 {
		t.Fatalf("recover = %+v, %v", symbols, err)
	}
}
