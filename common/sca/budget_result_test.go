package sca

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/gomod"
	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	"github.com/yaklang/yaklang/common/sca/core/locktoml"
	"github.com/yaklang/yaklang/common/sca/core/lockyaml"
	"github.com/yaklang/yaklang/common/sca/core/pyrequire"
	"github.com/yaklang/yaklang/common/sca/core/rpm"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/core/xmlrecord"
)

func largeNPMLock(n int) string {
	var b strings.Builder
	b.WriteString(`{"lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0","dependencies":{`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"p%d":"1.0.0"`, i)
	}
	b.WriteString(`}}`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `,"node_modules/p%d":{"version":"1.0.0","resolved":"https://example.invalid/p%d.tgz"}`, i, i)
	}
	b.WriteString(`}}`)
	return b.String()
}

func largeJSONObject(n int) []byte {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"k%d":%d`, i, i)
	}
	b.WriteByte('}')
	return []byte(b.String())
}

func TestResultBudgetGapBeforeDTO(t *testing.T) {
	lock := largeNPMLock(80)
	if len(lock) < 4000 {
		t.Fatalf("fixture too small: %d", len(lock))
	}
	limits := ResourceLimits{MaxFileBytes: 1 << 20, MaxTotalReadBytes: 4 << 20, MaxComponents: 200000, MaxObservations: 400000, MaxEdges: 1000000, MaxExpressionNodes: 100000, MaxResultBytes: 2048}
	r, err := ScanReport(context.Background(), fstest.MapFS{"package-lock.json": {Data: []byte(lock)}}, WithResourceLimits(limits), WithSnapshotID("budget"))
	if r == nil {
		t.Fatal("nil report")
	}
	if err == nil || r.Complete || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("result-memory budget must fail before completing oversized JSON inventory: complete=%v err=%v components=%d bytes=%d", r.Complete, err, len(r.Components), len(lock))
	}
}

func TestResultBudgetReaders(t *testing.T) {
	ctx := budget.Bind(context.Background(), mustLimits(t, ResourceLimits{MaxResultBytes: 512, MaxFileBytes: 1 << 20, MaxExpressionNodes: 100000}))
	raw := largeJSONObject(400)
	if _, err := jsonrecord.Parse(ctx, raw); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("JSON working copy not charged: %v", err)
	}
	yaml := []byte("lockfileVersion: '6.0'\npackages:\n" + strings.Repeat("  /p@1.0.0:\n    dev: false\n", 80))
	if _, err := lockyaml.Parse(ctx, yaml); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("YAML working copy not charged: %v", err)
	}
	toml := []byte("version = 3\n" + strings.Repeat("[[package]]\nname = 'p'\nversion = '1.0.0'\n", 80))
	if _, _, err := locktoml.ReadRecords(ctx, strings.NewReader(string(toml)), "package"); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("TOML working copy not charged: %v", err)
	}
	xml := []byte("<project>" + strings.Repeat("<d><g>a</g><a>b</a><v>1</v></d>", 80) + "</project>")
	var pom budgetPOM
	if err := xmlrecord.Decode(ctx, strings.NewReader(string(xml)), &pom); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("XML working copy not charged: %v", err)
	}
}

func TestResultBudgetCancelAndOnce(t *testing.T) {
	l, err := (budget.Limits{MaxResultBytes: 1 << 20}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	st := budget.From(budget.Bind(context.Background(), l))
	if err := st.Once("shared", 1, 100); err != nil {
		t.Fatal(err)
	}
	before := st.ResultBytes()
	if err := st.Once("shared", 1, 100); err != nil {
		t.Fatal(err)
	}
	if st.ResultBytes() != before {
		t.Fatalf("shared material charged twice: %d -> %d", before, st.ResultBytes())
	}
	ctx, cancel := context.WithCancel(budget.Bind(context.Background(), l))
	cancel()
	if _, err := jsonrecord.Parse(ctx, []byte(`{"a":1}`)); err == nil {
		t.Fatal("cancelled parse succeeded")
	}
}

func TestResultBudgetWorkersDeterministic(t *testing.T) {
	lock := largeNPMLock(80)
	limits := ResourceLimits{MaxFileBytes: 1 << 20, MaxTotalReadBytes: 4 << 20, MaxComponents: 200000, MaxObservations: 400000, MaxEdges: 1000000, MaxResultBytes: 2048}
	var want string
	for _, n := range []int{1, 2, 4, 8} {
		r, err := ScanReport(context.Background(), extraFS(map[string]string{"package-lock.json": lock}), _withConcurrent(n), WithResourceLimits(limits), WithSnapshotID("budget"))
		if r == nil || err == nil || r.Complete || !errors.Is(err, scanerr.ErrResourceLimit) {
			t.Fatalf("workers=%d expected result budget: complete=%v err=%v", n, r.Complete, err)
		}
		got := extra5149JSON(t, r)
		if want == "" {
			want = got
		} else if got != want {
			t.Fatalf("worker count changed exhausted report")
		}
	}
}

func TestDecoderScratchChargedBeforeAlloc(t *testing.T) {
	raw := []byte(`{"k":1}`)
	limits := mustLimits(t, ResourceLimits{MaxResultBytes: int64(len(raw) + 64), MaxFileBytes: 1 << 20, MaxExpressionNodes: 100000})
	ctx := budget.Bind(context.Background(), limits)
	if _, err := jsonrecord.Parse(ctx, raw); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("JSON decoder scratch not charged beyond file bytes: %v", err)
	}
	xml := []byte("<project><n>a</n></project>")
	ctx = budget.Bind(context.Background(), limits)
	var pom budgetPOM
	if err := xmlrecord.Decode(ctx, strings.NewReader(string(xml)), &pom); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("XML decoder scratch not charged beyond file bytes: %v", err)
	}
}

