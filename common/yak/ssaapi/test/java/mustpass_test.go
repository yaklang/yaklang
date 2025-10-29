package java

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed mustpass
var mustpassFS embed.FS

//go:embed sample
var sourceCodeSample embed.FS

// const MUSTPASS_JAVA_CACHE_KEY = "54Ot5qCH562+77yM5Y+v5Lul5Lit6Ze05aSE55CGZGVzIGFlc+etieWKoOWvhu+8jOaXoOmcgOWGjeeisHB5IOKAlOKAlOaYr+aenOWunuiPjOWVig==a-"

func TestMustPassMapping(t *testing.T) {
	ssatest.CheckFSWithProgram(
		t, "",
		filesys.NewEmbedFS(sourceCodeSample),
		filesys.NewEmbedFS(mustpassFS),
		ssaapi.WithLanguage(ssaconfig.JAVA),
	)
}

func TestMustPass_JAVA_Debug_Compile(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	progName := uuid.NewString()

	_, err := ssaapi.ParseProjectWithFS(filesys.NewEmbedFS(sourceCodeSample), ssaapi.WithProgramName(progName), ssaapi.WithLanguage(ssaconfig.JAVA))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	program, err := ssaapi.FromDatabase(progName)
	if err != nil {
		t.Fatalf("get program from database failed: %v", err)
	}
	_ = program
}

func TestMustPass_Debug(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	progName := uuid.NewString()

	prog, err := ssaapi.ParseProjectWithFS(filesys.NewEmbedFS(sourceCodeSample), ssaapi.WithProgramName(progName), ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err, "compile failed")
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

	keyword := "local-file-write.sf"
	// prog, err = ssaapi.FromDatabase(progName)
	// require.NoError(t, err, "load from database ")

	code := filesys.NewEmbedFS(mustpassFS)

	err = filesys.Recursive(".", filesys.WithEmbedFS(mustpassFS), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if !strings.Contains(s, keyword) {
			return nil
		}
		raw, err := code.ReadFile(s)
		if err != nil {
			return err
		}
		result, err := prog.SyntaxFlowWithError(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		result.Show()
		result.Dump(false)
		if len(result.GetErrors()) > 0 {
			t.Fatal("errors: ", strings.Join(result.GetErrors(), "\n"))
		}

		result.Show()

		fmt.Println("\n--------------------------------------")
		totalGraph := result.GetAllValuesChain().DotGraph()
		if err != nil {
			t.Fatalf("create dot graph failed: %v", err)
		}
		fmt.Println(totalGraph)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnnontation(t *testing.T) {

	check := func(t *testing.T, prog *ssaapi.Program) {
		rule := `
*Mapping.__ref__?{opcode: function} as $entryFunc;
$entryFunc<getFormalParams>?{opcode: param && !have: this} as $source;
`
		result, err := prog.SyntaxFlowWithError(rule)
		if err != nil {
			t.Fatal(err)
		}
		result.Show()
		valueName := lo.Map(result.GetValues("entryFunc"), func(value *ssaapi.Value, _ int) string {
			return value.String()
		})
		require.Greater(t, len(valueName), 10)
		require.Greater(t, len(result.GetValues("source")), 10)
	}

	t.Run("memory ", func(t *testing.T) {
		prog, err := ssaapi.ParseProjectWithFS(
			filesys.NewEmbedFS(sourceCodeSample),
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}
		check(t, prog[0])
	})

	t.Run("db", func(t *testing.T) {
		progName := uuid.NewString()
		// ssadb.DeleteProgram(ssadb.GetDB(), MUSTPASS_JAVA_CACHE_KEY)
		_, err := ssaapi.ParseProjectWithFS(
			filesys.NewEmbedFS(sourceCodeSample),
			ssaapi.WithProgramName(progName),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			// ssaapi.WithCacheTTL(500*time.Millisecond), //	trigger  cache save/refresh/load
		)
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

		prog, err := ssaapi.FromDatabase(progName)
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}
		check(t, prog)
	})

}

func TestAnnontation_SimpleCode(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("sample/groovy_eval.java", `
package org.vuln.javasec.controller.basevul.rce;
import groovy.lang.GroovyShell;
import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;

@Controller
@RequestMapping("/home/rce")
public class GroovyExecIF {

    @GetMapping("/groovy")
    public String groovyExec(String cmd, Model model) {
        GroovyShell shell = new GroovyShell();
        try {
            shell.evaluate(cmd);
            model.addAttribute("results", "执行成功！！！");
        } catch (Exception e) {
            e.printStackTrace();
            model.addAttribute("results", e.toString());
        }
        return "basevul/rce/groovy";
    }
}
	`)

	vf.AddFile("sample/annotation.java", `
package com.vuln.controller;

public class DemoABCEntryClass {
    @RequestMapping(value = "/one")
    public String methodEntry(@RequestParam(value = "xml_str") String xmlStr) throws Exception {
        return "Hello World" + xmlStr;
    }
}
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
*Mapping.__ref__ as $Mapping 
$Mapping?{opcode: function} as $entryFunc;
	`, map[string][]string{
		"entryFunc": {"Function-DemoABCEntryClass.methodEntry", "Function-GroovyExecIF.groovyExec"},
	}, false,
		ssaapi.WithLanguage(ssaconfig.JAVA),
	)
}
