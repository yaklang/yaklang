package yakcmds

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v3"
)

type AIModelConfigEntry = aiconfig.AIModelConfigEntry
type TieredAIConfigFile = aiconfig.TieredAIConfigFile

func configEntryToThirdPartyConfig(entry AIModelConfigEntry) *ypb.ThirdPartyApplicationConfig {
	return aiconfig.ConfigEntryToThirdPartyConfig(entry)
}

func thirdPartyConfigToEntry(cfg *ypb.ThirdPartyApplicationConfig) AIModelConfigEntry {
	return aiconfig.ThirdPartyConfigToEntry(cfg)
}

func configFileToTieredAIConfig(cfg *TieredAIConfigFile) *consts.TieredAIConfig {
	return aiconfig.ConfigFileToTieredAIConfig(cfg)
}

func getDefaultTieredAIConfigFile() *TieredAIConfigFile {
	return aiconfig.GetDefaultTieredAIConfigFile()
}

func getDefaultConfigDir() string {
	return aiconfig.GetDefaultConfigDir()
}

func getDefaultConfigPaths() []string {
	return aiconfig.GetDefaultConfigPaths()
}

func resolveConfigFilePath(specified string) string {
	return aiconfig.ResolveConfigFilePath(specified)
}

func loadTieredAIConfigFile(path string) (*TieredAIConfigFile, error) {
	return aiconfig.LoadTieredAIConfigFile(path)
}

func saveTieredAIConfigFile(path string, cfg *TieredAIConfigFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return utils.Errorf("failed to create config directory %s: %v", dir, err)
	}

	var data []byte
	var err error
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		data, err = json.MarshalIndent(cfg, "", "  ")
	default:
		data, err = yaml.Marshal(cfg)
	}
	if err != nil {
		return utils.Errorf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return utils.Errorf("failed to write config file %s: %v", path, err)
	}
	return nil
}

func buildAIOptionsFromEntry(entry AIModelConfigEntry) []aispec.AIConfigOption {
	var opts []aispec.AIConfigOption
	if entry.Type != "" {
		opts = append(opts, aispec.WithType(entry.Type))
	}
	if entry.APIKey != "" {
		opts = append(opts, aispec.WithAPIKey(entry.APIKey))
	}
	if entry.Domain != "" {
		opts = append(opts, aispec.WithDomain(entry.Domain))
	}
	if entry.Model != "" {
		opts = append(opts, aispec.WithModel(entry.Model))
	}
	return opts
}

func printTieredAIConfigStatus(cfg *TieredAIConfigFile, configPath string) {
	enabledStr := "false"
	if cfg.Enabled {
		enabledStr = "true"
	}
	fallbackStr := "false"
	if cfg.DisableFallback {
		fallbackStr = "true"
	}

	fmt.Println("Tiered AI Configuration Status")
	fmt.Println("===============================")
	fmt.Printf("  Enabled:           %s\n", enabledStr)
	fmt.Printf("  Config File:       %s\n", configPath)
	fmt.Printf("  Routing Policy:    %s\n", cfg.RoutingPolicy)
	fmt.Printf("  Disable Fallback:  %s\n", fallbackStr)
	fmt.Println()

	printTierConfigs("Intelligent", cfg.IntelligentConfigs)
	printTierConfigs("Lightweight", cfg.LightweightConfigs)
	printTierConfigs("Vision", cfg.VisionConfigs)
}

func printTierConfigs(tierName string, entries []AIModelConfigEntry) {
	fmt.Printf("%s Tier (%d config(s)):\n", tierName, len(entries))
	if len(entries) == 0 {
		fmt.Println("  (none)")
	}
	for i, e := range entries {
		apiKeyDisplay := "(empty)"
		if e.APIKey != "" {
			if len(e.APIKey) > 8 {
				apiKeyDisplay = e.APIKey[:4] + "****" + e.APIKey[len(e.APIKey)-4:]
			} else {
				apiKeyDisplay = e.APIKey
			}
		}
		fmt.Printf("  [%d] type=%-12s domain=%-30s model=%-25s api_key=%s\n",
			i, e.Type, e.Domain, e.Model, apiKeyDisplay)
	}
	fmt.Println()
}

