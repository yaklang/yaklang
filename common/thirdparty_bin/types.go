package thirdparty_bin

import (
	"context"
	"runtime"
)

type DownloadInfo struct {
	// URL
	URL string `json:"url"`
	// MD5 校验和
	MD5 string `json:"md5,omitempty"`
	// SHA256 校验和
	SHA256 string `json:"sha256,omitempty"`
	// BinPath
	BinPath string `json:"bin_path,omitempty"`
	// 二进制文件目录
	BinDir string `json:"bin_dir,omitempty"`
	// 提取文件
	Pick string `json:"pick,omitempty"`
	// 安装类型
	InstallType string `json:"install_type,omitempty"`
	// 解压密码（仅 zip archive 生效），用于 AES 加密包的解压
	// 注意: 在当前用例中（如 hack-skills），密码是公开常量，仅用于绕过 AV 启发式扫描，并非访问控制
	// 关键词: 解压密码, 加密 zip 安装, anti-AV public password
	Password string `json:"password,omitempty"`
}

// BinaryDescriptor 描述一个二进制文件的信息
type BinaryDescriptor struct {
	// 二进制文件名称
	Name string `json:"name"`
	// 描述
	Description string `json:"description"`
	// 标签
	Tags []string `json:"tags,omitempty"`
	// 版本
	Version string `json:"version"`
	// 各个平台的下载信息
	DownloadInfoMap map[string]*DownloadInfo `json:"download_info_map"`
	// 安装类型 (executable, archive, installer)
	InstallType string `json:"install_type"`
	// archive 类型
	ArchiveType string `json:"archive_type,omitempty"`
	// 依赖的其他二进制文件
	Dependencies []string `json:"dependencies,omitempty"`
	// InstallRoot 覆盖默认安装根目录
	// 取值 "ai-skills" 安装到 consts.GetDefaultAISkillsDir() (~/yakit-projects/ai-skills/)
	// 取值 "libs" 或空字符串安装到默认 libs 目录 (~/yakit-projects/libs/)
	// 关键词: install_root, ai-skills, libs, 安装根目录覆盖
	InstallRoot string `json:"install_root,omitempty"`
}

// ProgressCallback 下载进度回调函数
// progress: 下载进度 0.0-1.0
// downloaded: 已下载字节数
// total: 总字节数
// message: 状态消息
type ProgressCallback func(progress float64, downloaded, total int64, message string)

// DownloadOptions 下载选项
type DownloadOptions struct {
	// 代理地址
	Proxy string
	// 进度回调
	Progress ProgressCallback
	// 上下文，用于取消下载
	Context context.Context
	// 强制重新下载
	Force bool
}

// InstallOptions 安装选项
type InstallOptions struct {
	// 是否强制安装（覆盖已存在的文件）
	Force bool
	// 代理地址
	Proxy string
	// 进度回调
	Progress ProgressCallback
	// 上下文，用于取消下载
	Context context.Context
	// 系统类型
	SystemType string
}

// SystemInfo 系统信息
type SystemInfo struct {
	OS   string // windows, linux, darwin
	Arch string // amd64, arm64, 386
}

// GetCurrentSystemInfo 获取当前系统信息
func GetCurrentSystemInfo() SystemInfo {
	return SystemInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
}

// GetPlatformKey 获取平台标识符
func (si SystemInfo) GetPlatformKey() string {
	return si.OS + "-" + si.Arch
}

// BinaryStatus 二进制文件状态
type BinaryStatus struct {
	// 二进制文件名称
	Name string `json:"name"`
	// 是否已安装
	Installed bool `json:"installed"`
	// 安装版本
	InstalledVersion string `json:"installed_version,omitempty"`
	// 可用版本
	AvailableVersion string `json:"available_version"`
	// 安装路径
	InstallPath string `json:"install_path,omitempty"`
	// 是否需要更新
	NeedsUpdate bool `json:"needs_update"`
}
