package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklang/spec"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

var GlobalEvalExports = map[string]interface{}{
	//"eval":   QuickEvalWithoutContext,
	"import": ImportVarFromFile,
}

var EvalExports = map[string]interface{}{
	"Eval":            QuickEvalWithoutContext,
	"LoadVarFromFile": LoadingVariableFrom,
	"Import":          ImportVarFromFile,
	"IsYakFunc":       yaklib.IsYakFunction,
	"params":          setYakEvalParams,
	"recursive":       setYakBatchImportRecursiveParams,
}

// Eval 动态执行一段 yak 代码
// 参数:
//   - i: 要执行的 yak 代码(字符串或字节切片)
//
// 返回值:
//   - 执行过程中产生的错误，成功时为 nil
//
// Example:
// ```
// // VARS: 动态执行一段代码
// err = dyn.Eval("a = 1 + 1")
// // assert: 合法代码执行无错误
// assert err == nil, "valid code should evaluate without error"
// ```
func QuickEvalWithoutContext(i interface{}) error {
	switch ret := i.(type) {
	case []byte:
		return NewScriptEngine(1).Execute(string(ret))
	case string:
		return NewScriptEngine(1).Execute(ret)
	default:
		return utils.Errorf("invalid eval params... need string / []byte")
	}
}

type yakEvalConfig struct {
	params map[string]interface{}
	// 递归导入
	recursive bool
}

type yakEvalConfigOpt func(y *yakEvalConfig)

// params 生成一个 LoadVarFromFile 的配置项，为被加载脚本注入额外参数
// 参数:
//   - i: 注入到被加载脚本中的参数键值表
//
// 返回值:
//   - 可传给 dyn.LoadVarFromFile 的配置项
//
// Example:
// ```
// // 注入参数后加载脚本中的 Exports 变量(作示意)
// vars, err = dyn.LoadVarFromFile("./mod", "Exports", dyn.params({"key": "value"}))
// ```
func setYakEvalParams(i map[string]interface{}) yakEvalConfigOpt {
	return func(y *yakEvalConfig) {
		y.params = i
	}
}

// recursive 生成一个 LoadVarFromFile 的配置项，控制是否递归遍历子目录加载脚本
// 参数:
//   - i: 是否递归加载子目录中的 yak 文件
//
// 返回值:
//   - 可传给 dyn.LoadVarFromFile 的配置项
//
// Example:
// ```
// // 递归加载目录下所有脚本中的 Exports 变量(作示意)
// vars, err = dyn.LoadVarFromFile("./mods", "Exports", dyn.recursive(true))
// ```
func setYakBatchImportRecursiveParams(i bool) yakEvalConfigOpt {
	return func(y *yakEvalConfig) {
		y.recursive = i
	}
}

type yakVariable struct {
	FilePath string
	YakMod   string
	Value    interface{}
	Engine   *antlr4yak.Engine
}

func (y *yakVariable) Callable() bool {
	return yaklib.IsYakFunction(y.Value)
}

func ImportVarFromYakFile(path string, exportsName string) (interface{}, error) {
	engine := yaklang.New()
	if err := engine.RunFile(context.Background(), path); err != nil {
		return nil, utils.Errorf("load file \"%s\" failed: %s", path, err)
	}
	v, ok := engine.GetVar(exportsName)
	if !ok {
		return nil, utils.Errorf("import var[%s] from file[%s] failed", exportsName, path)
	}
	return v, nil
}

func ImportVarFromScript(engine *antlr4yak.Engine, script string, exportsName string) (interface{}, error) {
	if engine == nil {
		return nil, utils.Error("empty engine")
	}

	if err := engine.LoadCode(context.Background(), string(script), nil); err != nil {
		return nil, err
	}
	v, ok := engine.GetVar(exportsName)
	if !ok {
		return nil, utils.Errorf("import var[%s] failed", exportsName)
	}
	return v, nil
}

// Import 从指定 yak 文件中加载并导入名为 exportsName 的变量
// 参数:
//   - file: yak 文件路径(可省略 .yak 后缀)或包含 main.yak 的目录
//   - exportsName: 要导入的变量名
//
// 返回值:
//   - 导入的变量值
//   - 加载失败时返回的错误
//
// Example:
// ```
// // 从 ./mod.yak 导入名为 Exports 的变量(依赖外部文件，作示意)
// v, err = dyn.Import("./mod", "Exports")
// ```
func ImportVarFromFile(file string, exportsName string) (interface{}, error) {
	var absFile string
	yakFile := utils.GetFirstExistedFile(file, fmt.Sprintf("%v.yak", file))
	if yakFile == "" {
		if utils.IsDir(file) {
			yakFile = utils.GetFirstExistedPath(filepath.Join(file, "main.yak"))
		}
	}
	if yakFile == "" {
		return nil, utils.Errorf("error for loading yakfile: %s", file)
	}

	absFile = yakFile
	var err error
	if !filepath.IsAbs(absFile) {
		absFile, err = filepath.Abs(absFile)
		if err != nil {
			return nil, utils.Errorf("fetch abs path[%s] failed: %v", yakFile, err)
		}
	}
	return ImportVarFromYakFile(absFile, exportsName)
}

