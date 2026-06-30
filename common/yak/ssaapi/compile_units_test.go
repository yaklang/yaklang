package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// buildPlanForLanguage creates the language's Builder and runs the plan
// through it, mirroring how the engine drives partition + dependencies.
func buildPlanForLanguage(t *testing.T, lang ssaconfig.Language, fs filesys_interface.FileSystem, files []string) *UnitPlan {
	t.Helper()
	create, ok := LanguageBuilderCreater[lang]
	require.True(t, ok, "no builder registered for language %v", lang)
	return buildCompileUnitPlan(create(), lang, fs, files)
}

func TestCompileUnitPlanJavaTopoOrder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/a/A.java", "package a;\nimport b.B;\nclass A { B b; }\n")
	vf.AddFile("src/b/B.java", "package b;\nclass B {}\n")

	plan := buildPlanForLanguage(t, ssaconfig.JAVA, vf, []string{"src/a/A.java", "src/b/B.java"})

	require.Len(t, plan.Units, 2)
	require.Contains(t, plan.Units, "java:a")
	require.Contains(t, plan.Units, "java:b")
	require.Contains(t, plan.Edges, UnitRef{From: "java:a", To: "java:b", Kind: "import", Raw: "b.B"})
	require.Len(t, plan.Order, 2)
	require.Equal(t, "java:b", plan.Order[0][0].Key)
	require.Equal(t, "java:a", plan.Order[1][0].Key)
}

func TestCompileUnitPlanMergesCycles(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/a/A.java", "package a;\nimport b.B;\nclass A { B b; }\n")
	vf.AddFile("src/b/B.java", "package b;\nimport a.A;\nclass B { A a; }\n")

	plan := buildPlanForLanguage(t, ssaconfig.JAVA, vf, []string{"src/a/A.java", "src/b/B.java"})

	require.Len(t, plan.Order, 1)
	require.Len(t, plan.Order[0], 2)
	require.Equal(t, "java:a", plan.Order[0][0].Key)
	require.Equal(t, "java:b", plan.Order[0][1].Key)
}

func TestCompileUnitExecutionBatchesMergeSmallSCCs(t *testing.T) {
	order := [][]*CompileUnit{
		{testCompileUnit("unit:a", 2, 200)},
		{testCompileUnit("unit:b", 3, 300)},
		{testCompileUnit("unit:c", 10, 1000)},
	}

	batches := buildCompileUnitExecutionBatches(order, 5, 0)

	require.Len(t, batches, 2)
	require.Equal(t, []string{"unit:a", "unit:b"}, batches[0].unitKeys)
	require.Equal(t, 0, batches[0].startSCC)
	require.Equal(t, 1, batches[0].endSCC)
	require.Equal(t, 5, batches[0].files)
	require.Equal(t, int64(500), batches[0].bytes)
	require.Equal(t, []string{"unit:c"}, batches[1].unitKeys)
	require.Equal(t, 2, batches[1].startSCC)
	require.Equal(t, 2, batches[1].endSCC)
}

func TestCompileUnitExecutionBatchesCanKeepSCCGranularity(t *testing.T) {
	order := [][]*CompileUnit{
		{testCompileUnit("unit:a", 2, 200)},
		{testCompileUnit("unit:b", 3, 300)},
		{testCompileUnit("unit:c", 10, 1000)},
	}

	batches := buildCompileUnitExecutionBatches(order, 1, 0)

	require.Len(t, batches, 3)
	require.Equal(t, []string{"unit:a"}, batches[0].unitKeys)
	require.Equal(t, []string{"unit:b"}, batches[1].unitKeys)
	require.Equal(t, []string{"unit:c"}, batches[2].unitKeys)
}

func TestCompileUnitWriterCacheDisabledForSingleSmallBatch(t *testing.T) {
	batches := []compileUnitExecutionBatch{
		{unitKeys: []string{"unit:a"}, files: 2, bytes: 200},
	}

	require.False(t, compileUnitWriterCacheEnabled(false, batches, compileUnitResidentFastPathMaxBytes/2))
	require.False(t, compileUnitWriterCacheEnabled(true, batches, compileUnitResidentFastPathMaxBytes/2))
}