type tierCheckResult struct {
	TierName string
	Model    string
	Type     string
	OK       bool
	Duration time.Duration
	Detail   string
	Error    string
}

func checkTextTier(tierName string, entries []AIModelConfigEntry) tierCheckResult {
	if len(entries) == 0 {
		return tierCheckResult{TierName: tierName, OK: false, Error: "no configuration"}
	}
	entry := entries[0]
	opts := buildAIOptionsFromEntry(entry)
	opts = append(opts, aispec.WithTimeout(30))

	start := time.Now()
	resp, err := ai.Chat("Ping. Please respond with Pong.", opts...)
	elapsed := time.Since(start)

	if err != nil {
		return tierCheckResult{
			TierName: tierName, Model: entry.Model, Type: entry.Type,
			OK: false, Duration: elapsed, Error: fmt.Sprintf("%v", err),
		}
	}
	if resp == "" {
		return tierCheckResult{
			TierName: tierName, Model: entry.Model, Type: entry.Type,
			OK: false, Duration: elapsed, Error: "empty response",
		}
	}
	return tierCheckResult{
		TierName: tierName, Model: entry.Model, Type: entry.Type,
		OK: true, Duration: elapsed, Detail: fmt.Sprintf("responded in %.1fs", elapsed.Seconds()),
	}
}

// visionTestImageBase64 is the same test image used in the built-in plugin "知识库可用性诊断"
// It contains the Chinese text "数据库" for OCR verification.
const visionTestImageBase64 = `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`

func checkVisionTier(entries []AIModelConfigEntry) tierCheckResult {
	tierName := "Vision"
	if len(entries) == 0 {
		return tierCheckResult{TierName: tierName, OK: false, Error: "no configuration"}
	}
	entry := entries[0]
	opts := buildAIOptionsFromEntry(entry)
	opts = append(opts, aispec.WithImageBase64(visionTestImageBase64))
	opts = append(opts, aispec.WithTimeout(60))

	funcs := map[string]string{
		"ai_ocr_data": "put recognized text from the image here, type is string",
	}

	start := time.Now()
	results, err := ai.FunctionCall("Recognize the text in the image", funcs, opts...)
	elapsed := time.Since(start)

	if err != nil {
		return tierCheckResult{
			TierName: tierName, Model: entry.Model, Type: entry.Type,
			OK: false, Duration: elapsed, Error: fmt.Sprintf("%v", err),
		}
	}
	if results == nil {
		return tierCheckResult{
			TierName: tierName, Model: entry.Model, Type: entry.Type,
			OK: false, Duration: elapsed, Error: "empty result from AI",
		}
	}

	ocrData, ok := results["ai_ocr_data"]
	if !ok || ocrData == nil {
		return tierCheckResult{
			TierName: tierName, Model: entry.Model, Type: entry.Type,
			OK: false, Duration: elapsed, Error: "AI did not return OCR data",
		}
	}

	ocrStr := fmt.Sprintf("%v", ocrData)
	if !strings.Contains(ocrStr, "数据库") {
		return tierCheckResult{
			TierName: tierName, Model: entry.Model, Type: entry.Type,
			OK: false, Duration: elapsed,
			Error: fmt.Sprintf("OCR result does not contain expected text, got: %s", strings.TrimSpace(ocrStr)),
		}
	}

	return tierCheckResult{
		TierName: tierName, Model: entry.Model, Type: entry.Type,
		OK: true, Duration: elapsed,
		Detail: fmt.Sprintf("OCR recognized target text in %.1fs", elapsed.Seconds()),
	}
}

func runAllChecks(cfg *TieredAIConfigFile) []tierCheckResult {
	var results []tierCheckResult
	results = append(results, checkTextTier("Intelligent", cfg.IntelligentConfigs))
	results = append(results, checkTextTier("Lightweight", cfg.LightweightConfigs))
	results = append(results, checkVisionTier(cfg.VisionConfigs))
	return results
}

