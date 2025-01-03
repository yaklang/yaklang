package php

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestCFG(t *testing.T) {
	t.Run("test condition1", func(t *testing.T) {
		code := `<?php
	$data = $_POST['data'] ??"aa";
	println($data);
	`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			result, err := prog.SyntaxFlowWithError(`println(* #-> * as $param)`)
			if err != nil {
				return err
			}
			values := result.GetValues("param")
			require.Contains(t, values.String(), "aa")
			return nil
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("check no variables declare", func(t *testing.T) {
		code := `<?php
$a = $a??12312;
println($a);`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			result, err := prog.SyntaxFlowWithError(`println(* #-> * as $param)`)
			if err != nil {
				return err
			}
			values := result.GetValues("param")
			require.Contains(t, values.String(), "12312")
			return nil
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
