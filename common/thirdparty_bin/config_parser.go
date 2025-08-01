package thirdparty_bin

import (
	"embed"
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

//go:embed bin_cfg.yml
var configFS embed.FS

// ConfigFile 配置文件结构
type ConfigFile struct {
	Version     string              `yaml:"version"`
	Description string              `yaml:"description"`
	Binaries    []*BinaryDescriptor `yaml:"binaries"`
}

// ConfigDownloadInfo YAML中的下载信息结构
type ConfigDownloadInfo struct {
	URL       string `yaml:"url"`
	Checksums string `yaml:"checksums,omitempty"`
	BinPath   string `yaml:"bin_path,omitempty"`
	BinDir    string `yaml:"bin_dir,omitempty"`
	Pick      string `yaml:"pick,omitempty"`
}

// ConfigBinaryDescriptor YAML中的二进制描述符结构
type ConfigBinaryDescriptor struct {
	Name            string                         `yaml:"name"`
	Description     string                         `yaml:"description"`
	Version         string                         `yaml:"version"`
	InstallType     string                         `yaml:"install_type"`
	ArchiveType     string                         `yaml:"archive_type,omitempty"`
	DownloadInfoMap map[string]*ConfigDownloadInfo `yaml:"download_info_map"`
	Dependencies    []string                       `yaml:"dependencies,omitempty"`
}

// LoadConfigFromEmbedded 从嵌入的配置文件加载配置
func LoadConfigFromEmbedded() (*ConfigFile, error) {
	data, err := configFS.ReadFile("bin_cfg.yml")
	if err != nil {
		return nil, utils.Errorf("failed to read embedded config file: %v", err)
	}

	return ParseConfig(data)
}

// ParseConfig 解析配置文件内容
func ParseConfig(data []byte) (*ConfigFile, error) {
	var configFile struct {
		Version     string                    `yaml:"version"`
		Description string                    `yaml:"description"`
		Binaries    []*ConfigBinaryDescriptor `yaml:"binaries"`
	}

	if err := yaml.Unmarshal(data, &configFile); err != nil {
		return nil, utils.Errorf("failed to parse config file: %v", err)
	}

	// 转换为标准的BinaryDescriptor格式
	result := &ConfigFile{
		Version:     configFile.Version,
		Description: configFile.Description,
		Binaries:    make([]*BinaryDescriptor, len(configFile.Binaries)),
	}

	for i, configBinary := range configFile.Binaries {
		binary := &BinaryDescriptor{
			Name:            configBinary.Name,
			Description:     configBinary.Description,
			Version:         configBinary.Version,
			InstallType:     configBinary.InstallType,
			ArchiveType:     configBinary.ArchiveType,
			DownloadInfoMap: make(map[string]*DownloadInfo),
			Dependencies:    configBinary.Dependencies,
		}

		// 转换下载信息并验证pick和BinDir的一致性
		for platform, configDownloadInfo := range configBinary.DownloadInfoMap {
			// 验证pick和BinDir必须同时存在或同时不存在
			hasPick := configDownloadInfo.Pick != ""
			hasBinDir := configDownloadInfo.BinDir != ""

			if hasPick != hasBinDir {
				return nil, utils.Errorf("binary %s platform %s: pick and bin_dir must both be present or both be absent",
					configBinary.Name, platform)
			}

			binary.DownloadInfoMap[platform] = &DownloadInfo{
				URL:       configDownloadInfo.URL,
				Checksums: configDownloadInfo.Checksums,
				BinDir:    configDownloadInfo.BinDir,
				BinPath:   configDownloadInfo.BinPath,
				Pick:      configDownloadInfo.Pick,
			}
		}

		result.Binaries[i] = binary
	}

	return result, nil
}

// LoadAndRegisterBuiltinBinaries 加载并注册内置的二进制工具
func LoadAndRegisterBuiltinBinaries() error {
	config, err := LoadConfigFromEmbedded()
	if err != nil {
		return utils.Errorf("failed to load builtin binaries config: %v", err)
	}

	var registeredCount int
	var failedCount int

	for _, binary := range config.Binaries {
		if err := Register(binary); err != nil {
			log.Warnf("Failed to register binary %s: %v", binary.Name, err)
			failedCount++
		} else {
			log.Debugf("Registered binary: %s (version: %s)", binary.Name, binary.Version)
			registeredCount++
		}
	}

	if failedCount > 0 {
		log.Warnf("Registered %d builtin binaries, %d failed", registeredCount, failedCount)
	}

	return nil
}

// GetBuiltinBinaryNames 获取所有内置二进制工具的名称列表
func GetBuiltinBinaryNames() ([]string, error) {
	config, err := LoadConfigFromEmbedded()
	if err != nil {
		return nil, err
	}

	names := make([]string, len(config.Binaries))
	for i, binary := range config.Binaries {
		names[i] = binary.Name
	}

	return names, nil
}

// GetBuiltinBinaryByName 根据名称获取内置二进制工具的描述符
func GetBuiltinBinaryByName(name string) (*BinaryDescriptor, error) {
	config, err := LoadConfigFromEmbedded()
	if err != nil {
		return nil, err
	}

	for _, binary := range config.Binaries {
		if binary.Name == name {
			return binary, nil
		}
	}

	return nil, utils.Errorf("builtin binary %s not found", name)
}

// PrintBuiltinBinaries 打印所有内置二进制工具的信息
func PrintBuiltinBinaries() error {
	config, err := LoadConfigFromEmbedded()
	if err != nil {
		return err
	}

	fmt.Printf("=== Builtin Binary Tools (Config Version: %s) ===\n", config.Version)
	fmt.Printf("Description: %s\n", config.Description)
	fmt.Printf("Total: %d tools\n\n", len(config.Binaries))

	for i, binary := range config.Binaries {
		fmt.Printf("%d. %s (v%s)\n", i+1, binary.Name, binary.Version)
		fmt.Printf("   Description: %s\n", binary.Description)
		fmt.Printf("   Install Type: %s\n", binary.InstallType)
		if binary.ArchiveType != "" {
			fmt.Printf("   Archive Type: %s\n", binary.ArchiveType)
		}

		fmt.Printf("   Supported Platforms:\n")
		for platform, downloadInfo := range binary.DownloadInfoMap {
			fmt.Printf("     - %s: %s", platform, downloadInfo.URL)
			if downloadInfo.Pick != "" {
				fmt.Printf(" (pick: %s)", downloadInfo.Pick)
			}
			fmt.Println()
		}

		if len(binary.Dependencies) > 0 {
			fmt.Printf("   Dependencies: %v\n", binary.Dependencies)
		}
		fmt.Println()
	}

	return nil
}
