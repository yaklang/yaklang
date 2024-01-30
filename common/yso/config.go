package yso

import (
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
	Name       string
	IsTemplate bool
	Template   []byte
	Desc       string
}
type YsoConfig struct {
	Classes map[string]*ClassConfig
	Gadgets map[string]*GadgetConfig
}

var YsoConfigInstance *YsoConfig

func init() {
	YsoConfigInstance, _ = GetConfig()
}
func GetConfig() (*YsoConfig, error) {
	config := &YsoConfig{
		Classes: map[string]*ClassConfig{},
		Gadgets: map[string]*GadgetConfig{},
	}
	content, err := resources.YsoResourceFS.ReadFile("config.yaml")
	if err != nil {
		return nil, err
	}
	configMap := make(map[string]any)
	err = yaml.Unmarshal(content, &configMap)
	if err != nil {
		return nil, err
	}
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
				Name: name,
			}
			templateBytes, err := resources.YsoResourceFS.ReadFile(path.Join("gadgets", name+".ser"))
			if err != nil {
				return utils.Errorf("read gadget %s template failed: %v", name, err)
			}
			gadgetConfig.Template = templateBytes
			currentKey := append(currentKey, name)
			err = runWorkFlow(
				getStringOrEmptyTask(currentKey, attr, "name", func(currentKey []string, v string) error {
					gadgetConfig.Name = v
					return nil
				}),
				getStringOrEmptyTask(currentKey, attr, "desc", func(currentKey []string, v string) error {
					gadgetConfig.Desc = v
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
			config.Gadgets[name] = gadgetConfig
		}
		return nil
	})
	err = runWorkFlow(parseClassesTask, parseGadgetTask)
	if err != nil {
		return nil, err
	}

	return config, nil
}