func printCheckResults(results []tierCheckResult) bool {
	allOK := true
	for _, r := range results {
		status := "[OK]  "
		detail := r.Detail
		if !r.OK {
			status = "[FAIL]"
			detail = r.Error
			allOK = false
		}
		modelInfo := ""
		if r.Model != "" {
			modelInfo = fmt.Sprintf("%s (%s)", r.Model, r.Type)
		}
		fmt.Printf("  %s %-12s %-40s %s\n", status, r.TierName+":", modelInfo, detail)
	}
	fmt.Println()
	if allOK {
		fmt.Println("All tiers are available.")
	} else {
		fmt.Println("Some tiers failed the check. See errors above.")
	}
	return allOK
}

func applyConfigToMemory(cfg *TieredAIConfigFile) {
	tiered := configFileToTieredAIConfig(cfg)
	consts.SetTieredAIConfig(tiered)
	log.Infof("tiered AI config applied to memory: enabled=%v, policy=%s", tiered.Enabled, tiered.RoutingPolicy)
}

func saveConfigToDB(cfg *TieredAIConfigFile) error {
	return aiconfig.SaveTieredAIConfigToDB(cfg)
}

var configFileFlag = cli.StringFlag{
	Name:  "config-file",
	Usage: "Path to the tiered AI config file (YAML or JSON)",
}

