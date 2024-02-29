package yso

import (
	"github.com/yaklang/yaklang/common/log"
	yaml "github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yso/resources"
	"path"
	"strconv"
)

type ClassGenConfigParam struct {
	Name                           string
	Desc                           string
	DefaultValue, TestDefaultValue any
}
type ClassConfig struct {
	FileName string
	Name     string
	Template []byte
	Desc     string
	Params   []*ClassGenConfigParam
}
type GadgetConfig struct {
	Name          string
	IsTemplate    bool
	Template      []byte
	ChainTemplate map[string][]byte
	Desc          string
	CCVersion     string
}

type ReflectChainFunctionConfig struct {
	Name string
	Desc string
	Args []string
}
type YsoConfig struct {
	Classes              map[string]*ClassConfig
	Gadgets              map[string]*GadgetConfig
	ReflectChainFunction map[string]*ReflectChainFunctionConfig
}

var YsoConfigInstance *YsoConfig

func init() {
	var err error
	YsoConfigInstance, err = getConfig()
	if err != nil {
		log.Errorf("load yso config failed: %v", err)
	}
}
func getConfig() (*YsoConfig, error) {
	config := &YsoConfig{
		Classes:              map[string]*ClassConfig{},
		Gadgets:              map[string]*GadgetConfig{},
		ReflectChainFunction: map[string]*ReflectChainFunctionConfig{},
	}
	content, err := resources.YsoResourceFS.ReadFile("config.yaml")
	if err != nil {
		return nil, utils.Errorf("read config.yaml failed: %v", err)
	}
	configMap := make(map[string]any)
	err = yaml.Unmarshal(content, &configMap)
	if err != nil {
		return nil, utils.Errorf("unmarshal config.yaml failed: %v", err)
	}
	parseReflectChainFounction := getMapOrEmptyTask([]string{}, configMap, "ReflectChainFunction", func(currentKey []string, dict map[string]any) error {
		for k, value := range dict {
			reflectFunc := &ReflectChainFunctionConfig{Name: k}
			err = getStringOrEmptyTask(currentKey, value, "desc", func(currentKey []string, v string) error {
				reflectFunc.Desc = v
				return nil
			})()
			if err != nil {
				return err
			}
			config.ReflectChainFunction[reflectFunc.Name] = reflectFunc
		}
		return nil
	})
	parseClassesTask := getMapOrEmptyTask([]string{}, configMap, "Classes", func(currentKey []string, classesInfo map[string]any) error {
		for name, attr := range classesInfo {
			cfg := &ClassConfig{}
			templateBytes, err := resources.YsoResourceFS.ReadFile(path.Join("classes", name+".class"))
			if err != nil {
				return utils.Errorf("read class %s failed: %v", name, err)
			}
			cfg.Template = templateBytes
			cfg.FileName = name + ".class"
			cfg.Name = name
			config.Classes[name] = cfg
			currentKey := append(currentKey, name)
			err = runWorkFlow(
				getStringOrEmptyTask(currentKey, attr, "name", func(currentKey []string, v string) error {
					cfg.Name = v
					return nil
				}),
				getStringOrEmptyTask(currentKey, attr, "desc", func(currentKey []string, v string) error {
					cfg.Desc = v
					return nil
				}),
				getListOrEmptyTask(currentKey, attr, "params", func(currentKey []string, params []any) error {
					var tasks []func() error // parse class attr tasks
					for index, param := range params {
						currentKey := append(currentKey, "["+strconv.Itoa(index)+"]")
						paramConfig := &ClassGenConfigParam{}
						cfg.Params = append(cfg.Params, paramConfig)
						tasks = append(
							tasks,
							getStringTask(currentKey, param, "name", func(currentKey []string, v string) error {
								paramConfig.Name = v
								return nil
							}),
							getStringOrEmptyTask(currentKey, param, "desc", func(currentKey []string, v string) error {
								paramConfig.Desc = v
								return nil
							}),
							getStringOrEmptyTask(currentKey, param, "default", func(currentKey []string, v string) error {
								paramConfig.DefaultValue = v
								return nil
							}),
							getStringOrEmptyTask(currentKey, param, "test-default", func(currentKey []string, v string) error {
								paramConfig.TestDefaultValue = v
								return nil
							}),
						)
					}
					err = runWorkFlow(tasks...)
					if err != nil {
						return err
					}
					return nil
				}),
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
	parseGadgetTask := getMapOrEmptyTask([]string{}, configMap, "Gadgets", func(currentKey []string, gadgetsMap map[string]any) error {
		for name, attr := range gadgetsMap {
			gadgetConfig := &GadgetConfig{
				Name:          name,
				ChainTemplate: map[string][]byte{},
			}
			currentKey := append(currentKey, name)
			err = runWorkFlow(
				getStringOrEmptyTask(currentKey, attr, "name", func(currentKey []string, v string) error {
					gadgetConfig.Name = v
					return nil
				}),
				getStringOrEmptyTask(currentKey, attr, "desc", func(currentKey []string, v string) error {
					if v == "<nil>" {
						println()
					}
					gadgetConfig.Desc = v
					return nil
				}),
				getStringOrEmptyTask(currentKey, attr, "group", func(currentKey []string, v string) error {
					switch v {
					case "1":
						gadgetConfig.CCVersion = "1"
					case "2":
						gadgetConfig.CCVersion = "4"
					default:
						gadgetConfig.CCVersion = "1"
					}
					return nil
				}),
				getStringOrEmptyTask(currentKey, attr, "template", func(currentKey []string, v string) error {
					gadgetConfig.IsTemplate = v == "true"
					return nil
				}),
			)
			if err != nil {
				return err
			}
			fileName := name
			if gadgetConfig.IsTemplate {
				fileName = "template_" + name
				templateBytes, err := resources.YsoResourceFS.ReadFile(path.Join("gadgets", fileName+".ser"))
				if err != nil {
					return utils.Errorf("read gadget %s template failed: %v", fileName, err)
				}
				gadgetConfig.Template = templateBytes
			} else {
				for _, chainInfo := range config.ReflectChainFunction {
					fileName = "transform_" + chainInfo.Name + "_" + name
					templateBytes, err := resources.YsoResourceFS.ReadFile(path.Join("gadgets", fileName+".ser"))
					if err != nil {
						return utils.Errorf("read gadget %s template failed: %v", fileName, err)
					}
					gadgetConfig.ChainTemplate[chainInfo.Name] = templateBytes
				}
			}
			config.Gadgets[name] = gadgetConfig
		}
		return nil
	})
	err = runWorkFlow(parseReflectChainFounction, parseClassesTask, parseGadgetTask)
	if err != nil {
		return nil, utils.Errorf("load yso config failed: %v", err)
	}

	return config, nil
}
