package php

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"path/filepath"
	"testing"
)

func TestExclude(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("/var/www/html/1.php", `<?php
phpinfo();
`)
	fs.AddFile("/var/www/exclude/2.php", `<?php
println(2);
`)
	prog, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaapi.PHP))
	require.NoError(t, err)
	prog.Show()
	result, err := prog.SyntaxFlowWithError(`println(* #-> * as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	require.True(t, result.GetValues("param").Len() != 0)
	prog, err = ssaapi.ParseProjectWithFS(fs, ssaapi.WithExcludeFile(func(path, filename string) bool {
		dir, _ := filepath.Split(path)
		if dir == "var/www/exclude/" {
			return true
		}
		return false
	}), ssaapi.WithLanguage(ssaapi.PHP))
	require.NoError(t, err)
	prog.Show()
	result, err = prog.SyntaxFlowWithError(`println(* #-> * as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	require.True(t, result.GetValues("param").Len() == 0)
}

func TestExcludeFile(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("/var/www/html/1.php", `<?php
phpinfo();
`)
	fs.AddFile("/var/www/html/vendor/src/main/3.php", `<?php
println(2);
`)
	fs.AddFile("/vendor/src/main/3.php", `<?php
println(2);
`)
	prog, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaapi.PHP))
	require.NoError(t, err)
	result, err := prog.SyntaxFlowWithError(`println(* #-> * as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	values := result.GetValues("param")
	require.True(t, values.Len() != 0)

	filterFunc, err := ssaapi.DefaultExcludeFunc([]string{"**/vendor/**", "vendor/**"})
	require.NoError(t, err)
	prog, err = ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaapi.PHP), filterFunc)
	require.NoError(t, err)
	prog.Show()
	result, err = prog.SyntaxFlowWithError(`println(* #-> * as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	values = result.GetValues("param")
	require.True(t, values.Len() == 0)
}
