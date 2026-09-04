package tests

import (
	"embed"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/csharp/asp"
	aspparser "github.com/yaklang/yaklang/common/yak/csharp/asp/parser"
)

const savedASPFrontFixtureMaxParseDuration = 30 * time.Second

//go:embed all:code
var aspFs embed.FS

func TestAllASPFrontFixtures(t *testing.T) {
	found := false
	err := fs.WalkDir(aspFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() {
			return nil
		}
		lower := strings.ToLower(filePath)
		if !strings.HasSuffix(lower, ".aspx") && !strings.HasSuffix(lower, ".asp") {
			return nil
		}
		raw, err := aspFs.ReadFile(filePath)
		require.NoError(t, err)
		t.Run(filePath, func(t *testing.T) {
			start := time.Now()
			ast, err := asp.Front(string(raw))
			require.NoError(t, err, "parse ASP AST FrontEnd error")
			require.NotNil(t, ast)
			require.LessOrEqual(t, time.Since(start), savedASPFrontFixtureMaxParseDuration)
		})
		found = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, found, "no embed asp fixtures found")
	require.GreaterOrEqual(t, len(listASPFixtures(t)), aspMinFixtureCount)
}

func TestASPScriptletAndEcho(t *testing.T) {
	src := `<html><body><% int x = 1; %><%= x %></body></html>`
	start := time.Now()
	ast, err := asp.Front(src)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.LessOrEqual(t, time.Since(start), savedASPFrontFixtureMaxParseDuration)
}

func TestASPEdgeSnippets_Front(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "directive only",
			src:  `<%@ Page Language="C#" %>`,
		},
		{
			name: "scriptlet echo databind",
			src:  `<% int x = 1; %><%= x %><%# Eval("N") %>`,
		},
		{
			name: "nested tags with scriptlet",
			src:  `<div><span><% int n = 2; %><b><%= n %></b></span></div>`,
		},
		{
			name: "script island",
			src:  `<script runat="server">void F() {}</script>`,
		},
		{
			name: "declaration block",
			src:  `<%! int k = 1; %><p>ok</p>`,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			ast, err := asp.Front(tc.src)
			require.NoError(t, err, "parse ASP AST FrontEnd error")
			require.NotNil(t, ast)
			require.LessOrEqual(t, time.Since(start), savedASPFrontFixtureMaxParseDuration)
		})
	}
}

func TestASPFrontNamedScriptlet(t *testing.T) {
	src := `<html><body><% int keptScriptlet = 42; %><%= keptScriptlet %></body></html>`
	start := time.Now()
	ast, err := asp.Front(src)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.LessOrEqual(t, time.Since(start), savedASPFrontFixtureMaxParseDuration)

	var scriptlets []aspparser.IAspScriptletContext
	var echos []aspparser.IAspExpressionContext
	var walk func(antlr.Tree)
	walk = func(n antlr.Tree) {
		if n == nil {
			return
		}
		if sl, ok := n.(aspparser.IAspScriptletContext); ok {
			scriptlets = append(scriptlets, sl)
		}
		if ex, ok := n.(aspparser.IAspExpressionContext); ok {
			echos = append(echos, ex)
		}
		for i := 0; i < n.GetChildCount(); i++ {
			walk(n.GetChild(i))
		}
	}
	walk(ast)
	require.NotEmpty(t, scriptlets, "shipped asp.Front AST must contain a scriptlet node")
	require.NotEmpty(t, echos, "shipped asp.Front AST must contain an echo expression node")
	foundKept := false
	for _, sl := range scriptlets {
		if strings.Contains(sl.GetText(), "keptScriptlet") {
			foundKept = true
		}
	}
	require.True(t, foundKept, "scriptlet node must contain keptScriptlet")
}
