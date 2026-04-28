package thirdparty_bin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

// 测试 resolveInstallDir 在不同 InstallRoot 取值下的回落逻辑
// 关键词: resolveInstallDir, install_root 切换, ai-skills, libs fallback
func TestResolveInstallDir(t *testing.T) {
	defaultDir := t.TempDir()
	bi := &BaseInstaller{defaultInstallDir: defaultDir}

	t.Run("empty install_root falls back to libs default", func(t *testing.T) {
		desc := &BinaryDescriptor{Name: "x", InstallRoot: ""}
		got := bi.resolveInstallDir(desc)
		assert.Equal(t, defaultDir, got)
	})

	t.Run("install_root=libs equals libs default", func(t *testing.T) {
		desc := &BinaryDescriptor{Name: "x", InstallRoot: "libs"}
		got := bi.resolveInstallDir(desc)
		assert.Equal(t, defaultDir, got)
	})

	t.Run("install_root=ai-skills routes to ai-skills dir", func(t *testing.T) {
		desc := &BinaryDescriptor{Name: "hack-skills", InstallRoot: "ai-skills"}
		got := bi.resolveInstallDir(desc)
		assert.Equal(t, consts.GetDefaultAISkillsDir(), got)
		assert.NotEqual(t, defaultDir, got, "ai-skills must not be the libs default")
	})

	t.Run("unknown install_root falls back with warning", func(t *testing.T) {
		desc := &BinaryDescriptor{Name: "x", InstallRoot: "no-such-root"}
		got := bi.resolveInstallDir(desc)
		assert.Equal(t, defaultDir, got)
	})

	t.Run("nil descriptor falls back safely", func(t *testing.T) {
		got := bi.resolveInstallDir(nil)
		assert.Equal(t, defaultDir, got)
	})
}

// 测试 GetTargetPath / GetInstallDir 在 install_root="ai-skills" 时
// 一定锚定到 consts.GetDefaultAISkillsDir() 而不是 libs 目录
// 关键词: GetTargetPath ai-skills, GetInstallDir ai-skills, 安装目录覆盖
func TestInstaller_AISkillsRoot_PathCalculation(t *testing.T) {
	defaultDir := t.TempDir()
	bi := &BaseInstaller{defaultInstallDir: defaultDir, downloadDir: t.TempDir()}

	desc := &BinaryDescriptor{
		Name:        "hack-skills",
		InstallType: "archive",
		ArchiveType: ".zip",
		InstallRoot: "ai-skills",
		DownloadInfoMap: map[string]*DownloadInfo{
			"*": {
				URL:      "https://example.com/hack-skills.zip",
				Password: "hack-skills",
				Pick:     "*",
				BinDir:   "hack-skills",
				BinPath:  "hack-skills/version.txt",
			},
		},
	}

	target := bi.GetTargetPath(desc, nil)
	installDir := bi.GetInstallDir(desc, nil)
	aiSkills := consts.GetDefaultAISkillsDir()

	assert.True(t, strings.HasPrefix(target, aiSkills),
		"target path must live under ai-skills root; got %q (ai-skills=%q)", target, aiSkills)
	assert.False(t, strings.HasPrefix(target, defaultDir),
		"target path must NOT live under libs default; got %q (libs=%q)", target, defaultDir)
	assert.Equal(t, filepath.Join(aiSkills, "hack-skills", "version.txt"), target)
	assert.Equal(t, filepath.Join(aiSkills, "hack-skills"), installDir)
}

// 测试 IsInstalled 锚点判断：bin_path 必须指向真实文件，目录不算（GetFirstExistedFile 跳过 dir）
// 关键词: IsInstalled anchor, bin_path 文件锚点, version.txt manifest, 目录不算锚
func TestInstaller_IsInstalled_AnchorFile(t *testing.T) {
	installRoot := t.TempDir()
	bi := &BaseInstaller{defaultInstallDir: installRoot, downloadDir: t.TempDir()}

	desc := &BinaryDescriptor{
		Name:        "hack-skills",
		InstallType: "archive",
		ArchiveType: ".zip",
		// 故意留空 InstallRoot，让 GetTargetPath 用 t.TempDir(),
		// 避免污染真实 ~/yakit-projects/ai-skills 目录
		InstallRoot: "",
		DownloadInfoMap: map[string]*DownloadInfo{
			"*": {
				URL:     "https://example.com/hack-skills.zip",
				BinDir:  "hack-skills",
				BinPath: "hack-skills/version.txt",
			},
		},
	}

	t.Run("absent anchor file -> not installed", func(t *testing.T) {
		assert.False(t, bi.IsInstalled(desc, nil))
	})

	t.Run("anchor as directory only -> still not installed (GetFirstExistedFile skips dirs)", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(installRoot, "hack-skills", "version.txt"), 0o755))
		assert.False(t, bi.IsInstalled(desc, nil),
			"GetFirstExistedFile must skip directories; otherwise IsInstalled would falsely report installed")
		require.NoError(t, os.RemoveAll(filepath.Join(installRoot, "hack-skills")))
	})

	t.Run("anchor present as file -> installed", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(installRoot, "hack-skills"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(installRoot, "hack-skills", "version.txt"),
			[]byte("20260428-deadbee\n"),
			0o644,
		))
		assert.True(t, bi.IsInstalled(desc, nil))
	})

	t.Run("anchor removed -> not installed again", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(filepath.Join(installRoot, "hack-skills")))
		assert.False(t, bi.IsInstalled(desc, nil))
	})
}

