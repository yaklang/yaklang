package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/antlr4yak"
	"yaklang.io/yaklang/common/yak/yaklang"
	"yaklang.io/yaklang/common/yak/yaklang/spec"
)

var GlobalEvalExports = map[string]interface{}{
	//"eval":   QuickEvalWithoutContext,
	"import": ImportVarFromFile,
}

var EvalExports = map[string]interface{}{
	"Eval":            QuickEvalWithoutContext,
	"LoadVarFromFile": LoadingVariableFrom,
	"Import":          ImportVarFromFile,
	"IsYakFunc":       yaklang.IsYakFunction,
	"params":          setYakEvalParams,
	"recursive":       setYakBatchImportRecursiveParams,
}

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

func setYakEvalParams(i map[string]interface{}) yakEvalConfigOpt {
	return func(y *yakEvalConfig) {
		y.params = i
	}
}

func setYakBatchImportRecursiveParams(i bool) yakEvalConfigOpt {
	return func(y *yakEvalConfig) {
		y.recursive = i
	}
}

type yakVariable struct {
	FilePath string
	YakMod   string
	Value    interface{}
	Engine   yaklang.YaklangEngine
}

func (y *yakVariable) Callable() bool {
	return yaklang.IsYakFunction(y.Value)
}
func ImportVarFromYakFile(path string, exportsName string) (interface{}, error) {
	engine := yaklang.New()
	if v, ok := engine.(*antlr4yak.Engine); ok {
		if err := v.RunFile(context.Background(), path); err != nil {
			return nil, utils.Errorf("load file \"%s\" failed: %s", path, err)
		}
	} else {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, utils.Errorf("read file[%s] failed: %s", path, err)
		}
		if err := engine.LoadCode(context.Background(), string(raw), nil); err != nil {
			return nil, utils.Errorf("load file \"%s\" failed: %s", path, err)
		}
	}
	v, ok := engine.GetVar(exportsName)
	if !ok {
		return nil, utils.Errorf("import var[%s] from file[%s] failed", exportsName, path)
	}
	return v, nil
}
func ImportVarFromScript(engine yaklang.YaklangEngine, script string, exportsName string) (interface{}, error) {
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

		var absFileName = fileName
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

		var mergedParams = make(map[string]interface{})
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
		var modName = fmt.Sprint(engine.Var("YAK_MOD"))
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