func TestCompileUnitWriterCacheEnabledForMultiBatchOrLargeProject(t *testing.T) {
	multiBatch := []compileUnitExecutionBatch{
		{unitKeys: []string{"unit:a"}, files: 2, bytes: 200},
		{unitKeys: []string{"unit:b"}, files: 2, bytes: 200},
	}
	singleLargeBatch := []compileUnitExecutionBatch{
		{unitKeys: []string{"unit:a"}, files: 2, bytes: compileUnitResidentFastPathMaxBytes + 1},
	}

	require.True(t, compileUnitWriterCacheEnabled(true, multiBatch, compileUnitResidentFastPathMaxBytes/2))
	require.True(t, compileUnitWriterCacheEnabled(true, singleLargeBatch, compileUnitResidentFastPathMaxBytes+1))
}

func TestSSACompileAdaptiveGCPolicyDefaultsAndOverrides(t *testing.T) {
	t.Setenv("GOGC", "")
	t.Setenv("GOMEMLIMIT", "")
	t.Setenv(ssaCompileAdaptiveGCEnv, "")
	t.Setenv(ssaCompileGOGCEnv, "")
	t.Setenv(ssaCompileMemLimitEnv, "")

	gcPercent, setGC := ssaCompileGCPercent()
	memLimit, setMemLimit := ssaCompileMemoryLimit()
	require.False(t, setGC)
	require.Equal(t, defaultSSACompileGOGC, gcPercent)
	require.False(t, setMemLimit)
	require.Equal(t, int64(defaultSSACompileMemLimit), memLimit)
	require.Nil(t, startSSACompileAdaptiveGC(nil))

	t.Setenv(ssaCompileAdaptiveGCEnv, "1")
	stop := startSSACompileAdaptiveGC(nil)
	require.NotNil(t, stop)
	stop()

	t.Setenv(ssaCompileGOGCEnv, "120")
	t.Setenv(ssaCompileMemLimitEnv, "512MiB")
	gcPercent, setGC = ssaCompileGCPercent()
	memLimit, setMemLimit = ssaCompileMemoryLimit()
	require.True(t, setGC)
	require.Equal(t, 120, gcPercent)
	require.True(t, setMemLimit)
	require.Equal(t, int64(512*1024*1024), memLimit)

	t.Setenv(ssaCompileGOGCEnv, "off")
	t.Setenv(ssaCompileMemLimitEnv, "disabled")
	_, setGC = ssaCompileGCPercent()
	_, setMemLimit = ssaCompileMemoryLimit()
	require.False(t, setGC)
	require.False(t, setMemLimit)
}

func TestEnvFlagEnabledFalseSpellings(t *testing.T) {
	t.Setenv("YAK_TEST_FLAG", "no")
	require.False(t, envFlagEnabled("YAK_TEST_FLAG"))
	t.Setenv("YAK_TEST_FLAG", "disabled")
	require.False(t, envFlagEnabled("YAK_TEST_FLAG"))
	t.Setenv("YAK_TEST_FLAG", "1")
	require.True(t, envFlagEnabled("YAK_TEST_FLAG"))
}

func TestHeapProfileDirDisabledSpellings(t *testing.T) {
	t.Setenv("YAK_SSA_HEAP_PROFILE_DIR", "off")
	require.Empty(t, heapProfileDir())
	t.Setenv("YAK_SSA_HEAP_PROFILE_DIR", "none")
	require.Empty(t, heapProfileDir())
	t.Setenv("YAK_SSA_HEAP_PROFILE_DIR", "build/heap")
	require.Equal(t, "build/heap", heapProfileDir())
}

