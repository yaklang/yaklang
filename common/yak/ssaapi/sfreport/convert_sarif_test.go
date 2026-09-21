package sfreport_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// sarifKindEnum is the closed set the SARIF 2.1.0 schema allows for
// result.kind. yak used to put its risk_type there (command-injection, xss,
// ...), which made the document schema-invalid and GitHub rejected the whole
// upload, so no alert ever appeared.
var sarifKindEnum = map[string]bool{
	"notApplicable": true,
	"pass":          true,
	"fail":          true,
	"review":        true,
	"open":          true,
	"informational": true,
}

const weakDigestRule = `
	.getInstance?{<typeName>?{have:'java.security'}}(*<slice(index=1)> as $algorithm);
	$algorithm #{ until:` + "`*?{ opcode:const && have:/MD5/}`" + ` }-> as $sink;
	alert $sink for {
		title: "Use of a broken or risky hash algorithm",
		level: "low",
		risk: "weak-crypto"
	}
`

// The file lives in a subdirectory on purpose: the emitted URI has to keep the
// repository-relative path, otherwise GitHub cannot place the annotation.
const riskyJavaApp = `package com.example;

import java.security.MessageDigest;

public class App {
    public String weakDigest(String input) throws Exception {
        MessageDigest md5 = MessageDigest.getInstance("MD5");
        return new String(md5.digest(input.getBytes()));
    }
}`

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

func sarifEntries(t *testing.T, value interface{}) []map[string]interface{} {
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

// scanJavaProject runs the weak-hash rule over a virtual project and returns a
// SyntaxFlowResult that has real risks registered.
func scanJavaProject(t *testing.T) *ssaapi.SyntaxFlowResult {
	t.Helper()

	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/com/example/App.java", riskyJavaApp)

	programName := "sarif-report-" + uuid.NewString()
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName}})
	})

	progs, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(programName),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	result, err := progs[0].SyntaxFlowWithError(weakDigestRule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)
	require.NoError(t, result.CreateRisk())
	require.Positive(t, result.RiskCount(), "fixture must produce at least one risk")
	return result
}

func renderSarif(t *testing.T, result *ssaapi.SyntaxFlowResult) []byte {
	t.Helper()
	report, err := sfreport.NewSarifReport()
	require.NoError(t, err)
	require.True(t, report.AddSyntaxFlowResult(result))

	var buf bytes.Buffer
	require.NoError(t, report.SetWriter(&buf))
	require.NoError(t, report.Save())
	return buf.Bytes()
}

func TestSarifReport_ValidKindAndGitHubMetadata(t *testing.T) {
	result := scanJavaProject(t)
	doc := parseSarifDocument(t, renderSarif(t, result))
	run := firstSarifRun(t, doc)

	results := sarifEntries(t, run["results"])
	require.NotEmpty(t, results)
	for _, entry := range results {
		kind, _ := entry["kind"].(string)
		require.True(t, sarifKindEnum[kind],
			"result.kind %q is not part of the SARIF enum; GitHub rejects the upload", kind)

		fingerprints, ok := entry["partialFingerprints"].(map[string]interface{})
		require.True(t, ok, "partialFingerprints is what keeps an alert matched across runs")
		require.NotEmpty(t, fingerprints[sfreport.SarifFingerprintKey])

		require.NotEmpty(t, entry["ruleId"])
		// The fixture declares level: "low", which maps to warning/2.0.
		require.Equal(t, "warning", entry["level"])
		require.NotEmpty(t, entry["message"])
	}

	driver := sarifDriver(t, run)
	require.Equal(t, sfreport.SarifDriverName, driver["name"])
	require.Equal(t, sfreport.SarifInformationURI, driver["informationUri"])
	require.NotEmpty(t, driver["version"], "GitHub shows the tool version for each analysis")
	require.Equal(t, "diff-code-check", run["automationDetails"].(map[string]interface{})["id"])
}

