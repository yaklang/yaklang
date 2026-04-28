package thirdparty_bin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

// hack-skills 是 yaklang 第三方应用注册表中唯一一个使用 archive + 公开密码 +
// install_root=ai-skills 组合的条目。本文件聚集对它的回归测试，覆盖：
//
//  1. bin_cfg.yml 中关键字段不能被误改 (URL / password / install_root /
//     bin_dir / bin_path / archive_type / install_type)
//  2. 端到端: 用本地 httptest 服务模拟 OSS, 跑完整 Install 链路 + AutoSkillLoader 兼容的
//     目录布局校验, 完全离线
//
// 关键词: hack-skills 注册表回归, anti-AV public password, ai-skills install root,
//        version.txt manifest, AutoSkillLoader 兼容布局, 离线 essential-test

const (
	hackSkillsName            = "hack-skills"
	hackSkillsExpectedPwd     = "hack-skills"
	hackSkillsExpectedRoot    = "ai-skills"
	hackSkillsExpectedBinDir  = "hack-skills"
	hackSkillsExpectedBinPath = "hack-skills/version.txt"
	hackSkillsExpectedURLTail = "/hack-skills/latest/hack-skills.zip"
	hackSkillsExpectedAType   = ".zip"
	hackSkillsExpectedIType   = "archive"
)

// findHackSkillsDescriptor 从内嵌 bin_cfg.yml 中读取 hack-skills 注册条目
// 关键词: LoadConfigFromEmbedded, hack-skills 注册条目
func findHackSkillsDescriptor(t *testing.T) *BinaryDescriptor {
	t.Helper()
	cfg, err := LoadConfigFromEmbedded()
	require.NoError(t, err, "embedded bin_cfg.yml must parse")

	for _, b := range cfg.Binaries {
		if b.Name == hackSkillsName {
			return b
		}
	}
	t.Fatalf("hack-skills entry missing in bin_cfg.yml")
	return nil
}

// 验证 bin_cfg.yml 中 hack-skills 条目所有关键字段，避免后续误改导致用户安装行为飘移
// 关键词: hack-skills 字段回归, install_root, public password, bin_path anchor
func TestHackSkills_BinCfgEntry(t *testing.T) {
	desc := findHackSkillsDescriptor(t)

	assert.Equal(t, hackSkillsExpectedIType, desc.InstallType, "install_type")
	assert.Equal(t, hackSkillsExpectedAType, desc.ArchiveType, "archive_type")
	assert.Equal(t, hackSkillsExpectedRoot, desc.InstallRoot,
		"install_root must be %q so files land under ~/yakit-projects/ai-skills/",
		hackSkillsExpectedRoot)

	dl, ok := desc.DownloadInfoMap["*"]
	require.True(t, ok, `download_info_map must have "*" platform key`)

	assert.Equal(t, hackSkillsExpectedPwd, dl.Password,
		"password must be the public anti-AV constant %q", hackSkillsExpectedPwd)
	assert.Equal(t, "*", dl.Pick, `pick must be "*" so all skills are extracted`)
	assert.Equal(t, hackSkillsExpectedBinDir, dl.BinDir, "bin_dir")
	assert.Equal(t, hackSkillsExpectedBinPath, dl.BinPath,
		"bin_path must point to a real file (version.txt) — directories cannot be IsInstalled anchors")

	u, err := url.Parse(dl.URL)
	require.NoError(t, err, "URL must be parseable")
	assert.True(t, u.IsAbs(), "URL must be absolute after baseurl join, got %q", dl.URL)
	assert.Contains(t, dl.URL, hackSkillsExpectedURLTail,
		"URL must end with %q (latest channel)", hackSkillsExpectedURLTail)
}

