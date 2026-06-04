package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYaklibSSA_NewConfigMultiReturn(t *testing.T) {
	code := `
yakit.AutoInitYakit()
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withProgramName("probe"), ssa.withLanguage("php"))
if err != nil { die("err: %v", err) }
if config == nil { die("config nil") }
jsonStr, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
if len(jsonStr) == 0 { die("empty json") }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
	require.NotContains(t, output, "empty json")
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_ConfigJSONLoadsAndPrintsViaYakitCode(t *testing.T) {
	code := `
yakit.AutoInitYakit()
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withProgramName("probe"), ssa.withProjectName("probe"), ssa.withLanguage("php"))
if err != nil { die("err: %v", err) }
if config == nil { die("config nil") }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
if len(configJSON) == 0 { die("empty json") }
result = json.loads(configJSON)
resultJSON = json.dumps(result)
yakit.Code(resultJSON)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `[code]`)
	require.Contains(t, output, `"project_name": "probe"`)
	require.Contains(t, output, `"language": "php"`)
	require.NotContains(t, output, "empty json")
	require.NotContains(t, output, "YakVM Code DIE")
	require.NotContains(t, output, "unexpected end of JSON input")
}

func TestYaklibSSA_ConfigJSONDynamicMapWritesViaYakitCode(t *testing.T) {
	code := `
yakit.AutoInitYakit()
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withProgramName("probe"), ssa.withProjectName("probe"), ssa.withLanguage("php"))
if err != nil { die("err: %v", err) }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
result = json.loads(configJSON)
result["compile_immediately"] = true
result["kind"] = "local"
result["file_count"] = 12
resultJSON = json.dumps(result)
yakit.Code(resultJSON)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"project_name": "probe"`)
	require.Contains(t, output, `"compile_immediately": true`)
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"file_count": 12`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_ResultMapWritesUseCurrentObjectAfterJsonLoads(t *testing.T) {
	code := `
yakit.AutoInitYakit()
projectExists = false
params = {"info": {"kind": ""}}
params.info.kind = "local"
result = json.loads("{}")
result["compile_immediately"] = true
result["kind"] = params.info.kind
result["project_exists"] = projectExists
resultJSON = json.dumps(result)
yakit.Code(resultJSON)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"compile_immediately": true`)
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"project_exists": false`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_MemberObjectWriteUsesCurrentNestedMap(t *testing.T) {
	code := `
yakit.AutoInitYakit()
params = {"error": {"kind": "", "msg": ""}, "compile_immediately": false}
func setError() {
    params.error.kind = "languageNeedSelectException"
    params.error.msg = "select language"
}
if params.compile_immediately == false {
    setError()
}
result = json.loads("{}")
result["error"] = params.error
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"error": {`)
	require.Contains(t, output, `"kind": "languageNeedSelectException"`)
	require.Contains(t, output, `"msg": "select language"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_YakitCodeDumpsPlainResultMap(t *testing.T) {
	code := `
yakit.AutoInitYakit()
result = {}
result["project_exists"] = "aa"
resultJSON = json.dumps(result)
yakit.Code(resultJSON)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `[code]`)
	require.Contains(t, output, `"project_exists": "aa"`)
	require.NotContains(t, output, "IsMessage:true")
	require.NotContains(t, output, "YakVM Code DIE")
}