func TestSarifReport_RulesCarrySeverityAndTags(t *testing.T) {
	run := firstSarifRun(t, parseSarifDocument(t, renderSarif(t, scanJavaProject(t))))
	rules := sarifEntries(t, sarifDriver(t, run)["rules"])
	require.NotEmpty(t, rules)

	for _, rule := range rules {
		require.NotEmpty(t, rule["id"])
		require.NotEmpty(t, rule["shortDescription"], "shortDescription is required by the schema")

		props, ok := rule["properties"].(map[string]interface{})
		require.True(t, ok, "security-severity and tags drive GitHub's alert classification")

		severity, _ := props["security-severity"].(string)
		require.NotEmpty(t, severity, "without security-severity GitHub cannot rank the alert")
		require.Equal(t, "2.0", severity)

		tags, ok := props["tags"].([]interface{})
		require.True(t, ok)
		require.NotEmpty(t, tags)
		for _, tag := range tags {
			// The schema only accepts strings here; a nested array made the
			// upload fail with "is not of a type(s) string".
			_, isString := tag.(string)
			require.True(t, isString, "tag %v must be a string", tag)
		}
	}
}

// Every annotation needs a repository-relative path; a bare basename or a
// leading slash leaves GitHub unable to attach the alert to a file.
func TestSarifReport_ArtifactURIIsRepoRelative(t *testing.T) {
	run := firstSarifRun(t, parseSarifDocument(t, renderSarif(t, scanJavaProject(t))))

	seen := 0
	for _, entry := range sarifEntries(t, run["results"]) {
		locations := sarifEntries(t, entry["locations"])
		require.NotEmpty(t, locations)
		physical := locations[0]["physicalLocation"].(map[string]interface{})
		location := physical["artifactLocation"].(map[string]interface{})

		uri, _ := location["uri"].(string)
		require.NotEmpty(t, uri)
		require.False(t, strings.HasPrefix(uri, "/"), "SARIF wants a repo-relative URI, got %q", uri)
		require.NotContains(t, uri, "(", "the virtual scan root must be stripped, got %q", uri)
		require.Equal(t, "src/main/java/com/example/App.java", uri,
			"the full repository-relative path is required for annotations")
		seen++
	}
	require.Positive(t, seen)
}

// Risks stream into the report as each stage runs; one save at the end must
// publish every streamed result as a single JSON document.
func TestSarifReport_StreamedResultsSavedOnceOnFile(t *testing.T) {
	report, err := sfreport.NewSarifReport()
	require.NoError(t, err)

	result := scanJavaProject(t)
	// Several stages feed the same report before anything is written.
	require.True(t, report.AddSyntaxFlowResult(result))
	require.True(t, report.AddSyntaxFlowResult(result))

	path := filepath.Join(t.TempDir(), "out.sarif")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	require.NoError(t, report.SetWriter(file))
	require.NoError(t, report.Save())

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	doc := parseSarifDocument(t, raw)
	run := firstSarifRun(t, doc)
	require.NotContains(t, string(raw), "}{", "one report must produce one document")

	// Nothing streamed in may be dropped by the single save.
	results := sarifEntries(t, run["results"])
	require.Len(t, results, 2, "both streamed results must survive into the saved document")
}

// Two rules must be merged into the single run GitHub accepts, with every
// result referencing a declared rule.
func TestSarifReport_MergesMultipleResultsIntoOneRun(t *testing.T) {
	report, err := sfreport.NewSarifReport()
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/com/example/App.java", riskyJavaApp)

	programName := "sarif-merge-" + uuid.NewString()
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName}})
	})

	progs, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(programName),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	// Same match, two distinct rule bodies -> two rule ids, one run.
	for _, level := range []string{"low", "high"} {
		rule := `
	.getInstance?{<typeName>?{have:'java.security'}}(*<slice(index=1)> as $algorithm);
	$algorithm #{ until:` + "`*?{ opcode:const && have:/MD5/}`" + ` }-> as $sink;
	alert $sink for {
		title: "Use of a broken or risky hash algorithm",
		level: "` + level + `",
		risk: "weak-crypto"
	}
`
		result, err := progs[0].SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
		require.NoError(t, err)
		require.NoError(t, result.CreateRisk())
		require.True(t, report.AddSyntaxFlowResult(result))
	}

	var buf bytes.Buffer
	require.NoError(t, report.SetWriter(&buf))
	require.NoError(t, report.Save())

	doc := parseSarifDocument(t, buf.Bytes())
	run := firstSarifRun(t, doc)

	rules := sarifEntries(t, sarifDriver(t, run)["rules"])
	require.Len(t, rules, 2, "each distinct rule needs its own descriptor")

	known := map[string]bool{}
	for _, rule := range rules {
		known[rule["id"].(string)] = true
	}
	results := sarifEntries(t, run["results"])
	require.Len(t, results, 2)
	for _, entry := range results {
		ruleID, _ := entry["ruleId"].(string)
		require.True(t, known[ruleID], "every result must reference a declared rule")
	}
}

