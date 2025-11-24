package tests

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func TestGenerateTestAlerts(t *testing.T) {
	config := AlertGeneratorConfig{
		TotalCount:      100,
		NoiseRatio:      0.9,
		OutputFile:      "/tmp/alerts.log",
		TimeSpanMinutes: 60,
	}

	err := GenerateTestAlerts(config)
	if err != nil {
		t.Fatalf("Failed to generate alerts: %v", err)
	}

	// 验证文件存在，且行数大于等于1000
	if !utils.FileExists(config.OutputFile) {
		t.Fatalf("File %s does not exist", config.OutputFile)
	}

	data, err := os.ReadFile(config.OutputFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if len(string(data)) < 1000 {
		t.Fatalf("Expected at least 1000 lines, got %d", len(string(data)))
	}
}