// 端到端：用本地 httptest 起一个返回 AES-256 加密 zip 的服务器，走真实 Install 链路，
// 验证 password 字段从 DownloadInfo 透传到 ExtractFileWithPassword 后能成功解密落盘
// 关键词: archive install with password, ExtractFileWithPassword 透传, 加密 zip 安装链路
func TestInstaller_ArchiveInstall_PasswordPassthrough(t *testing.T) {
	files := map[string]string{
		"version.txt":               "20260428-cafebab\n",
		"skills/api-sec/SKILL.md":   "# API Security skill",
		"skills/recon/SKILL.md":     "# Recon skill",
		"skills/recon/notes/raw.md": "private notes",
	}
	password := "hack-skills"
	zipData := makeEncryptedZip(t, files, password)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(zipData)))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", strconv.Itoa(len(zipData)))
		_, _ = w.Write(zipData)
	}))
	defer srv.Close()

	installRoot := t.TempDir()
	downloadDir := t.TempDir()
	bi := &BaseInstaller{defaultInstallDir: installRoot, downloadDir: downloadDir}

	desc := &BinaryDescriptor{
		Name:        "hack-skills",
		InstallType: "archive",
		ArchiveType: ".zip",
		// 留空 InstallRoot，让安装目录走 t.TempDir()，避免污染本机 ai-skills 目录
		InstallRoot: "",
		DownloadInfoMap: map[string]*DownloadInfo{
			"*": {
				URL:      srv.URL + "/hack-skills.zip",
				Password: password,
				Pick:     "*",
				BinDir:   "hack-skills",
				BinPath:  "hack-skills/version.txt",
			},
		},
	}

	require.NoError(t, bi.Install(desc, &InstallOptions{Force: true}))

	manifestPath := filepath.Join(installRoot, "hack-skills", "version.txt")
	got, err := os.ReadFile(manifestPath)
	require.NoError(t, err, "version.txt manifest must exist after install")
	assert.Equal(t, "20260428-cafebab\n", string(got))

	skillPath := filepath.Join(installRoot, "hack-skills", "skills", "api-sec", "SKILL.md")
	gotSkill, err := os.ReadFile(skillPath)
	require.NoError(t, err, "extracted SKILL.md must be reachable for AutoSkillLoader")
	assert.Equal(t, "# API Security skill", string(gotSkill))

	assert.True(t, bi.IsInstalled(desc, nil),
		"after archive extract, IsInstalled must report true via version.txt anchor")
}

// 验证 yaml 解析层把 password / install_root 正确透传到 BinaryDescriptor / DownloadInfo
// 关键词: ParseConfig password, ParseConfig install_root, yaml 配置透传
func TestParseConfig_PasswordAndInstallRootPropagation(t *testing.T) {
	yamlStr := `version: "1.0"
description: "test"
baseurl: "https://example.com"
binaries:
  - name: "hack-skills"
    description: "test pack"
    tags: "skill,ai-skills"
    version: "latest"
    install_type: "archive"
    archive_type: ".zip"
    install_root: "ai-skills"
    download_info_map:
      "*":
        url: "/hack-skills/latest/hack-skills.zip"
        password: "hack-skills"
        pick: "*"
        bin_dir: "hack-skills"
        bin_path: "hack-skills/version.txt"
`

	cfg, err := ParseConfig([]byte(yamlStr))
	require.NoError(t, err)
	require.Len(t, cfg.Binaries, 1)

	b := cfg.Binaries[0]
	assert.Equal(t, "hack-skills", b.Name)
	assert.Equal(t, "ai-skills", b.InstallRoot, "install_root must be parsed from yaml")
	assert.Equal(t, "archive", b.InstallType)
	assert.Equal(t, ".zip", b.ArchiveType)

	dl, ok := b.DownloadInfoMap["*"]
	require.True(t, ok)
	assert.Equal(t, "hack-skills", dl.Password, "password must be parsed and propagated to DownloadInfo")
	assert.Equal(t, "*", dl.Pick)
	assert.Equal(t, "hack-skills", dl.BinDir)
	assert.Equal(t, "hack-skills/version.txt", dl.BinPath)
	assert.Equal(t, "https://example.com/hack-skills/latest/hack-skills.zip", dl.URL,
		"baseurl prefix must still work alongside new fields")
}