// The helper other packages call must produce the same GitHub-ready run.
func TestConvertSyntaxFlowResultsToSarif_SingleRun(t *testing.T) {
	report, err := sfreport.ConvertSyntaxFlowResultsToSarif(scanJavaProject(t))
	require.NoError(t, err)
	require.Len(t, report.Runs, 1)
	require.Equal(t, sfreport.SarifDriverName, report.Runs[0].Tool.Driver.Name)
	require.NotEmpty(t, report.Runs[0].Results)
}

// Guard the CLI wiring: the format switch must hand back the compatible report.
func TestConvertSyntaxFlowResultToReport_SarifUsesCompatReport(t *testing.T) {
	instance, err := sfreport.ConvertSyntaxFlowResultToReport(sfreport.SarifReportType)
	require.NoError(t, err)
	report, ok := instance.(*sfreport.SarifReport)
	require.True(t, ok)

	require.True(t, report.AddSyntaxFlowResult(scanJavaProject(t)))

	var buf bytes.Buffer
	require.NoError(t, report.SetWriter(&buf))
	require.NoError(t, report.Save())

	run := firstSarifRun(t, parseSarifDocument(t, buf.Bytes()))
	require.Equal(t, sfreport.SarifDriverName, sarifDriver(t, run)["name"])
}

// A scan that finds nothing still has to declare the tool so GitHub can close
// the alerts the previous upload opened.
func TestSarifReport_CleanProjectStillDeclaresTool(t *testing.T) {
	const safeApp = `package com.example;

import java.security.MessageDigest;

public class App {
    public String safeDigest(String input) throws Exception {
        MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
        return new String(sha256.digest(input.getBytes()));
    }
}`

	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/com/example/App.java", safeApp)

	programName := "sarif-clean-" + uuid.NewString()
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName}})
	})

	progs, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(programName),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	result, err := progs[0].SyntaxFlowWithError(weakDigestRule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)

	report, err := sfreport.NewSarifReport()
	require.NoError(t, err)
	report.AddSyntaxFlowResult(result)

	var buf bytes.Buffer
	require.NoError(t, report.SetWriter(&buf))
	require.NoError(t, report.Save())

	doc := parseSarifDocument(t, buf.Bytes())
	run := firstSarifRun(t, doc)
	require.Empty(t, run["results"])
	require.Equal(t, sfreport.SarifDriverName, sarifDriver(t, run)["name"])
}

// The severity carried by the rule drives both the SARIF level and the score
// GitHub ranks the alert with.
func TestSarifReport_SeverityMapping(t *testing.T) {
	require.Equal(t, "note", sfreport.ToSarifLevel(schema.SFR_SEVERITY_INFO))
	require.Equal(t, "warning", sfreport.ToSarifLevel(schema.SFR_SEVERITY_LOW))
	require.Equal(t, "warning", sfreport.ToSarifLevel(schema.SFR_SEVERITY_WARNING))
	require.Equal(t, "error", sfreport.ToSarifLevel(schema.SFR_SEVERITY_HIGH))
	require.Equal(t, "error", sfreport.ToSarifLevel(schema.SFR_SEVERITY_CRITICAL))
}
