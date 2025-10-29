package php

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestExclude(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("/var/www/html/1.php", `<?php
phpinfo();
`)
	fs.AddFile("/var/www/exclude/2.php", `<?php
println(2);
`)
	prog, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.PHP))
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
	}), ssaapi.WithLanguage(ssaconfig.PHP))
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
	prog, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.PHP))
	require.NoError(t, err)
	result, err := prog.SyntaxFlowWithError(`println(* #-> * as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	values := result.GetValues("param")
	require.True(t, values.Len() != 0)

	filterFunc, err := ssaapi.DefaultExcludeFunc([]string{"**/vendor/**", "vendor/**"})
	require.NoError(t, err)
	prog, err = ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.PHP), filterFunc)
	require.NoError(t, err)
	prog.Show()
	result, err = prog.SyntaxFlowWithError(`println(* #-> * as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	values = result.GetValues("param")
	require.True(t, values.Len() == 0)
}

func TestExcludeFile_Temp(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("src/main/main.go", `
package main

func main() {

}		
		`)
	fs.AddFile("src/main/template.go", `
package main

import (
	"encoding/base64"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"os/exec"
)

func CMD1(c *gin.Context) {
	var ipaddr string
	// Check the request method
	if c.Request.Method == "GET" {
		ipaddr = c.Query("ip")
	} else if c.Request.Method == "POST" {
		ipaddr = c.PostForm("ip")
	}

	Command := fmt.Sprintf("ping -c 4 %s", ipaddr)
	output, err := exec.Command("/bin/sh", "-c", Command).Output()
	if err != nil {
		fmt.Println(err)
		return
	}
	c.JSON(200, gin.H{
		"success": string(output),
	})
}
`)
	prog, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.GO))
	require.NoError(t, err)
	result, err := prog.SyntaxFlowWithError(`exec.Command(*?{opcode:const} #-> as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	values := result.GetValues("param")
	require.True(t, values.Len() != 0)

	filterFunc, err := ssaapi.DefaultExcludeFunc([]string{"**temp**"})
	require.NoError(t, err)
	prog, err = ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.GO), filterFunc)
	require.NoError(t, err)
	prog.Show()
	result, err = prog.SyntaxFlowWithError(`exec.Command(*?{opcode:const} #-> as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	values = result.GetValues("param")
	require.True(t, values.Len() == 0)

	filterFunc, err = ssaapi.DefaultExcludeFunc([]string{"**temp/**"})
	require.NoError(t, err)
	prog, err = ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.GO), filterFunc)
	require.NoError(t, err)
	prog.Show()
	result, err = prog.SyntaxFlowWithError(`exec.Command(*?{opcode:const} #-> as $param)`, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	values = result.GetValues("param")
	require.True(t, values.Len() != 0)
}
