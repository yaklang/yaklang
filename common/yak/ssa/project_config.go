package ssa

import (
	"encoding/json"
	"fmt"
	"strings"

	yaml "github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"github.com/yaklang/yaklang/common/utils"
)

type ProjectConfigType int

const (
	PROJECT_CONFIG_YAML ProjectConfigType = iota
	PROJECT_CONFIG_JSON
	PROJECT_CONFIG_PROPERTIES
)

type ProjectConfig struct {
	ConfigValue string
	Filepath    string
}

func (p *Program) GetProjectConfig(key string) *ProjectConfig {
	if p == nil {
		return nil
	}
	return p.ProjectConfig[key]
}

func (p *Program) GetProjectConfigValue(key string) string {
	if p == nil {
		return ""
	}
	config := p.GetProjectConfig(key)
	if config == nil {
		return ""
	}
	return config.ConfigValue
}

func (p *Program) SetProjectConfig(key string, value string, filePath string) {
	if p == nil {
		return
	}
	config := &ProjectConfig{
		ConfigValue: value,
		Filepath:    filePath,
	}
	p.ProjectConfig[key] = config
}

func (p *Program) ParseProjectConfig(raw []byte, path string, typ ProjectConfigType) error {
	switch typ {
	case PROJECT_CONFIG_PROPERTIES:
		err := p.parsePropertiesProjectConfig(raw, path)
		if err != nil {
			return err
		}
	case PROJECT_CONFIG_YAML:
		err := p.parseYamlProjectConfig(raw, path)
		if err != nil {
			return err
		}
	case PROJECT_CONFIG_JSON:
		err := p.parseJsonProjectConfig(raw, path)
		if err != nil {
			return err
		}
	default:
		return utils.Errorf("not support project config type: %d", typ)
	}
	return nil
}

func (p *Program) parsePropertiesProjectConfig(raw []byte, path string) error {
	if p == nil {
		return utils.Errorf("program is nil")
	}
	content := string(raw)
	lines := strings.Split(content, "\n")
	var errs error
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			errs = utils.JoinErrors(errs, utils.Errorf("bad properties line: %s", line))
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		p.SetProjectConfig(key, value, path)
	}
	return nil
}

func (p *Program) parseYamlProjectConfig(raw []byte, path string) error {
	if p == nil {
		return utils.Errorf("program is nil")
	}

	config, err := yaml.YamlToKVParis(raw)
	if err != nil {
		return utils.Errorf("parse yaml project config error: %v", err)
	}
	for k, v := range config {
		p.SetProjectConfig(k, v, path)
	}
	return nil
}

func (p *Program) parseJsonProjectConfig(raw []byte, path string) error {
	if p == nil {
		return utils.Errorf("program is nil")
	}

	var jsonObj map[string]interface{}
	err := json.Unmarshal(raw, &jsonObj)
	if err != nil {
		return fmt.Errorf("error unmarshaling JSON: %v", err)
	}

	result := make(map[string]string)
	yaml.FlattenJSON(jsonObj, "", result)
	for k, v := range result {
		p.SetProjectConfig(k, v, path)
	}
	return nil
}
