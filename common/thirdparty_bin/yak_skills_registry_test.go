package thirdparty_bin

import (
	"archive/zip"
	"bytes"
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

// yak-skills 与 hack-skills 形态一致(archive + install_root=ai-skills + version.txt 锚点),
// 区别在于 yak-skills 是普通编程技能, 使用 *明文 zip*(无 password)。本文件聚集对它的
// 回归测试, 覆盖:
//
//  1. bin_cfg.yml 中关键字段不能被误改 (URL / install_root / bin_dir / bin_path /
//     archive_type / install_type), 且 password 必须为空(明文 zip)
//  2. 端到端: 用本地 httptest 服务模拟 OSS, 跑完整 Install 链路 + AutoSkillLoader 兼容的
//     目录布局校验, 完全离线
//
// 关键词: yak-skills 注册表回归, plain zip no password, ai-skills install root,
//        version.txt manifest, AutoSkillLoader 兼容布局, 离线 essential-test

const (
	yakSkillsName            = "yak-skills"
	yakSkillsExpectedRoot    = "ai-skills"
	yakSkillsExpectedBinDir  = "yak-skills"
	yakSkillsExpectedBinPath = "yak-skills/version.txt"
	yakSkillsExpectedURLTail = "/yak-skills/latest/yak-skills.zip"
	yakSkillsExpectedAType   = ".zip"
	yakSkillsExpectedIType   = "archive"
)

// makePlainZip 创建普通(未加密)zip, 用于 yak-skills 离线端到端测试
// 关键词: plain zip, archive/zip, yak-skills 离线测试
func makePlainZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// findYakSkillsDescriptor 从内嵌 bin_cfg.yml 中读取 yak-skills 注册条目
// 关键词: LoadConfigFromEmbedded, yak-skills 注册条目
func findYakSkillsDescriptor(t *testing.T) *BinaryDescriptor {
	t.Helper()
	cfg, err := LoadConfigFromEmbedded()
	require.NoError(t, err, "embedded bin_cfg.yml must parse")

	for _, b := range cfg.Binaries {
		if b.Name == yakSkillsName {
			return b
		}
	}
	t.Fatalf("yak-skills entry missing in bin_cfg.yml")
	return nil
}

// 验证 bin_cfg.yml 中 yak-skills 条目所有关键字段, 避免后续误改导致用户安装行为飘移
// 关键词: yak-skills 字段回归, install_root, plain zip(no password), bin_path anchor
func TestYakSkills_BinCfgEntry(t *testing.T) {
	desc := findYakSkillsDescriptor(t)

	assert.Equal(t, yakSkillsExpectedIType, desc.InstallType, "install_type")
	assert.Equal(t, yakSkillsExpectedAType, desc.ArchiveType, "archive_type")
	assert.Equal(t, yakSkillsExpectedRoot, desc.InstallRoot,
		"install_root must be %q so files land under ~/yakit-projects/ai-skills/",
		yakSkillsExpectedRoot)

	dl, ok := desc.DownloadInfoMap["*"]
	require.True(t, ok, `download_info_map must have "*" platform key`)

	assert.Empty(t, dl.Password,
		"yak-skills must be a plain zip with NO password (unlike hack-skills)")
	assert.Equal(t, "*", dl.Pick, `pick must be "*" so all skills are extracted`)
	assert.Equal(t, yakSkillsExpectedBinDir, dl.BinDir, "bin_dir")
	assert.Equal(t, yakSkillsExpectedBinPath, dl.BinPath,
		"bin_path must point to a real file (version.txt) — directories cannot be IsInstalled anchors")

	u, err := url.Parse(dl.URL)
	require.NoError(t, err, "URL must be parseable")
	assert.True(t, u.IsAbs(), "URL must be absolute after baseurl join, got %q", dl.URL)
	assert.Contains(t, dl.URL, yakSkillsExpectedURLTail,
		"URL must end with %q (latest channel)", yakSkillsExpectedURLTail)
}

// 端到端 (离线): 取真实 yak-skills 描述符, 把 URL 替换成本地 httptest 服务,
// 用真实 BaseInstaller 跑完整 Install 链路, 验证:
//   - version.txt manifest 落到 install_dir/version.txt
//   - 所有 skills/<topic>/SKILL.md 都被解出
//   - IsInstalled 通过 manifest 锚点工作
//
// 注意 install_root 临时改为 "" 避免污染真实 ~/yakit-projects/ai-skills/ 目录
// 关键词: yak-skills 端到端离线, httptest OSS 模拟, version.txt manifest 校验
func TestYakSkills_OfflineEndToEnd(t *testing.T) {
	skillTopics := []string{"yak", "mitm-hotpatch", "webfuzzer-hotpatch", "global-hotpatch", "yaklang-syntax"}
	manifestVersion := "20260630-essential-test"

	zipFiles := map[string]string{
		"version.txt": manifestVersion + "\n",
	}
	for _, topic := range skillTopics {
		zipFiles["skills/"+topic+"/SKILL.md"] = "# " + topic + " skill"
	}
	zipFiles["skills/mitm-hotpatch/examples/hijack-request.yak"] = "// example"

	zipData := makePlainZip(t, zipFiles)

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

	// 拷一份真实 yak-skills 描述符, 替换 URL 为本地 httptest, 同时把 install_root
	// 临时清空, 避免端到端测试污染真机的 ~/yakit-projects/ai-skills/
	// 关键词: 描述符浅拷贝, install_root 临时空串, 沙盒化测试根目录
	original := findYakSkillsDescriptor(t)
	desc := *original
	desc.InstallRoot = ""
	desc.DownloadInfoMap = map[string]*DownloadInfo{}
	for k, v := range original.DownloadInfoMap {
		copyDL := *v
		copyDL.URL = srv.URL + "/yak-skills.zip"
		desc.DownloadInfoMap[k] = &copyDL
	}

	require.NoError(t, bi.Install(&desc, &InstallOptions{Force: true}),
		"Install must succeed against local OSS replica")

	manifestPath := filepath.Join(tmpRoot, yakSkillsExpectedBinPath)
	got, err := os.ReadFile(manifestPath)
	require.NoError(t, err, "version.txt anchor must exist after install")
	assert.Equal(t, manifestVersion+"\n", string(got),
		"manifest content must round-trip through plain zip + extract")

	for _, topic := range skillTopics {
		p := filepath.Join(tmpRoot, yakSkillsExpectedBinDir, "skills", topic, "SKILL.md")
		_, statErr := os.Stat(p)
		assert.NoError(t, statErr,
			"SKILL.md must be reachable under installed dir for AutoSkillLoader: %s", p)
	}

	assert.True(t, bi.IsInstalled(&desc, nil),
		"IsInstalled must be true once version.txt anchor exists")
}

// 在 install_root="ai-skills" 时, 计算的目标路径必须落到 consts.GetDefaultAISkillsDir()
// 这样 AutoSkillLoader 才能通过默认 ai-skills 扫描发现解压后的 SKILL.md
// 关键词: ai-skills 安装根, AutoSkillLoader 默认扫描, yak-skills 路径计算
func TestYakSkills_TargetPath_AnchoredOnAISkills(t *testing.T) {
	desc := findYakSkillsDescriptor(t)

	bi := &BaseInstaller{defaultInstallDir: t.TempDir(), downloadDir: t.TempDir()}
	target := bi.GetTargetPath(desc, nil)
	installDir := bi.GetInstallDir(desc, nil)

	aiSkills := consts.GetDefaultAISkillsDir()
	assert.Equal(t,
		filepath.Join(aiSkills, yakSkillsExpectedBinDir, "version.txt"),
		target,
		"GetTargetPath must point to ai-skills/yak-skills/version.txt")
	assert.Equal(t,
		filepath.Join(aiSkills, yakSkillsExpectedBinDir),
		installDir,
		"GetInstallDir must point to ai-skills/yak-skills")
}