// LoadVarFromFile 从指定文件或目录中批量加载脚本，并提取每个脚本中名为 exportsName 的变量
// 参数:
//   - path: yak 文件路径或目录(目录会遍历其中的 .yak 文件)
//   - exportsName: 要从每个脚本中提取的变量名
//   - opts: 可选配置，如 dyn.params(...)、dyn.recursive(...)
//
// 返回值:
//   - 提取到的变量列表，每个元素包含文件路径、模块名与变量值
//   - 加载失败时返回的错误
//
// Example:
// ```
// // 从目录加载所有脚本的 Exports 变量(依赖外部文件，作示意)
// vars, err = dyn.LoadVarFromFile("./mods", "Exports", dyn.recursive(true))
// ```
func LoadingVariableFrom(path string, exportsName string, opts ...yakEvalConfigOpt) ([]*yakVariable, error) {
	if path == "" {
		path = "."
	}

	// 加载配置
	config := &yakEvalConfig{}
	for _, opt := range opts {
		opt(config)
	}

	if config.params == nil {
		config.params = make(map[string]interface{})
	}

	var fileInfos []*utils.FileInfo
	var err error
	if utils.IsDir(path) {
		// 直接导入一个文件夹的 mod
		if config.recursive {
			fileInfos, err = utils.ReadDirsRecursively(path)
			if err != nil {
				return nil, utils.Errorf("read dir failed[recursively]: %v", err)
			}
		} else {
			fileInfos, err = utils.ReadDir(path)
			if err != nil {
				return nil, utils.Errorf("read dir failed: %s", err)
			}
		}
	} else {
		// 直接导入一个文件
		exitedFile := utils.GetFirstExistedPath(
			path, fmt.Sprintf("%v.yak", path),
		)
		if exitedFile != "" {
			file, err := os.Stat(exitedFile)
			if err != nil {
				return nil, utils.Errorf("fetch [%s] fileInfo failed: %s", exitedFile, err)
			}
			path := exitedFile
			if !filepath.IsAbs(path) {
				path, err = filepath.Abs(exitedFile)
				if err != nil || !utils.IsFile(path) {
					return nil, utils.Errorf("get abs path failed for: %s reason: %s", file.Name(), err)
				}

			}
			fileInfos = append(fileInfos, &utils.FileInfo{
				BuildIn: file,
				Path:    path,
				Name:    file.Name(),
				IsDir:   file.IsDir(),
			})
		}
	}

	var files []string
	for _, f := range fileInfos {
		if f.IsDir {
			continue
		}
		if strings.HasSuffix(strings.ToLower(f.Path), ".yak") {
			files = append(files, f.Path)
		}
	}

	if files == nil {
		return nil, utils.Errorf("cannot found yak source code by %v", path)
	}

	var vars []*yakVariable
	for _, fileName := range files {
		targetFile := utils.GetFirstExistedPath(fileName, fmt.Sprintf("%v.yak", fileName))
		if targetFile == "" {
			return nil, utils.Errorf("not a existed file: %v", fileName)
		}

		absFileName := fileName
		if !filepath.IsAbs(absFileName) {
			absFileName, err = filepath.Abs(absFileName)
			if err != nil {
				return nil, utils.Errorf("fetch abs file name[%v] failed: %s", absFileName, err)
			}
		}

		readFile, err := ioutil.ReadFile(fileName)
		if err != nil {
			return nil, utils.Errorf("loading yak source code failed: %s", err)
		}

		config.params["YAK_FILENAME"] = absFileName

		mergedParams := make(map[string]interface{})
		raw, _ := json.Marshal(config.params)
		if raw == nil {
			mergedParams["YAK_FILENAME"] = absFileName
		} else {
			_ = json.Unmarshal(raw, &mergedParams)
		}

		engine, err := NewScriptEngine(1).ExecuteEx(string(readFile), mergedParams)
		if err != nil {
			return nil, utils.Errorf("execute file %s code failed: %s", fileName, err.Error())
		}
		modName := fmt.Sprint(engine.Var("YAK_MOD"))
		if modName == spec.Undefined {
			modName = ""
		}
		value := engine.Var(exportsName)
		if value == spec.Undefined || value == nil || value == new(interface{}) {
			log.Errorf("loading yak file: %s failed: no variable[%v]", targetFile, exportsName)
			continue
		}
		vars = append(vars, &yakVariable{
			FilePath: fileName,
			YakMod:   modName,
			Value:    value,
		})
	}

	if vars == nil {
		log.Error("no variable found")
	}
	return vars, nil
}
