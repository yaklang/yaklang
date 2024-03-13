package yso

import (
	"github.com/yaklang/yaklang/common/log"
	yaml "github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yso/resources"
	"path"
	"strconv"
	"strings"
)

type ClassGenConfigParam struct {
	Name                           ClassParamType
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
	Name           string
	IsTemplateImpl bool
	Template       []byte
	ChainTemplate  map[string][]byte
	Desc           string
}

type ReflectChainFunctionConfig struct {
	Name string
	Desc string
	Args []*ClassGenConfigParam
}
type YsoConfig struct {
	Classes              map[ClassType]*ClassConfig
	Gadgets              map[GadgetType]*GadgetConfig
	ReflectChainFunction map[GadgetType]*ReflectChainFunctionConfig
}

var YsoConfigInstance *YsoConfig

func init() {
	var err error
	YsoConfigInstance, err = getConfig()
	if err != nil {
		log.Error(err)
	}
}
func getConfig() (*YsoConfig, error) {
	config := &YsoConfig{
		Classes:              map[ClassType]*ClassConfig{},
		Gadgets:              map[GadgetType]*GadgetConfig{},
		ReflectChainFunction: map[GadgetType]*ReflectChainFunctionConfig{},
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
			err := runWorkFlow(getStringOrEmptyTask(currentKey, value, "desc", func(currentKey []string, v string) error {
				reflectFunc.Desc = v
				return nil
			}), getListOrEmptyTask(currentKey, value, "params", func(currentKey []string, args []any) error {
				for _, arg := range args {
					p := &ClassGenConfigParam{}
					err := runWorkFlow(getStringOrEmptyTask(currentKey, arg, "name", func(currentKey []string, v string) error {
						p.Name = ClassParamType(v)
						return nil
					}), getStringTask(currentKey, arg, "desc", func(currentKey []string, v string) error {
						p.Desc = v
						return nil
					}))
					if err != nil {
						return err
					}
					reflectFunc.Args = append(reflectFunc.Args, p)
				}
				return nil
			}),
			)
			if err != nil {
				return err
			}
			config.ReflectChainFunction[GadgetType(reflectFunc.Name)] = reflectFunc
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
			config.Classes[ClassType(name)] = cfg
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
								paramConfig.Name = ClassParamType(v)
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
				getStringOrEmptyTask(currentKey, attr, "template", func(currentKey []string, v string) error {
					gadgetConfig.IsTemplateImpl = v == "true"
					return nil
				}),
			)
			if err != nil {
				return err
			}
			fileName := name
			if gadgetConfig.IsTemplateImpl {
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
					if err != nil && !strings.Contains(err.Error(), "file does not exist") {
						return utils.Errorf("read gadget %s template failed: %v", fileName, err)
					}
					gadgetConfig.ChainTemplate[chainInfo.Name] = templateBytes
				}
			}
			config.Gadgets[GadgetType(name)] = gadgetConfig
		}
		return nil
	})
	err = runWorkFlow(parseReflectChainFounction, parseClassesTask, parseGadgetTask)
	if err != nil {
		return nil, utils.Errorf("load yso config failed: %v", err)
	}

	return config, nil
}