// TestYaklibSSA_YakitCodeAndLogSmoke bundles the minimal yakit.Code / yakit.Info
// runtime checks used across map and side-effect regression tests.
func TestYaklibSSA_YakitCodeAndLogSmoke(t *testing.T) {
	output := runBinaryWithEnv(t, `
yakit.AutoInitYakit()
yakit.Info("probe-ok")
yakit.Code(json.dumps({"ok": true, "n": 1}))
`, "", nil)
	require.Contains(t, output, "[yakit][info] probe-ok")
	require.Contains(t, output, `[code]`)
	require.Contains(t, output, `"ok": true`)
	require.NotContains(t, output, "IsMessage:true")
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_NewConfigWithVariadicOptionsSlice(t *testing.T) {
	code := `
yakit.AutoInitYakit()
options = [
    ssa.withProgramName("probe"),
    ssa.withProjectName("probe"),
    ssa.withLanguage("php"),
    ssa.withCodeSourceKind("local"),
    ssa.withCodeSourceLocalFile("/tmp/probe"),
    ssa.withReCompile(true),
]
config, err = ssa.NewConfig(ssa.ModeAll, options...)
if err != nil { die("err: %v", err) }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
yakit.Code(configJSON)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"project_name":"probe"`)
	require.Contains(t, output, `"language":"php"`)
	require.Contains(t, output, `"kind":"local"`)
	require.Contains(t, output, `"local_file":"/tmp/probe"`)
	require.Contains(t, output, `"re_compile":true`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_NewConfigWithProjectDetectOptionShape(t *testing.T) {
	code := `
yakit.AutoInitYakit()
entry = []
params = {"program_name": "probe", "project_name": "probe", "language": "php", "info": {"kind": "local", "local_file": "/tmp/probe", "url": "", "branch": "", "path": ""}}
description = "desc"
excludeFile = ""
reCompile = true
concurrency = 10
strictMode = false
filePerformanceLog = false
jarRecursiveParse = true
options = [
    ssa.withProgramName(params.program_name),
    ssa.withProjectName(params.project_name),
    ssa.withProjectDescription(description),
    ssa.withLanguage(params.language),
    ssa.withCodeSourceKind(params.info.kind),
    ssa.withCodeSourceLocalFile(params.info.local_file),
    ssa.withCodeSourceURL(params.info.url),
    ssa.withCodeSourceBranch(params.info.branch),
    ssa.withCodeSourcePath(params.info.path),
    ssa.withExcludeFile(excludeFile),
    ssa.withReCompile(reCompile),
    ssa.withConcurrency(concurrency),
    ssa.withStrictMode(strictMode),
    ssa.withDescription(description),
    ssa.withEntryFile(entry...),
    ssa.withFilePerformanceLog(filePerformanceLog),
    ssa.withCodeSourceJarRecursiveParse(jarRecursiveParse),
]
config, err = ssa.NewConfig(ssa.ModeAll, options...)
if err != nil { die("err: %v", err) }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
result = json.loads(configJSON)
result["compile_immediately"] = true
resultJSON = json.dumps(result)
yakit.Code(resultJSON)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"project_name": "probe"`)
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"local_file": "/tmp/probe"`)
	require.Contains(t, output, `"compile_immediately": true`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_NewConfigWithSkippedProjectDetectOptionalBlocks(t *testing.T) {
	code := `
yakit.AutoInitYakit()
authConfig = cli.Json("auth-config", cli.setDefault("{}"))
enableIncrementalCompile := cli.Bool("incremental-compile", cli.setDefault(false))
baseProgramName := cli.String("base-program-name")
cli.check()
params = {"program_name": "probe", "project_name": "probe", "language": "php", "info": {"kind": "local", "local_file": "/tmp/probe", "url": "", "branch": "", "path": ""}}
options = [
    ssa.withProgramName(params.program_name),
    ssa.withProjectName(params.project_name),
    ssa.withLanguage(params.language),
    ssa.withCodeSourceKind(params.info.kind),
    ssa.withCodeSourceLocalFile(params.info.local_file),
    ssa.withCodeSourceURL(params.info.url),
    ssa.withCodeSourceBranch(params.info.branch),
    ssa.withCodeSourcePath(params.info.path),
]
if authConfig != nil && authConfig != undefined {
    authKind = authConfig["auth_kind"]
    if authKind != nil && authKind != "" {
        options = append(options, ssa.withCodeSourceAuthKind(authKind))
    }
}
if enableIncrementalCompile {
    options = append(options, ssa.withEnableIncrementalCompile(true), ssa.withReCompile(true))
    if baseProgramName != "" {
        options = append(options, ssa.withBaseProgramName(baseProgramName))
    }
}
config, err = ssa.NewConfig(ssa.ModeAll, options...)
if err != nil { die("new config: %v", err) }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
if len(configJSON) == 0 { die("empty config json") }
result = json.loads(configJSON)
result["kind"] = params.info.kind
resultJSON = json.dumps(result)
yakit.Code(resultJSON)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"project_name": "probe"`)
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"kind": "local"`)
	require.NotContains(t, output, "empty config json")
	require.NotContains(t, output, "unexpected end of JSON input")
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_NewConfigAfterProjectLookupMiss(t *testing.T) {
	code := `
yakit.AutoInitYakit()
params = {"program_name": "probe", "project_name": "probe", "language": "php", "info": {"kind": "local", "local_file": "/tmp/probe"}}
path = params.info.local_file
var config
result = {"branch": "", "config": {}}
existingSSAProject, err = ssa.GetSSAProjectByNameAndURL(params.project_name, path)
if err == nil && existingSSAProject != nil {
    config, err = existingSSAProject.GetConfig()
}
options = [
    ssa.withProgramName(params.program_name),
    ssa.withProjectName(params.project_name),
    ssa.withLanguage(params.language),
    ssa.withCodeSourceKind(params.info.kind),
    ssa.withCodeSourceLocalFile(params.info.local_file),
]
if config == nil {
    result["branch"] = "new"
    config, err = ssa.NewConfig(ssa.ModeAll, options...)
    if err != nil { die("new config: %v", err) }
} else {
    result["branch"] = "update"
    err = config.Update(options...)
    if err != nil { die("update config: %v", err) }
}
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
result["config"] = json.loads(configJSON)
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	})
	require.Contains(t, output, `"branch": "new"`)
	require.Contains(t, output, `"project_name": "probe"`)
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"local_file": "/tmp/probe"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_ProjectLookupFullOptionsBuildResult(t *testing.T) {
	code := `
yakit.AutoInitYakit()
authConfig = cli.Json("auth-config", cli.setDefault("{}"))
enableIncrementalCompile := cli.Bool("incremental-compile", cli.setDefault(false))
baseProgramName := cli.String("base-program-name")
compile_immediately := cli.Bool("compile-immediately", cli.setDefault(false))
concurrency := cli.Int("concurrency", cli.setDefault(10))
entry := cli.FileNames("entry")
strictMode = cli.Bool("StrictMode", cli.setDefault(false))
reCompile := cli.Bool("re-compile", cli.setDefault(true))
filePerformanceLog := cli.Bool("filePerformanceLog", cli.setDefault(false))
jarRecursiveParse := cli.Bool("jar-recursive-parse", cli.setDefault(true))
cli.check()
description = ""
excludeFile = ""
params = {"program_name": "probe", "project_name": "probe", "language": "php", "info": {"kind": "local", "local_file": "/tmp/probe", "url": "", "branch": "", "path": ""}, "error": {"kind": "", "msg": ""}, "file_count": 1, "compile_immediately": false}
params.compile_immediately = compile_immediately
path = params.info.local_file
var projectExists = false
var config
existingSSAProject, err = ssa.GetSSAProjectByNameAndURL(params.project_name, path)
if err == nil && existingSSAProject != nil {
    projectExists = true
    config, err = existingSSAProject.GetConfig()
    if err != nil || config == nil {
    }
}
options = [
    ssa.withProgramName(params.program_name),
    ssa.withProjectName(params.project_name),
    ssa.withProjectDescription(description),
    ssa.withLanguage(params.language),
    ssa.withCodeSourceKind(params.info.kind),
    ssa.withCodeSourceLocalFile(params.info.local_file),
    ssa.withCodeSourceURL(params.info.url),
    ssa.withCodeSourceBranch(params.info.branch),
    ssa.withCodeSourcePath(params.info.path),
    ssa.withExcludeFile(excludeFile),
    ssa.withReCompile(reCompile),
    ssa.withConcurrency(concurrency),
    ssa.withStrictMode(strictMode),
    ssa.withDescription(description),
    ssa.withEntryFile(entry...),
    ssa.withFilePerformanceLog(filePerformanceLog),
    ssa.withCodeSourceJarRecursiveParse(jarRecursiveParse),
]
if authConfig != nil && authConfig != undefined {
    authKind = authConfig["auth_kind"]
    if authKind != nil && authKind != "" {
        options = append(options, ssa.withCodeSourceAuthKind(authKind))
    }
}
if enableIncrementalCompile {
    options = append(options, ssa.withEnableIncrementalCompile(true), ssa.withReCompile(true))
    if baseProgramName != "" {
        options = append(options, ssa.withBaseProgramName(baseProgramName))
    }
}
if config == nil {
    config, err = ssa.NewConfig(ssa.ModeAll, options...)
    if err != nil { die("new config: %v", err) }
} else {
    err = config.Update(options...)
    if err != nil { die("update config: %v", err) }
}
if config == nil { die("config nil") }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
if len(configJSON) == 0 { die("empty config json") }
result = json.loads(configJSON)
result["error"] = params.error
result["file_count"] = params.file_count
result["compile_immediately"] = params.compile_immediately
result["kind"] = params.info.kind
result["project_exists"] = projectExists
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withArgs("--compile-immediately"))
	require.Contains(t, output, `"project_name": "probe"`)
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"local_file": "/tmp/probe"`)
	require.Contains(t, output, `"file_count": 1`)
	require.Contains(t, output, `"compile_immediately": true`)
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"project_exists": false`)
	require.NotContains(t, output, "empty config json")
	require.NotContains(t, output, "unexpected end of JSON input")
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_NestedMapMemberAssignmentsFeedConfigOptions(t *testing.T) {
	code := `
yakit.AutoInitYakit()
params = {"program_name": "", "project_name": "", "language": "", "info": {"kind": "", "local_file": "", "url": ""}}
params.program_name = "probe"
params.project_name = "probe"
params.language = "php"
params.info.kind = "local"
params.info.local_file = "/tmp/probe"
options = [
    ssa.withProgramName(params.program_name),
    ssa.withProjectName(params.project_name),
    ssa.withLanguage(params.language),
    ssa.withCodeSourceKind(params.info.kind),
    ssa.withCodeSourceLocalFile(params.info.local_file),
]
config, err = ssa.NewConfig(ssa.ModeAll, options...)
if err != nil { die("err: %v", err) }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
yakit.Code(configJSON)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"project_name":"probe"`)
	require.Contains(t, output, `"language":"php"`)
	require.Contains(t, output, `"kind":"local"`)
	require.Contains(t, output, `"local_file":"/tmp/probe"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_LocalProjectProbeFeedsConfigOptions(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "index.php"), []byte("<?php echo 'ok';"), 0o644))

	code := `
yakit.AutoInitYakit()
target = cli.String("target", cli.setRequired(true))
languages = cli.StringSlice("language", cli.setMultipleSelect(false), cli.setSelectOption("PHP", "php"))
compile_immediately := cli.Bool("compile-immediately", cli.setDefault(false))
concurrency := cli.Int("concurrency", cli.setDefault(10))
entry := cli.FileNames("entry")
strictMode = cli.Bool("StrictMode", cli.setDefault(false))
reCompile := cli.Bool("re-compile", cli.setDefault(true))
filePerformanceLog := cli.Bool("filePerformanceLog", cli.setDefault(false))
jarRecursiveParse := cli.Bool("jar-recursive-parse", cli.setDefault(true))
cli.check()
target = target.Trim(" ", "\n", "\t")
phpFiles = ["index.php", "composer.json"]
extToLanguage = {"php": "php"}
var countMap = map[string]int{"php": 0}
params = {"program_name": "", "project_name": "", "language": "", "info": {"kind": "", "local_file": "", "url": "", "branch": "", "path": ""}, "file_count": 0, "error": {"kind": "", "msg": ""}, "compile_immediately": false}
func setLanguage(language) {
    if params.language == "" {
        params.language = language
    }
}
if len(languages) > 0 {
    setLanguage(languages[0])
}
func generateFileName(kind, filename) {
    if params.program_name != "" {
        return
    }
    filename2 = str.Split(filename, "/")[-1]
    name := sprintf("%s(%s)", filename2, time.Now().Format("2006-01-02 15:04:05"))
    params.program_name = name
    params.project_name = filename2
}
func AutoParseLanguage() {
    maxinfo = {"key": "", "value": 0}
    for key, value := range countMap {
        if maxinfo.value < value {
            maxinfo.key = key
            maxinfo.value = value
        }
    }
    if maxinfo.value != 0 {
        setLanguage(maxinfo.key)
    }
}
func getLocalInfo(localFile) {
    if !file.IsExisted(localFile) {
        params.error.kind = "fileNotFoundException"
        return
    }
    params.info.kind = "local"
    params.info.local_file = localFile
    filesys.Recursive(
        localFile,
        filesys.onFileStat(func(path, info) {
            params.file_count++
            ext = file.GetExt(path).Lower().TrimLeft(".")
            if ext in extToLanguage {
                lang = extToLanguage[ext]
                countMap[lang] = countMap[lang] + 1
            }
            if params.language == "" {
                switch  {
                case info.Name().Lower() in phpFiles:
                    setLanguage(ssa.PHP)
                }
            }
        }),
    )
    generateFileName("local", file.Abs(localFile))
}
getLocalInfo(target)
if params.language == "" {
    AutoParseLanguage()
}
params.compile_immediately = compile_immediately
options = [
    ssa.withProgramName(params.program_name),
    ssa.withProjectName(params.project_name),
    ssa.withLanguage(params.language),
    ssa.withCodeSourceKind(params.info.kind),
    ssa.withCodeSourceLocalFile(params.info.local_file),
    ssa.withReCompile(reCompile),
    ssa.withConcurrency(concurrency),
    ssa.withStrictMode(strictMode),
    ssa.withEntryFile(entry...),
    ssa.withFilePerformanceLog(filePerformanceLog),
    ssa.withCodeSourceJarRecursiveParse(jarRecursiveParse),
]
config, err = ssa.NewConfig(ssa.ModeAll, options...)
if err != nil { die("new config: %v", err) }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
result = json.loads(configJSON)
result["file_count"] = params.file_count
result["compile_immediately"] = params.compile_immediately
result["kind"] = params.info.kind
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withArgs("--target", projectDir, "--compile-immediately", "--language", "php"))
	require.Contains(t, output, `"compile_immediately": true`)
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"local_file": "`+projectDir+`"`)
	require.Contains(t, output, `"file_count": 1`)
	require.Contains(t, output, `"program_names": [`)
	require.Contains(t, output, `"project_name": "001"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_CLIValuesReachRuntime(t *testing.T) {
	code := `
yakit.AutoInitYakit()
target = cli.String("target", cli.setRequired(true))
languages = cli.StringSlice("language", cli.setMultipleSelect(false), cli.setSelectOption("PHP", "php"))
compile_immediately := cli.Bool("compile-immediately", cli.setDefault(false))
jarRecursiveParse := cli.Bool("jar-recursive-parse", cli.setDefault(true))
cli.check()
result = {"target": target, "languages": languages, "compile_immediately": compile_immediately, "jar_recursive_parse": jarRecursiveParse}
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withArgs("--target", "/tmp/probe", "--compile-immediately", "--language", "php"))
	require.Contains(t, output, `"target": "/tmp/probe"`)
	require.Contains(t, output, `"languages": [`)
	require.Contains(t, output, `"php"`)
	require.Contains(t, output, `"compile_immediately": true`)
	require.Contains(t, output, `"jar_recursive_parse": false`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_CLIStringTrimFeedsFileExists(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "index.php"), []byte("<?php echo 'ok';"), 0o644))

	code := `
yakit.AutoInitYakit()
target = cli.String("target", cli.setRequired(true))
cli.check()
trimmed = target.Trim(" ", "\n", "\t")
result = {"target": target, "trimmed": trimmed, "exists": file.IsExisted(trimmed)}
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withArgs("--target", projectDir))
	require.Contains(t, output, `"target": "`+projectDir+`"`)
	require.Contains(t, output, `"trimmed": "`+projectDir+`"`)
	require.Contains(t, output, `"exists": true`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_FilesysRecursiveCallbackUpdatesOuterMap(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "index.php"), []byte("<?php echo 'ok';"), 0o644))

	code := `
yakit.AutoInitYakit()
target = cli.String("target", cli.setRequired(true))
cli.check()
result = {"count": 0, "name": ""}
filesys.Recursive(
    target,
    filesys.onFileStat(func(path, info) {
        result["count"] = result["count"] + 1
        result["name"] = info.Name().Lower()
    }),
)
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withArgs("--target", projectDir))
	require.Contains(t, output, `"count": 1`)
	require.Contains(t, output, `"name": "index.php"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_FunctionUpdatesOuterOrderedMap(t *testing.T) {
	code := `
yakit.AutoInitYakit()
params = {"language": "", "info": {"kind": ""}}
func setLanguage(language) {
    if params.language == "" {
        params.language = language
    }
}
func setKind(kind) {
    params.info.kind = kind
}
setLanguage("php")
setKind("local")
result = json.loads("{}")
result["language"] = params.language
result["kind"] = params.info.kind
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"kind": "local"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_BranchMemberWriteKeepsTakenPath(t *testing.T) {
	code := `
yakit.AutoInitYakit()
result = {"branch": ""}
flag = true
if flag {
    result["branch"] = "new"
} else {
    result["branch"] = "update"
}
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"branch": "new"`)
	require.NotContains(t, output, `"branch": "update"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_NestedFunctionUpdatesOuterOrderedMap(t *testing.T) {
	code := `
yakit.AutoInitYakit()
params = {"program_name": "", "project_name": "", "info": {"kind": "", "local_file": ""}}
func generateFileName(kind, filename) {
    if params.program_name != "" {
        return
    }
    filename2 = str.Split(filename, "/")[-1]
    params.program_name = filename2
    params.project_name = filename2
}
func getLocalInfo(localFile) {
    params.info.kind = "local"
    params.info.local_file = localFile
    generateFileName("local", file.Abs(localFile))
}
getLocalInfo("/tmp/demo")
yakit.Code(json.dumps(params))
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"program_name": "demo"`)
	require.Contains(t, output, `"project_name": "demo"`)
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"local_file": "/tmp/demo"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_SwitchDefaultUpdatesNestedOuterMap(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "index.php"), []byte("<?php echo 'ok';"), 0o644))

	code := `
yakit.AutoInitYakit()
target = cli.String("target", cli.setRequired(true))
cli.check()
params = {"info": {"kind": "", "local_file": ""}, "file_count": 0, "trace": [], "error": {"kind": ""}}
func getLocalInfo(localFile) {
    ext := file.GetExt(localFile).Lower()
    switch ext {
    case ".zip":
        params.info.kind = "compression"
        params.info.local_file = localFile
    default:
        params.info.kind = "local"
        params.trace = append(params.trace, params.info.kind)
        if !file.IsDir(localFile) {
            params.error.kind = "fileTypeException"
            return
        }
        params.info.local_file = localFile
        filesys.Recursive(localFile, filesys.onFileStat(func(path, info) {
            params.file_count++
        }))
    }
}
getLocalInfo(target)
yakit.Code(json.dumps(params))
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withArgs("--target", projectDir))
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"local_file": "`+projectDir+`"`)
	require.Contains(t, output, `"file_count": 1`)
	require.Contains(t, output, `"trace": [`)
	require.Contains(t, output, `"local"`)
	require.NotContains(t, output, `"kind": "fileTypeException"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_SwitchDefaultNestedMapFeedsResultAndConfig(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "index.php"), []byte("<?php echo 'ok';"), 0o644))

	code := `
yakit.AutoInitYakit()
target = cli.String("target", cli.setRequired(true))
cli.check()
params = {"program_name": "probe", "project_name": "probe", "language": "php", "info": {"kind": "", "local_file": ""}, "file_count": 0}
func getLocalInfo(localFile) {
    switch file.GetExt(localFile).Lower() {
    case ".zip":
        params.info.kind = "compression"
    default:
        params.info.kind = "local"
        params.info.local_file = localFile
        filesys.Recursive(localFile, filesys.onFileStat(func(path, info) {
            params.file_count++
        }))
    }
}
getLocalInfo(target)
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withProgramName(params.program_name), ssa.withProjectName(params.project_name), ssa.withLanguage(params.language), ssa.withCodeSourceKind(params.info.kind), ssa.withCodeSourceLocalFile(params.info.local_file))
if err != nil { die("new config: %v", err) }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
result = json.loads(configJSON)
result["kind"] = params.info.kind
result["local_file"] = params.info.local_file
result["file_count"] = params.file_count
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withArgs("--target", projectDir))
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"local_file": "`+projectDir+`"`)
	require.Contains(t, output, `"file_count": 1`)
	require.Contains(t, output, `"project_name": "probe"`)
	require.Contains(t, output, `"language": "php"`)
	require.NotContains(t, output, "unexpected end of JSON input")
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_SideEffectMemberPhiFeedsYaklibOption(t *testing.T) {
	code := `
