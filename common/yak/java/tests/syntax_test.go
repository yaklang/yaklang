package tests

import (
	"embed"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
)

//go:embed code
var codeFs embed.FS

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		_, err := java2ssa.Frontend(src)
		require.Nil(t, err, "parse AST FrontEnd error : %v", err)
	})
}

func TestAllSyntaxForJava_G4(t *testing.T) {
	entry, err := codeFs.ReadDir("code")
	if err != nil {
		t.Fatalf("no embed syntax files found: %v", err)
	}
	for _, f := range entry {
		if f.IsDir() {
			continue
		}
		codePath := path.Join("code", f.Name())
		if !strings.HasSuffix(codePath, ".java") {
			continue
		}
		raw, err := codeFs.ReadFile(codePath)
		if err != nil {
			t.Fatalf("cannot found syntax fs: %v", codePath)
		}
		validateSource(t, codePath, string(raw))
	}
}

func TestCheck1(t *testing.T) {
	badCode := `package org.apache.avalon.framework.logger;

public interface LogEnabled {
	public abstract void enableLogging(Logger var1) {	}
}
`
	validateSource(t, "", badCode)
}

func TestPHPInterpolatedCurlyBraces(t *testing.T) {
	phpCode := `<?php
function build_rules($cpips, $cpiplist, $interfaces, $rdrtag, $authtag) {
	$rules = "table <{$cpips}> { " . join(' ', $cpiplist)  . "}\n";
	$rules .= "ether pass in on { {$interfaces} } tag \"{$rdrtag}\"\n";
	$rules .= "pass in quick on {$interfaces} proto tcp from any to <{$cpips}> port {$rdrtag} ridentifier {$authtag}\n";
	return $rules;
}

$rules = build_rules('cpzone', ['127.0.0.1', '127.0.0.2'], 'igb0 igb1', 'rdr', 'auto');
echo $rules;
`

	ast, err := php2ssa.Frontend(phpCode)
	require.NoError(t, err)
	require.NotNil(t, ast)
}