func TestResultBudgetNegativeRejected(t *testing.T) {
	if _, err := (budget.Limits{MaxResultBytes: -1}).Normalize(); err == nil {
		t.Fatal("negative MaxResultBytes accepted")
	}
	z, err := (budget.Limits{}).Normalize()
	if err != nil || z.MaxResultBytes != 256<<20 {
		t.Fatalf("zero MaxResultBytes default: %+v %v", z, err)
	}
}

func TestResultBudgetObjectsExceedFileBytes(t *testing.T) {
	raw := largeJSONObject(80)
	limits := mustLimits(t, ResourceLimits{MaxResultBytes: int64(len(raw) + 64), MaxFileBytes: 1 << 20, MaxExpressionNodes: 100000})
	ctx := budget.Bind(context.Background(), limits)
	if _, err := jsonrecord.Parse(ctx, raw); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("node tree must be charged beyond file bytes (%d): %v", len(raw), err)
	}
}

func TestResultBudgetTextJSONTOMLYAMLXML(t *testing.T) {
	ctx := budget.Bind(context.Background(), mustLimits(t, ResourceLimits{MaxResultBytes: 256, MaxFileBytes: 1 << 20, MaxExpressionNodes: 100000}))
	mod := []byte("module example.test/app\n" + strings.Repeat("require example.test/p v1.0.0\n", 40))
	if _, err := gomod.Parse(ctx, mod, gomod.Limits{}); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("go.mod directives not charged: %v", err)
	}
	req := []byte(strings.Repeat("pkg==1.0.0\n", 40))
	if _, err := pyrequire.Parse(ctx, req); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("requirements records not charged: %v", err)
	}
}

func TestResultBudgetRPMAndJAR(t *testing.T) {
	gz, err := os.ReadFile("core/rpm/testdata/libuuid.gz")
	if err != nil {
		t.Fatal(err)
	}
	z, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(io.LimitReader(z, 4<<20))
	z.Close()
	if err != nil {
		t.Fatal(err)
	}
	ctx := budget.Bind(context.Background(), mustLimits(t, ResourceLimits{MaxResultBytes: 64, MaxFileBytes: 8 << 20, MaxTotalReadBytes: 16 << 20, MaxExpressionNodes: 100000}))
	if _, err := rpm.Parse(ctx, bytes.NewReader(data), int64(len(data)), rpm.Limits{}); err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("RPM package emit not charged: %v", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("META-INF/maven/g/a/pom.properties")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write([]byte("groupId=g\nartifactId=a\nversion=1.0.0\n")); err != nil {
		t.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := ScanReport(context.Background(), fstest.MapFS{"x.jar": {Data: buf.Bytes()}}, WithResourceLimits(ResourceLimits{MaxResultBytes: 64, MaxFileBytes: 1 << 20, MaxTotalReadBytes: 4 << 20, MaxComponents: 200000, MaxObservations: 400000, MaxEdges: 1000000}), WithSnapshotID("budget-jar"))
	if r == nil || err == nil || r.Complete || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("JAR inventory not charged: complete=%v err=%v", r != nil && r.Complete, err)
	}
}

func TestResultBudgetReportAndCancel(t *testing.T) {
	lock := largeNPMLock(80)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := ScanReport(ctx, extraFS(map[string]string{"package-lock.json": lock}), WithResourceLimits(ResourceLimits{MaxResultBytes: 2048, MaxFileBytes: 1 << 20, MaxTotalReadBytes: 4 << 20}), WithSnapshotID("budget-cancel"))
	if r == nil || err == nil || r.Complete {
		t.Fatalf("cancelled scan completed: %+v %v", r, err)
	}
}

func mustLimits(t *testing.T, l ResourceLimits) budget.Limits {
	t.Helper()
	n, err := l.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// A fixed-schema empty record exercises decoder scratch/input refusal before
// mapping; real POM field semantics are checked in the POM package.
type budgetPOM struct{}

func (*budgetPOM) XMLRecordKind() xmlrecord.Kind { return xmlrecord.POM }