var TieredAIConfigCommands = []*cli.Command{
	{
		Name:  "list-tiered-ai-config",
		Usage: "List current tiered AI configuration status",
		Flags: []cli.Flag{configFileFlag},
		Action: func(c *cli.Context) error {
			configPath := resolveConfigFilePath(c.String("config-file"))
			if utils.GetFirstExistedFile(configPath) == "" {
				fmt.Println("Tiered AI Configuration Status")
				fmt.Println("===============================")
				fmt.Printf("  Config File:  %s (not found)\n", configPath)
				fmt.Println("  Status:       not configured, system defaults will be used")
				fmt.Println()
				fmt.Println("Default configuration (aibalance):")
				fmt.Println()
				defaultCfg := getDefaultTieredAIConfigFile()
				printTieredAIConfigStatus(defaultCfg, "(default)")
				return nil
			}

			cfg, err := loadTieredAIConfigFile(configPath)
			if err != nil {
				return err
			}
			printTieredAIConfigStatus(cfg, configPath)
			return nil
		},
	},
	{
		Name:  "check-tiered-ai-config",
		Usage: "Check if current tiered AI configuration is available",
		Flags: []cli.Flag{configFileFlag},
		Action: func(c *cli.Context) error {
			configPath := resolveConfigFilePath(c.String("config-file"))
			var cfg *TieredAIConfigFile
			if utils.GetFirstExistedFile(configPath) != "" {
				var err error
				cfg, err = loadTieredAIConfigFile(configPath)
				if err != nil {
					return err
				}
				fmt.Printf("Checking tiered AI configuration from: %s\n\n", configPath)
			} else {
				cfg = getDefaultTieredAIConfigFile()
				fmt.Println("Config file not found, checking default (aibalance) configuration...")
			fmt.Println()
			}

			applyConfigToMemory(cfg)

			fmt.Println("Checking Tiered AI Configuration...")
			results := runAllChecks(cfg)
			printCheckResults(results)
			return nil
		},
	},
	{
		Name:  "tiered-ai-config",
		Usage: "Configure tiered AI model settings",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  "disabled",
				Usage: "Disable the tiered AI configuration",
			},
			cli.BoolFlag{
				Name:  "enable",
				Usage: "Enable the tiered AI configuration (use existing DB config or defaults)",
			},
			configFileFlag,
		},
		Action: func(c *cli.Context) error {
			specifiedPath := c.String("config-file")

			if c.Bool("disabled") {
				cfg := getDefaultTieredAIConfigFile()
				cfg.Enabled = false

				if err := saveConfigToDB(cfg); err != nil {
					return err
				}
				fmt.Println("Tiered AI configuration disabled and saved to database.")
				return nil
			}

			if c.Bool("enable") {
				cfg := getDefaultTieredAIConfigFile()
				if specifiedPath != "" && utils.GetFirstExistedFile(specifiedPath) != "" {
					var err error
					cfg, err = loadTieredAIConfigFile(specifiedPath)
					if err != nil {
						return err
					}
					fmt.Printf("Loading configuration from file: %s\n", specifiedPath)
				} else {
					fmt.Println("Using default (aibalance) configuration")
				}
				cfg.Enabled = true
				if err := saveConfigToDB(cfg); err != nil {
					return err
				}
				fmt.Println("Tiered AI configuration enabled and saved to database.")
				return nil
			}

			var cfg *TieredAIConfigFile
			if specifiedPath != "" && utils.GetFirstExistedFile(specifiedPath) != "" {
				var err error
				cfg, err = loadTieredAIConfigFile(specifiedPath)
				if err != nil {
					return err
				}
				fmt.Printf("Loading configuration from file: %s\n", specifiedPath)
			} else {
				cfg = getDefaultTieredAIConfigFile()
				fmt.Println("No config file specified, using default (aibalance) configuration")
			}

			cfg.Enabled = true
			applyConfigToMemory(cfg)

			fmt.Println("\nChecking configuration before enabling...")
			fmt.Println("Checking Tiered AI Configuration...")
			results := runAllChecks(cfg)
			allOK := printCheckResults(results)

			if !allOK {
				return utils.Error("configuration check failed, tiered AI config not enabled")
			}

			if err := saveConfigToDB(cfg); err != nil {
				return err
			}
			fmt.Println("\nTiered AI configuration enabled and saved to database.")
			return nil
		},
	},
	{
		Name:  "dump-tiered-ai-config",
		Usage: "Dump a tiered AI config template file for user customization",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "output,o",
				Usage: "Output file path (default: stdout). Format is inferred from extension (.yaml/.json)",
			},
			cli.StringFlag{
				Name:  "format,f",
				Value: "yaml",
				Usage: "Output format when writing to stdout: yaml or json",
			},
			configFileFlag,
		},
		Action: func(c *cli.Context) error {
			var cfg *TieredAIConfigFile
			specifiedPath := c.String("config-file")
			if specifiedPath != "" && utils.GetFirstExistedFile(specifiedPath) != "" {
				var err error
				cfg, err = loadTieredAIConfigFile(specifiedPath)
				if err != nil {
					return err
				}
			} else {
				existingPath := resolveConfigFilePath("")
				if utils.GetFirstExistedFile(existingPath) != "" {
					var err error
					cfg, err = loadTieredAIConfigFile(existingPath)
					if err != nil {
						cfg = getDefaultTieredAIConfigFile()
					}
				} else {
					cfg = getDefaultTieredAIConfigFile()
				}
			}

			outputPath := c.String("output")
			if outputPath != "" {
				if err := saveTieredAIConfigFile(outputPath, cfg); err != nil {
					return err
				}
				fmt.Printf("Tiered AI config dumped to: %s\n", outputPath)
				return nil
			}

			format := strings.ToLower(c.String("format"))
			var data []byte
			var err error
			switch format {
			case "json":
				data, err = json.MarshalIndent(cfg, "", "  ")
			default:
				data, err = yaml.Marshal(cfg)
			}
			if err != nil {
				return utils.Errorf("failed to marshal config: %v", err)
			}
			fmt.Println(string(data))
			return nil
		},
	},
	{
		Name:  "reset-tiered-ai-config",
		Usage: "Reset tiered AI configuration to default (aibalance)",
		Action: func(c *cli.Context) error {
			cfg := getDefaultTieredAIConfigFile()

			if err := saveConfigToDB(cfg); err != nil {
				return err
			}

			fmt.Println("Tiered AI configuration reset to default (aibalance)")
			fmt.Println("Config saved to database.")
			fmt.Println()
			printTieredAIConfigStatus(cfg, "(database)")
			return nil
		},
	},
}
