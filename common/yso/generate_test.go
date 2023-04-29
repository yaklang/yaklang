package yso

import (
	"yaklang/common/utils"
	"testing"
)

func TestGenerateClass(t *testing.T) {
	cfg := NewClassConfig()
	cfg.Command = "1"
	cfg.Domain = "1"
	cfg.Host = "1"
	cfg.Port = 1
	cfg.ClassBytes = template_class_EmptyClassInTemplate
	for _, payload := range GetAllClassGenerator() {
		_, err := payload.Generator(cfg)
		if err != nil {
			t.Error(utils.Errorf("payload[%s] generate failed: %s", payload.ClassName, err.Error()))
		}
	}
}
