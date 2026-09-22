package sfreport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sarif"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// sarifKindEnum is the closed set the SARIF 2.1.0 schema allows for
// result.kind. yak used to put its risk_type there (command-injection, xss,
// ...), which made the document schema-invalid, so GitHub rejected the whole
// upload and no alert ever appeared.
var sarifKindEnum = map[string]bool{
	"notApplicable": true,
	"pass":          true,
	"fail":          true,
	"review":        true,
	"open":          true,
	"informational": true,
}

// parseSarifDocument requires the payload to be exactly one JSON document;
// json.Unmarshal rejects trailing content, which is what catches the
// concatenated documents that per-stage saves used to produce.
func parseSarifDocument(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &doc), "SARIF output must be a single JSON document")
	return doc
}

func firstSarifRun(t *testing.T, doc map[string]interface{}) map[string]interface{} {
	t.Helper()
	runs, ok := doc["runs"].([]interface{})
	require.True(t, ok, "runs must be an array")
	require.Len(t, runs, 1, "GitHub rejects a document whose runs share one tool name")
	run, ok := runs[0].(map[string]interface{})
	require.True(t, ok)
	return run
}

func sarifDriver(t *testing.T, run map[string]interface{}) map[string]interface{} {
	t.Helper()
	tool, ok := run["tool"].(map[string]interface{})
	require.True(t, ok)
	driver, ok := tool["driver"].(map[string]interface{})
	require.True(t, ok)
	return driver
}

func mapSlice(t *testing.T, value interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := value.([]interface{})
	require.True(t, ok)
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]interface{})
		require.True(t, ok)
		out = append(out, entry)
	}
	return out
}

// A finding in a subdirectory must keep its full repo-relative path: emitting
// only the basename leaves GitHub unable to place the annotation, so the alert
// silently disappears.
func TestSarifArtifactURI_KeepsSubdirectories(t *testing.T) {
	editor := memedit.NewMemEditor("package main\n")
	editor.SetFileName("nested.go")
	editor.SetFolderPath("sub/deep")

	require.Equal(t, "sub/deep/nested.go", sarifArtifactURI(editor))
}

// The scan root is a virtual prefix, not a repository path.
func TestNormalizeSarifURI_StripsVirtualScanRoot(t *testing.T) {
	require.Equal(t, "sub/deep/nested.go", normalizeSarifURI("/fs.zip(2025-0923-11:47)/sub/deep/nested.go"))
	require.Equal(t, "main.go", normalizeSarifURI("/scan-project(2026-09-20 07:08:12)/main.go"))
	require.Equal(t, "a/b.go", normalizeSarifURI("/a/b.go"))
	require.Equal(t, "main.go", normalizeSarifURI("main.go"))
	require.Equal(t, "a/b.go", normalizeSarifURI(`a\b.go`))
}

// Zero findings still need a run: GitHub closes the alerts of a
// (tool, category) upload whose results no longer contain them, so an empty
// run is how a fixed repository stops being red forever.
func TestSarifReport_EmptyReportKeepsOneRun(t *testing.T) {
	report, err := NewSarifReport()
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, report.SetWriter(&buf))
	require.NoError(t, report.Save())

	run := firstSarifRun(t, parseSarifDocument(t, buf.Bytes()))
	results, ok := run["results"].([]interface{})
	require.True(t, ok, "results is a required field on run")
	require.Empty(t, results)
	require.Equal(t, SarifDriverName, sarifDriver(t, run)["name"])
}

// upload-sarif keys off the declared schema version.
func TestSarifReport_DeclaresSchemaAndVersion(t *testing.T) {
	report, err := NewSarifReport()
	require.NoError(t, err)
	require.Equal(t, string(sarif.Version210), report.report.Version)
	require.NotEmpty(t, report.report.Schema)
}

// An unknown severity must still produce a rankable score rather than falling
// back to an empty string, which GitHub reads as "no severity".
func TestSarifSecuritySeverityFor_DefaultsToMedium(t *testing.T) {
	require.Equal(t, "8.0", sarifSecuritySeverityFor("high"))
	require.Equal(t, "9.5", sarifSecuritySeverityFor("critical"))
	require.Equal(t, "5.0", sarifSecuritySeverityFor(""))
	require.Equal(t, "5.0", sarifSecuritySeverityFor("unknown"))
}

// stdout cannot be rewound, so the CLI wraps it in a SnapshotBuffer: every save
// is buffered, and only the latest one reaches the stream.
func TestSnapshotBuffer_PublishesOnlyLatestSnapshot(t *testing.T) {
	var dst bytes.Buffer
	buffer := NewSnapshotBuffer(&dst)

	require.NoError(t, buffer.Flush(), "nothing to publish before the first save")
	require.Empty(t, dst.String())

	_, err := buffer.Write([]byte(`{"snapshot":1}`))
	require.NoError(t, err)
	// A later save replaces the buffered snapshot instead of appending.
	buffer.Reset()
	_, err = buffer.Write([]byte(`{"snapshot":2}`))
	require.NoError(t, err)

	require.Empty(t, dst.String(), "snapshots stay buffered until the scan ends")
	require.NoError(t, buffer.Flush())
	require.Equal(t, "{\"snapshot\":2}\n", dst.String())
	require.True(t, strings.HasSuffix(dst.String(), "\n"),
		"the snapshot must end with a newline so later output is not glued to it")
}

// A report over a SnapshotBuffer must publish exactly one document to a
// forward-only stream, which is what stdout is.
func TestSarifReport_StdoutStreamGetsOneDocument(t *testing.T) {
	var stream bytes.Buffer
	buffer := NewSnapshotBuffer(&stream)

	report, err := NewSarifReport()
	require.NoError(t, err)
	require.NoError(t, report.SetWriter(buffer))

	require.NoError(t, report.Save())
	empty, err := json.Marshal(report.Report())
	require.NoError(t, err)
	_, err = buffer.Write(empty)
	require.NoError(t, err)
	require.NoError(t, report.Save())

	require.NoError(t, buffer.Flush())
	parseSarifDocument(t, stream.Bytes())
}