yakit.AutoInitYakit()
languages = cli.StringSlice("language", cli.setMultipleSelect(false), cli.setSelectOption("PHP", "php"))
cli.check()
params = {"language": "", "config_language": ""}
func setLanguage(language) {
    if params.language == "" {
        params.language = language
    }
}
if len(languages) > 0 {
    setLanguage(languages[0])
}
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withLanguage(params.language))
if err != nil { die("err: %v", err) }
configJSON, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
result = json.loads(configJSON)
params.config_language = result.BaseInfo.language
yakit.Code(json.dumps(params))
`
	output := runBinaryWithEnv(t, code, "", nil, withArgs("--language", "php"))
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"config_language": "php"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_DynamicMapWritesFromMemberValues(t *testing.T) {
	code := `
yakit.AutoInitYakit()
params = {"language": "", "info": {"kind": "", "local_file": "", "url": ""}}
params.language = "php"
params.info.kind = "local"
result = json.loads("{}")
result["language"] = params.language
result["kind"] = params.info.kind
yakit.Code(json.dumps(result))
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"kind": "local"`)
	require.NotContains(t, output, "YakVM Code DIE")
}

func TestYaklibSSA_ModeAllConstant(t *testing.T) {
	code := `
yakit.AutoInitYakit()
if ssa.ModeAll == 0 { die("ModeAll is zero") }
println(ssa.ModeAll)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.NotContains(t, output, "ModeAll is zero")
	require.Contains(t, output, "127")
}

func TestYaklibSSA_MultiReturnThreeValues(t *testing.T) {
	code := `
yakit.AutoInitYakit()
f = func() { return 1, 2, 3 }
a, b, c = f()
if a != 1 || b != 2 || c != 3 { die("tuple unpack failed: %v %v %v", a, b, c) }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}

func TestYaklibSSA_MultiReturnWithError(t *testing.T) {
	code := `
yakit.AutoInitYakit()
f = func() { return "data", nil }
a, err = f()
if a != "data" { die("bad data: %v", a) }
if err != nil { die("expected nil err, got %v", err) }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}

func TestYaklibSSA_WithExcludeFileEmptySlice(t *testing.T) {
	code := `
yakit.AutoInitYakit()
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withProgramName("t"), ssa.withLanguage("php"), ssa.withExcludeFile([]))
if err != nil { die("err: %v", err) }
if config == nil { die("config nil") }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}

func TestYaklibSSA_SyncWaitGroupRegression(t *testing.T) {
	code := `
yakit.AutoInitYakit()
wg = sync.NewWaitGroup()
if wg == nil { die("waitgroup nil") }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}
