package loop_yaklangcode

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
)

// skillRescanner 是 *aicommon.Config 的子集接口, 用于在安装 yak-skills 后刷新技能加载器,
// 让新解压出来的 SKILL.md 立即可被发现。通过接口断言获取, 避免硬依赖具体 config 实现。
// 关键词: skillRescanner, LoadBuiltinSkillsFromDir, AutoSkillLoader 刷新
type skillRescanner interface {
	LoadBuiltinSkillsFromDir(dirPath string) error
}

// ensureDependencies 在 init 阶段检查关键依赖并按需自动安装:
//   - yaklang-aikb (grep 搜索器)
//   - yaklang-aikb-rag (语义搜索器)
//   - yak-skills (技能包, 安装后刷新技能加载器)
//
// 策略(A 选项): 阻塞下载 + 进度 EmitStatus + 失败降级(log.Warnf 后继续, 不中断 loop)。
// 仅当未提供自定义路径且未禁用自动安装(aikb_auto_install_disabled=false)时触发, 已安装则
// thirdparty_bin.Install 内部跳过下载, 成本极低。
// 关键词: 关键依赖自动安装, 阻塞带进度, 失败降级, 回填 searcherHolder, 刷新技能
func ensureDependencies(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	holder *searcherHolder,
	cfg *aikbInstallConfig,
) {
	if cfg == nil || !cfg.autoInstall {
		return
	}
	// 测试环境下跳过自动下载: 避免 mock 单测触发真实网络 I/O; 此时走现有降级路径(仅 yakdoc)。
	// 关键词: InTestcase, 跳过自动安装, 避免单测网络, 降级
	if utils.InTestcase() {
		log.Info("skip aikb/yak-skills auto-install in testcase mode")
		return
	}
	ctx := task.GetContext()

	// 1. grep 搜索器: yaklang-aikb
	if holder.getGrep() == nil && cfg.aikbPath == "" {
		if installThirdpartyBinWithProgress(ctx, loop, "yaklang-aikb", "Yaklang 代码知识库 / Yaklang code knowledge base") {
			if s := createDocumentSearcher(""); s != nil {
				holder.setGrep(s)
				loop.Set("aikb_available", "true")
				reactloops.EmitStatus(loop, "Yaklang 代码知识库就绪 / Yaklang code KB ready")
			} else {
				log.Warnf("yaklang-aikb installed but grep searcher rebuild failed")
			}
		}
	}

	// 2. 语义搜索器: yaklang-aikb-rag
	if holder.getRAG() == nil && cfg.aikbRagPath == "" {
		if installThirdpartyBinWithProgress(ctx, loop, "yaklang-aikb-rag", "Yaklang 语义知识库 / Yaklang semantic knowledge base") {
			var ragSys *rag.RAGSystem
			var err error
			if cfg.enableTestRAG {
				ragSys, err = createTestDocumentSearcherByRag(cfg.ragCollection, "")
			} else {
				ragSys, err = createDocumentSearcherByRag(cfg.db, cfg.ragCollection, "")
			}
			if err != nil {
				log.Warnf("rebuild rag searcher after install failed: %v", err)
			} else if ragSys != nil {
				holder.setRAG(ragSys)
				loop.Set("aikb_available", "true")
				reactloops.EmitStatus(loop, "Yaklang 语义知识库就绪 / Yaklang semantic KB ready")
			}
		}
	}

	// 3. yak-skills: 安装并刷新技能加载器(让 SKILL.md 可被发现)
	ensureYakSkillsInstalled(r, loop, ctx)
}

// installThirdpartyBinWithProgress 阻塞安装一个第三方依赖, 并通过 EmitStatus 推送下载进度。
// 已安装时 Install 会快速返回。安装失败仅记录 warning 并返回 false, 由调用方降级处理。
// 关键词: 阻塞安装带进度, EmitStatus 进度节流, 失败返回 false
func installThirdpartyBinWithProgress(
	ctx context.Context,
	loop *reactloops.ReActLoop,
	name string,
	displayName string,
) bool {
	// 已安装则直接成功(交给调用方重建搜索器), 避免重复下载
	if _, err := thirdparty_bin.GetBinaryPath(name); err == nil {
		return true
	}

	reactloops.EmitStatus(loop, fmt.Sprintf("准备下载 %s ... / Preparing to download %s ...", displayName, name))

	lastPct := -1.0
	opts := &thirdparty_bin.InstallOptions{
		Context: ctx,
		Progress: func(progress float64, downloaded, total int64, message string) {
			// 节流: 每推进 ~5% 才 emit 一次, 避免刷屏
			if progress-lastPct < 0.05 && progress < 1.0 {
				return
			}
			lastPct = progress
			reactloops.EmitStatus(loop, fmt.Sprintf(
				"下载 %s %.0f%% / Downloading %s %.0f%%",
				displayName, progress*100, name, progress*100,
			))
		},
	}

	if err := thirdparty_bin.Install(name, opts); err != nil {
		log.Warnf("auto-install %s failed: %v (degraded mode)", name, err)
		reactloops.EmitStatus(loop, fmt.Sprintf(
			"%s 下载失败, 降级运行 / %s download failed, running in degraded mode", displayName, name,
		))
		return false
	}

	log.Infof("auto-install %s succeeded", name)
	return true
}

// ensureYakSkillsInstalled 安装 yak-skills 技能包, 成功后刷新技能加载器使其可被发现。
// 失败仅降级, 不影响主流程。
// 关键词: yak-skills 自动安装, 刷新技能加载器, 失败降级
func ensureYakSkillsInstalled(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop, ctx context.Context) {
	alreadyInstalled := false
	if _, err := thirdparty_bin.GetBinaryPath("yak-skills"); err == nil {
		alreadyInstalled = true
	}

	if !alreadyInstalled {
		if !installThirdpartyBinWithProgress(ctx, loop, "yak-skills", "Yak 技能包 / Yak skills pack") {
			return
		}
	}

	// 取安装目录(version.txt 所在目录), 刷新技能加载器
	binPath, err := thirdparty_bin.GetBinaryPath("yak-skills")
	if err != nil {
		log.Warnf("yak-skills installed but path lookup failed: %v", err)
		return
	}
	skillsDir := filepath.Dir(binPath)

	rescanner, ok := r.GetConfig().(skillRescanner)
	if !ok {
		log.Warnf("config does not support skill rescan; yak-skills installed at %s but not refreshed", skillsDir)
		return
	}
	if err := rescanner.LoadBuiltinSkillsFromDir(skillsDir); err != nil {
		log.Warnf("refresh skills from %s failed: %v", skillsDir, err)
		return
	}
	if !alreadyInstalled {
		reactloops.EmitStatus(loop, "Yak 技能包就绪 / Yak skills pack ready")
	}
	log.Infof("yak-skills refreshed into skill loader from: %s", skillsDir)
}
