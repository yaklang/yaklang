package ssaapi

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestProgramQueryContentDispatchesByMode covers the content entry for all
// three modes: the rule text is compiled first and the compiled frame decides
// how the query runs -- SSA on the program, source on the program snapshot,
// struct on the program's units.
func TestProgramQueryContentDispatchesByMode(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.go", `package main

func run(cmd string) {
	sink(cmd)
}

func sink(any) {}
`)
	vf.AddFile("leak.env", "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n")

	progName := "program-content-dispatch-" + uuid.NewString()
	progs, err := ParseProjectWithFS(vf,
		WithLanguage(ssaconfig.GO),
		WithProgramName(progName),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	prog := progs[0]
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	})

	cases := []struct {
		name    string
		content string
		alert   string
	}{
		{
			name: "ssa",
			content: `desc(mode: "ssa", language: golang, title: "content ssa")
sink(* as $arg) as $call
alert $call`,
			alert: "call",
		},
		{
			name: "source",
			// The snapshot holds the compiled source files, so the regex
			// matches the Go file rather than the unrelated .env fixture.
			content: `desc(mode: "source", language: general, title: "content source")
${*}.pattern_regex(/func sink\(/) as $hit
alert $hit`,
			alert: "hit",
		},
		{
			name: "struct",
			content: `desc(mode: "struct", language: golang, title: "content struct")
sink(* as $arg) as $call
alert $call`,
			alert: "call",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := prog.SyntaxFlowWithError(tc.content)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.GreaterOrEqual(t, len(res.GetValues(tc.alert)), 1,
				"the %s rule must produce an alert", tc.name)
		})
	}
}