func TestCompileUnitPlanJavaTemplateResourceMergesWithServletUnit(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/com/example/DemoServlet.java", `package com.example;
class DemoServlet {
    void doGet(javax.servlet.http.HttpServletRequest request) {
        request.getRequestDispatcher("/WEB-INF/jsp/demo.jsp").forward(request, null);
    }
}
`)
	vf.AddFile(`src\main\webapp\WEB-INF\jsp\demo.jsp`, `<html>${userInput}</html>`)

	plan := buildPlanForLanguage(t, ssaconfig.JAVA, vf, []string{
		"src/main/java/com/example/DemoServlet.java",
		`src\main\webapp\WEB-INF\jsp\demo.jsp`,
	})

	require.Contains(t, plan.Units, "java:com.example")
	require.Contains(t, plan.Units, "resource:src/main/webapp/WEB-INF/jsp")
	require.Contains(t, plan.Edges, UnitRef{From: "java:com.example", To: "resource:src/main/webapp/WEB-INF/jsp", Kind: "template", Raw: "/WEB-INF/jsp/demo.jsp"})
	require.Contains(t, plan.Edges, UnitRef{From: "resource:src/main/webapp/WEB-INF/jsp", To: "java:com.example", Kind: "template-owner", Raw: "/WEB-INF/jsp/demo.jsp"})
	require.Len(t, plan.Order, 1)
	require.Len(t, plan.Order[0], 2)
}

func TestCompileUnitPlanJavaDefaultPackageTemplateResourceMergesByBasename(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("XSSExampleServlet.java", `class XSSVulnerableServlet {
    void doPost(javax.servlet.http.HttpServletRequest request) {
        request.getRequestDispatcher("/xss-vulnerable.jsp").forward(request, null);
    }
}
`)
	vf.AddFile("src/main/webapp/jsp/xss-vulnerable.jsp", `<div>${requestScope.userInput}</div>`)

	plan := buildPlanForLanguage(t, ssaconfig.JAVA, vf, []string{
		"XSSExampleServlet.java",
		"src/main/webapp/jsp/xss-vulnerable.jsp",
	})

	require.Contains(t, plan.Units, "dir:.")
	require.Contains(t, plan.Units, "resource:src/main/webapp/jsp")
	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "resource:src/main/webapp/jsp", Kind: "template", Raw: "/xss-vulnerable.jsp"})
	require.Contains(t, plan.Edges, UnitRef{From: "resource:src/main/webapp/jsp", To: "dir:.", Kind: "template-owner", Raw: "/xss-vulnerable.jsp"})
	require.Len(t, plan.Order, 1)
	require.Len(t, plan.Order[0], 2)
}

func TestCompileUnitPlanJavaFreeMarkerViewNameMergesWithTemplateResource(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("controller.java", `import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;

@Controller
public class GreetingController {
    public String submit(String input, Model model) {
        model.addAttribute("userInput", input);
        return "greeting";
    }
}
`)
	vf.AddFile("src/main/resource/greeting.ftl", `<h1>Hello, ${name}!</h1>`)

	plan := buildPlanForLanguage(t, ssaconfig.JAVA, vf, []string{
		"controller.java",
		"src/main/resource/greeting.ftl",
	})

	require.Contains(t, plan.Units, "dir:.")
	require.Contains(t, plan.Units, "resource:src/main/resource")
	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "resource:src/main/resource", Kind: "template", Raw: "greeting"})
	require.Contains(t, plan.Edges, UnitRef{From: "resource:src/main/resource", To: "dir:.", Kind: "template-owner", Raw: "greeting"})
	require.Len(t, plan.Order, 1)
	require.Len(t, plan.Order[0], 2)
}

func TestCompileUnitPlanDynamicLanguageStillBuildsDirectoryUnits(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("app/index.php", "<?php require './lib/a.php';")
	vf.AddFile("app/lib/a.php", "<?php function a() {}")

	plan := buildPlanForLanguage(t, ssaconfig.PHP, vf, []string{"app/index.php", "app/lib/a.php"})

	require.Len(t, plan.Units, 2)
	require.Empty(t, plan.Edges)
	require.Len(t, plan.Order, 2)
}

func TestCompileUnitPlanPythonModuleFileImportTopoOrder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("helper/__init__.py", "")
	vf.AddFile("helper/db_manager.py", "class DBManager:\n    pass\n")
	vf.AddFile("sqli_app.py", "from helper.db_manager import DBManager\nDBManager()\n")

	plan := buildPlanForLanguage(t, ssaconfig.PYTHON, vf, []string{
		"helper/__init__.py",
		"helper/db_manager.py",
		"sqli_app.py",
	})

	require.Contains(t, plan.Units, "dir:helper")
	require.Contains(t, plan.Units, "dir:.")
	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "dir:helper", Kind: "import", Raw: "helper.db_manager"})
	requireUnitBefore(t, plan, "dir:helper", "dir:.")
}

func TestCompileUnitPlanTypeScriptRelativeImportTopoOrder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/interfaces/IService.ts", "export interface IService { getData(): string }\n")
	vf.AddFile("src/services/DataService.ts", "import { IService } from '../interfaces/IService';\nexport class DataService implements IService { getData() { return 'ok' } }\n")
	vf.AddFile("src/main.ts", "import { DataService } from './services/DataService';\nconsole.log(new DataService().getData())\n")

	plan := buildPlanForLanguage(t, ssaconfig.TS, vf, []string{
		"src/main.ts",
		"src/interfaces/IService.ts",
		"src/services/DataService.ts",
	})

	require.Contains(t, plan.Units, "dir:src")
	require.Contains(t, plan.Units, "dir:src/interfaces")
	require.Contains(t, plan.Units, "dir:src/services")
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src", To: "dir:src/services", Kind: "import", Raw: "./services/DataService"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/services", To: "dir:src/interfaces", Kind: "import", Raw: "../interfaces/IService"})
	require.Len(t, plan.Order, 3)
	require.Equal(t, "dir:src/interfaces", plan.Order[0][0].Key)
	require.Equal(t, "dir:src/services", plan.Order[1][0].Key)
	require.Equal(t, "dir:src", plan.Order[2][0].Key)
}

func TestCompileUnitPlanGoModuleImportTopoOrder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", "module github.com/yaklang/yaklang\n\ngo 1.20\n")
	vf.AddFile("src/main/go/main.go", `package main

import "github.com/yaklang/yaklang/A"

var PI = A.PI
`)
	vf.AddFile("src/main/go/A/test.go", `package A

import "github.com/yaklang/yaklang/B"

var PI = B.PI
`)
	vf.AddFile("src/main/go/B/test.go", `package B

var PI = 3.1415926
`)

	plan := buildPlanForLanguage(t, ssaconfig.GO, vf, []string{
		"src/main/go/go.mod",
		"src/main/go/main.go",
		"src/main/go/A/test.go",
		"src/main/go/B/test.go",
	})

	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/main/go", To: "dir:src/main/go/A", Kind: "import", Raw: "github.com/yaklang/yaklang/A"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/main/go/A", To: "dir:src/main/go/B", Kind: "import", Raw: "github.com/yaklang/yaklang/B"})
	requireUnitBefore(t, plan, "dir:src/main/go/B", "dir:src/main/go/A")
	requireUnitBefore(t, plan, "dir:src/main/go/A", "dir:src/main/go")
}

func TestCompileUnitPlanGoSourceRootAliasImportTopoOrder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", "module github.com/yaklang/yaklang\n\ngo 1.20\n")
	vf.AddFile("src/main/go/main.go", `package main

import "go0p/A"

var PI = A.PI
`)
	vf.AddFile("src/main/go/A/test.go", `package A

import "go0p/B"

var PI = B.PI
`)
	vf.AddFile("src/main/go/B/test.go", `package B

var PI = 3.1415926
`)

	plan := buildPlanForLanguage(t, ssaconfig.GO, vf, []string{
		"src/main/go/go.mod",
		"src/main/go/main.go",
		"src/main/go/A/test.go",
		"src/main/go/B/test.go",
	})

	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/main/go", To: "dir:src/main/go/A", Kind: "import", Raw: "go0p/A"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/main/go/A", To: "dir:src/main/go/B", Kind: "import", Raw: "go0p/B"})
	requireUnitBefore(t, plan, "dir:src/main/go/B", "dir:src/main/go/A")
	requireUnitBefore(t, plan, "dir:src/main/go/A", "dir:src/main/go")
}

func requireUnitBefore(t *testing.T, plan *UnitPlan, before string, after string) {
	t.Helper()
	positions := make(map[string]int)
	for i, group := range plan.Order {
		for _, unit := range group {
			positions[unit.Key] = i
		}
	}
	beforeIndex, ok := positions[before]
	require.Truef(t, ok, "missing unit %s in order", before)
	afterIndex, ok := positions[after]
	require.Truef(t, ok, "missing unit %s in order", after)
	require.Less(t, beforeIndex, afterIndex)
}

func testCompileUnit(key string, fileCount int, bytes int64) *CompileUnit {
	files := make([]string, 0, fileCount)
	for i := 0; i < fileCount; i++ {
		files = append(files, key)
	}
	return &CompileUnit{
		Key:   key,
		Path:  key,
		Files: files,
		Bytes: bytes,
	}
}

// TestCompileUnitPlanGoImportBlock exercises a multi-line `import ( ... )` block
// with bare, aliased, and dot imports, and asserts `package main` does not
// produce a spurious import edge.
func TestCompileUnitPlanGoImportBlock(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", "module github.com/yaklang/yaklang\n\ngo 1.20\n")
	vf.AddFile("src/main/go/main.go", `package main

import (
	"github.com/yaklang/yaklang/A"
	"github.com/yaklang/yaklang/B"

	myc "github.com/yaklang/yaklang/C"
	. "github.com/yaklang/yaklang/D"
)

var _ = A.PI
var _ = B.PI
var _ = myc.PI
var _ = PI
`)
	vf.AddFile("src/main/go/A/a.go", "package A\nvar PI = 1\n")
	vf.AddFile("src/main/go/B/b.go", "package B\nvar PI = 2\n")
	vf.AddFile("src/main/go/C/c.go", "package C\nvar PI = 3\n")
	vf.AddFile("src/main/go/D/d.go", "package D\nvar PI = 4\n")

	plan := buildPlanForLanguage(t, ssaconfig.GO, vf, []string{
		"src/main/go/go.mod", "src/main/go/main.go",
		"src/main/go/A/a.go", "src/main/go/B/b.go",
		"src/main/go/C/c.go", "src/main/go/D/d.go",
	})

	// `package main` must not be treated as an import path.
	for _, e := range plan.Edges {
		require.NotEqual(t, "main", e.Raw)
	}
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/main/go", To: "dir:src/main/go/A", Kind: "import", Raw: "github.com/yaklang/yaklang/A"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/main/go", To: "dir:src/main/go/B", Kind: "import", Raw: "github.com/yaklang/yaklang/B"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/main/go", To: "dir:src/main/go/C", Kind: "import", Raw: "github.com/yaklang/yaklang/C"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/main/go", To: "dir:src/main/go/D", Kind: "import", Raw: "github.com/yaklang/yaklang/D"})
}

// TestCompileUnitPlanJavaImportForms exercises multiple imports, a static
// import, and a wildcard import, plus a package declaration.
func TestCompileUnitPlanJavaImportForms(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/a/A.java", `package a;
import b.B;
import static c.C.method;
import d.*;
class A { B b; C c; }
`)
	vf.AddFile("src/b/B.java", "package b;\nclass B {}\n")
	vf.AddFile("src/c/C.java", "package c;\nclass C {}\n")
	vf.AddFile("src/d/D.java", "package d;\nclass D {}\n")

	plan := buildPlanForLanguage(t, ssaconfig.JAVA, vf, []string{
		"src/a/A.java", "src/b/B.java", "src/c/C.java", "src/d/D.java",
	})

	require.Contains(t, plan.Units, "java:a")
	require.Contains(t, plan.Edges, UnitRef{From: "java:a", To: "java:b", Kind: "import", Raw: "b.B"})
	require.Contains(t, plan.Edges, UnitRef{From: "java:a", To: "java:c", Kind: "import", Raw: "c.C.method"})
	require.Contains(t, plan.Edges, UnitRef{From: "java:a", To: "java:d", Kind: "import", Raw: "d"})
}

// TestCompileUnitPlanPythonMultiNameImport exercises `import a, b, c` and
// `import a as b, c as d` (multiple modules per statement, aliases stripped),
// plus `from x import ...`.
func TestCompileUnitPlanPythonMultiNameImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("helper/__init__.py", "")
	vf.AddFile("helper/db.py", "class DB: pass\n")
	vf.AddFile("helper/cache.py", "class Cache: pass\n")
	vf.AddFile("helper/util.py", "class Util: pass\n")
	vf.AddFile("app.py", "import helper.db, helper.cache\nimport helper.util as u\nfrom helper.db import DB\nDB()\n")

	plan := buildPlanForLanguage(t, ssaconfig.PYTHON, vf, []string{
		"helper/__init__.py", "helper/db.py", "helper/cache.py", "helper/util.py", "app.py",
	})

	require.Contains(t, plan.Units, "dir:helper")
	require.Contains(t, plan.Units, "dir:.")
	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "dir:helper", Kind: "import", Raw: "helper.db"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "dir:helper", Kind: "import", Raw: "helper.cache"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "dir:helper", Kind: "import", Raw: "helper.util"})
}

// TestCompileUnitPlanTypeScriptImportForms exercises a multi-line named import,
// a namespace import, `export * from`, `require()`, and dynamic `import()`.
func TestCompileUnitPlanTypeScriptImportForms(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/interfaces/I.ts", "export interface I {}\n")
	vf.AddFile("src/services/S.ts", "import {\n\tI,\n} from '../interfaces/I';\nexport class S {}\n")
	vf.AddFile("src/lib/util.ts", "export const u = 1\n")
	vf.AddFile("src/main.ts", "import * as S from './services/S';\nexport * from './lib/util';\nconst u = require('./lib/util');\nimport('./lib/util');\nS;\n")

	plan := buildPlanForLanguage(t, ssaconfig.TS, vf, []string{
		"src/interfaces/I.ts", "src/services/S.ts", "src/lib/util.ts", "src/main.ts",
	})

	require.Contains(t, plan.Edges, UnitRef{From: "dir:src/services", To: "dir:src/interfaces", Kind: "import", Raw: "../interfaces/I"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src", To: "dir:src/services", Kind: "import", Raw: "./services/S"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:src", To: "dir:src/lib", Kind: "import", Raw: "./lib/util"})
}

// TestCompileUnitPlanCIncludes exercises multiple #include forms (angle,
// quoted, and `#  include` with spaces) resolving to their owning units.
func TestCompileUnitPlanCIncludes(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/a.h", "int a(void);\n")
	vf.AddFile("b/b.h", "int b(void);\n")
	vf.AddFile("c/c.h", "int c(void);\n")
	vf.AddFile("main.c", "#include <a/a.h>\n#include \"b/b.h\"\n#  include <c/c.h>\nint main(void){ return 0; }\n")

	plan := buildPlanForLanguage(t, ssaconfig.C, vf, []string{
		"main.c", "a/a.h", "b/b.h", "c/c.h",
	})

	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "dir:a", Kind: "include", Raw: "a/a.h"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "dir:b", Kind: "include", Raw: "b/b.h"})
	require.Contains(t, plan.Edges, UnitRef{From: "dir:.", To: "dir:c", Kind: "include", Raw: "c/c.h"})
}
