package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestStringLiteral(t *testing.T) {
	code := `<?php 
	print("aaaaa--$a");
	`
	ssatest.CheckSyntaxFlowSource(t, code,
		`print(* #-> ?{!opcode:const} as $target)`, map[string][]string{
			"target": {"$a"},
		}, ssaapi.WithLanguage(ssaapi.PHP),
	)
}
