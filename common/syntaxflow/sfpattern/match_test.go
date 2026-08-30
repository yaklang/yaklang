package sfpattern

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestMatchRegexp_AWSKey(t *testing.T) {
	files := map[string]string{
		"config.env": `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
OTHER=1
`,
		"ok.txt": "no secrets here",
	}
	hits, err := MatchRegexpHits(files, "*", []string{`AKIA[0-9A-Z]{16}`})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Contains(t, hits[0].Text, "AKIA")
}

func TestExecRule_SourceMode_NoSSA(t *testing.T) {
	ruleContent := `
desc(
  mode: "source",
  language: "general",
  title: "aws akia",
  rule_id: "test-aws-akia",
  level: "critical",
)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit for {
  level: "critical",
  message: "aws key",
}
`
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(ruleContent)
	require.NoError(t, err)
	require.True(t, sfvm.FrameIsSourceMode(frame))

	files := map[string]string{
		"a.properties": `key=AKIAIOSFODNN7EXAMPLE`,
	}
	result, err := ExecFrame(frame, files)
	require.NoError(t, err)
	require.True(t, HasAlert(result))
	require.GreaterOrEqual(t, AlertCount(result), 1)
}

func TestExecFrameOnFS_PositiveNegative(t *testing.T) {
	ruleContent := `
desc(
  mode: "source",
  language: "general",
  title: "private key pem",
  level: "critical",
  "file://bad.pem": <<<POS
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF6PZGFw
-----END RSA PRIVATE KEY-----
POS,
  "safefile://good.txt": <<<NEG
public key material only
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC
NEG,
)
${*}.re(/-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----/) as $hit
alert $hit for { level: "critical", message: "pem" }
`
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(ruleContent)
	require.NoError(t, err)
	require.True(t, sfvm.FrameIsSourceMode(frame))

	pos, err := frame.ExtractVerifyFilesystemAndLanguage()
	require.NoError(t, err)
	require.NotEmpty(t, pos)
	result, err := ExecFrameOnFS(frame, pos[0].GetVirtualFs())
	require.NoError(t, err)
	require.True(t, HasAlert(result))

	neg, err := frame.ExtractNegativeFilesystemAndLanguage()
	require.NoError(t, err)
	require.NotEmpty(t, neg)
	negResult, err := ExecFrameOnFS(frame, neg[0].GetVirtualFs())
	if err == nil {
		require.False(t, HasAlert(negResult))
	}
}

func TestNewRootFromFS(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("x.java", `password = "s3cretValue"`)
	root, err := NewRootFromFS(vfs)
	require.NoError(t, err)
	require.False(t, root.IsEmpty())
	vals, err := root.FileFilter("*", "regexp", nil, []string{`(?i)password\s*=\s*["'][^"']+["']`})
	require.NoError(t, err)
	require.Greater(t, sfvm.ValuesLen(vals), 0)
}

func TestPatternRootMaterializesBoundedHitWindows(t *testing.T) {
	files := map[string]string{
		"many.txt": "key\n" + strings.Repeat("key\n", 5000),
	}
	root := NewRoot(files)
	all, err := root.FileFilter("many.txt", "regexp", nil, []string{`key`})
	require.NoError(t, err)
	require.Equal(t, 5001, sfvm.ValuesLen(all))

	root.SetSourceHitBatch(0, DefaultSourceHitBatchSize)
	first, err := root.FileFilter("many.txt", "regexp", nil, []string{`key`})
	require.NoError(t, err)
	require.Equal(t, DefaultSourceHitBatchSize, sfvm.ValuesLen(first))

	root.SetSourceHitBatch(DefaultSourceHitBatchSize, DefaultSourceHitBatchSize)
	second, err := root.FileFilter("many.txt", "regexp", nil, []string{`key`})
	require.NoError(t, err)
	require.Equal(t, DefaultSourceHitBatchSize, sfvm.ValuesLen(second))

	root.SetSourceHitBatch(5000, DefaultSourceHitBatchSize)
	last, err := root.FileFilter("many.txt", "regexp", nil, []string{`key`})
	require.NoError(t, err)
	require.Equal(t, 1, sfvm.ValuesLen(last))
	_, _, total := root.SourceHitBatch()
	require.Equal(t, 5001, total)
}

func TestMatchRegexpWithNegatives(t *testing.T) {
	files := map[string]string{
		"config.env": `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
OTHER=1
`,
		"example.env": `AWS_ACCESS_KEY_ID=AKIAEXAMPLEFAKEKEY
`,
		"ok.txt": "no secrets here",
	}
	// Positive matches both files; negative (?i)example|fake removes example.env.
	vals, err := MatchRegexpWithNegatives(files, "*", `\b(A3T[A-Z0-9]|AKIA|AGPA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}\b`, []string{`(?i)example|sample|test|fake`})
	require.NoError(t, err)
	require.Equal(t, 1, sfvm.ValuesLen(vals))
	first, _ := vals[0].(interface{ Path() string })
	require.Contains(t, first.Path(), "config.env")
}

func TestExecRule_PatternRegexNot(t *testing.T) {
	ruleContent := `
desc(
  mode: "source",
  language: "general",
  title: "aws key with negative",
  rule_id: "test-aws-neg",
  level: "critical",
)
${*}.pattern_regex_not(/\b(A3T[A-Z0-9]|AKIA|AGPA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}\b/, /(?i)example|sample|test|fake/) as $hit
alert $hit for {
  level: "critical",
  message: "aws key",
}
`
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(ruleContent)
	require.NoError(t, err)
	require.True(t, sfvm.FrameIsSourceMode(frame))

	files := map[string]string{
		"a.properties": `key=AKIAIOSFODNN7EXAMPLE`,
		"b.properties": `key=AKIAEXAMPLEFAKEKEY123`,
	}
	result, err := ExecFrame(frame, files)
	require.NoError(t, err)
	require.True(t, HasAlert(result))
	require.Equal(t, 1, AlertCount(result))
}