// 端到端 (离线): 取真实 hack-skills 描述符, 把 URL 替换成本地 httptest 服务,
// 用真实 BaseInstaller 跑完整 Install 链路, 验证:
//   - version.txt manifest 落到 install_dir/version.txt
//   - 所有 skills/<topic>/SKILL.md 都被解出
//   - IsInstalled 通过 manifest 锚点工作
//
// 注意 install_root 临时改为 "" 避免污染真实 ~/yakit-projects/ai-skills/ 目录
// 关键词: hack-skills 端到端离线, httptest OSS 模拟, version.txt manifest 校验
func TestHackSkills_OfflineEndToEnd(t *testing.T) {
	skillTopics := []string{"recon", "api-sec", "sqli", "xss", "ssrf"}
	manifestVersion := "20260428-essential-test"

	zipFiles := map[string]string{
		"version.txt": manifestVersion + "\n",
	}
	for _, topic := range skillTopics {
		zipFiles["skills/"+topic+"/SKILL.md"] = "# " + topic + " skill"
	}
	zipFiles["skills/recon/notes/raw.md"] = "extra recon notes"

	zipData := makeEncryptedZip(t, zipFiles, hackSkillsExpectedPwd)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(zipData)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipData)
	}))
	defer srv.Close()

	tmpRoot := t.TempDir()
	bi := &BaseInstaller{defaultInstallDir: tmpRoot, downloadDir: t.TempDir()}

	// 拷一份真实 hack-skills 描述符, 替换 URL 为本地 httptest, 同时把
	// install_root 临时清空, 避免端到端测试污染真机的 ~/yakit-projects/ai-skills/
	// 关键词: 描述符浅拷贝, install_root 临时空串, 沙盒化测试根目录
	original := findHackSkillsDescriptor(t)
	desc := *original
	desc.InstallRoot = ""
	desc.DownloadInfoMap = map[string]*DownloadInfo{}
	for k, v := range original.DownloadInfoMap {
		copyDL := *v
		copyDL.URL = srv.URL + "/hack-skills.zip"
		desc.DownloadInfoMap[k] = &copyDL
	}

	require.NoError(t, bi.Install(&desc, &InstallOptions{Force: true}),
		"Install must succeed against local OSS replica")

	manifestPath := filepath.Join(tmpRoot, hackSkillsExpectedBinPath)
	got, err := os.ReadFile(manifestPath)
	require.NoError(t, err, "version.txt anchor must exist after install")
	assert.Equal(t, manifestVersion+"\n", string(got),
		"manifest content must round-trip through encrypted zip + extract")

	for _, topic := range skillTopics {
		p := filepath.Join(tmpRoot, hackSkillsExpectedBinDir, "skills", topic, "SKILL.md")
		_, statErr := os.Stat(p)
		assert.NoError(t, statErr,
			"SKILL.md must be reachable under installed dir for AutoSkillLoader: %s", p)
	}

	assert.True(t, bi.IsInstalled(&desc, nil),
		"IsInstalled must be true once version.txt anchor exists")
}

// 在 install_root="ai-skills" 时, 计算的目标路径必须落到 consts.GetDefaultAISkillsDir()
// 这样 AutoSkillLoader 才能通过默认 ai-skills 扫描发现解压后的 SKILL.md
// 关键词: ai-skills 安装根, AutoSkillLoader 默认扫描, hack-skills 路径计算
func TestHackSkills_TargetPath_AnchoredOnAISkills(t *testing.T) {
	desc := findHackSkillsDescriptor(t)

	bi := &BaseInstaller{defaultInstallDir: t.TempDir(), downloadDir: t.TempDir()}
	target := bi.GetTargetPath(desc, nil)
	installDir := bi.GetInstallDir(desc, nil)

	aiSkills := consts.GetDefaultAISkillsDir()
	assert.Equal(t,
		filepath.Join(aiSkills, hackSkillsExpectedBinDir, "version.txt"),
		target,
		"GetTargetPath must point to ai-skills/hack-skills/version.txt")
	assert.Equal(t,
		filepath.Join(aiSkills, hackSkillsExpectedBinDir),
		installDir,
		"GetInstallDir must point to ai-skills/hack-skills")
}
