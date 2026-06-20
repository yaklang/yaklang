package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc/webdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// noLocalVerifyMarker 是示例"无法本地验证"的标注短语：既是给人看的说明，也是给执行校验用的机器标记。
// 凡是涉及真实网络出站/外部目标/凭据/特权且无法本地 mock 的示例，请在示例代码里用注释写明该短语，
// 例如: // 无法本地验证: 需要真实可达的外部 HTTP 目标。执行校验会跳过这类示例并记录清单，便于后续单独处理。
// 关键词: 无法本地验证, 示例执行标注, mock
const noLocalVerifyMarker = "无法本地验证"

// safeExecLibs 是"可安全、确定性、不出网执行"的纯计算/本地解析库白名单：对这些库的所有示例做真实执行
// (SafeEval)，任何失败都视为示例缺陷(强约束，测试判红)。它们无网络出站、无命令执行、无文件写删、无特权。
// 关键词: 安全执行白名单, 纯计算库, 强约束
var safeExecLibs = map[string]bool{
	"codec": true, "str": true, "math": true, "re": true, "re2": true,
	"json": true, "yaml": true, "xml": true, "regen": true, "container": true,
	"orderedmap": true, "x": true, "bin": true, "jsonschema": true,
	"dictutil": true, "gzip": true, "jwt": true, "mfa": true, "twofa": true, "mimetype": true,
	// 批次2: 纯文本/解析/序列化(无网络出站、无命令执行、无外部依赖)
	// 注: yso 暂不纳入 —— 其多条示例引用了已改名/不存在的导出函数(文档准确性问题)，需单独的 API 校准跟进。
	"diff": true, "xpath": true, "java": true, "jsonstream": true,
	// 批次3: 纯本地(读环境变量/日志/内存编辑/HTML解析/沙箱求值，均不出网)
	// 注: zip 暂不纳入 —— 其解压/检索类示例需要磁盘上真实存在的 zip 文件(可后续用本地 fixture mock 纳入)。
	"env": true, "log": true, "memeditor": true, "xhtml": true, "sandbox": true,
	// 批次4: 时间/时区/本地 JS 引擎(均不出网、不需外部资源)
	"time": true, "timezone": true, "js": true,
}

func setupLocalExecEnv(t *testing.T) {
	// 使用临时数据库，避免读写用户真实的 Yakit profile/project，杜绝副作用。
	dir := t.TempDir()
	if err := consts.InitializeYakitDatabase(
		filepath.Join(dir, "project.db"),
		filepath.Join(dir, "profile.db"),
		filepath.Join(dir, "ssa.db"),
	); err != nil {
		t.Logf("init temp database failed (continue best-effort): %v", err)
	}
	// 起一个本地 mock HTTP 服务，地址通过环境变量暴露，便于将来示例以 mock 方式自给自足地联调。
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 4\r\n\r\nmock"))
	_ = os.Setenv("YAK_DOC_MOCK_HTTP", utils.HostPort(host, port))
}

// TestSafeLibsExampleExecution 对纯计算库白名单的示例做真实执行，强约束(失败即红)。这是在"语法正确"
// 之上的"运行正确"下限：纯计算示例必须能本地跑通且无错。关键词: 示例执行验证, 纯计算库强约束
func TestSafeLibsExampleExecution(t *testing.T) {
	debug.SetGCPercent(-1)
	setupLocalExecEnv(t)
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())

	names := make([]string, 0, len(safeExecLibs))
	for name := range helper.Libs {
		if safeExecLibs[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	total, annotated, failed := 0, 0, 0
	var report strings.Builder
	for _, name := range names {
		lib := helper.Libs[name]
		examples := webdoc.ExtractYakExamples(webdoc.RenderLibMarkdown(lib, "", nil))
		for i, code := range examples {
			total++
			if strings.Contains(code, noLocalVerifyMarker) {
				annotated++
				continue
			}
			engine := yaklang.New()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			err := engine.SafeEval(ctx, code)
			cancel()
			if err == nil {
				continue
			}
			failed++
			report.WriteString(fmt.Sprintf("=== %s example #%d ===\n%v\n--- code ---\n%s\n\n", name, i+1, err, code))
		}
	}
	_ = os.WriteFile("/tmp/exec_fail.txt", []byte(report.String()), 0o644)
	t.Logf("SAFE-EXEC: libs=%d examples=%d annotated=%d failed=%d (detail: /tmp/exec_fail.txt)", len(names), total, annotated, failed)
	if failed > 0 {
		t.Errorf("safe-lib example execution had %d failures (see /tmp/exec_fail.txt)", failed)
	}
}

// TestAllLibsExampleVerifiabilityReport 对【全部】库的示例做"本地可验证性"的【静态】分类(不执行、不出网、
// 不产生任何副作用)，只统计与产出清单，用于推进文档示例的本地可验证覆盖率：
//   - executed : 属于安全执行白名单(safeExecLibs)的示例，已在 TestSafeLibsExampleExecution 真实执行校验。
//   - annotated: 含 "无法本地验证" 标注的示例(涉及真实网络/外部目标/凭据/特权，作者已显式说明)。
//   - unverified: 既不在白名单、也未标注的示例 —— 需要后续"做本地 mock 改写"或"加无法本地验证标注"。
//
// 之所以不直接执行非白名单库的示例：这些示例会真实出网(已观测到 bot webhook 发送)、可能执行命令/写删
// 文件、甚至调用 os.Exit/log.Fatal 直接崩溃进程，违背"尽量不出网、无副作用"的原则。因此对它们只做静态
// 分类，把清单交给人工逐个 mock 或标注。关键词: 示例本地可验证性, 静态分类, 无法本地验证清单, 不出网
func TestAllLibsExampleVerifiabilityReport(t *testing.T) {
	debug.SetGCPercent(-1)
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())

	names := make([]string, 0, len(helper.Libs))
	for name := range helper.Libs {
		names = append(names, name)
	}
	sort.Strings(names)

	total, executed, annotated, unverified := 0, 0, 0, 0
	var unverList strings.Builder
	for _, name := range names {
		lib := helper.Libs[name]
		examples := webdoc.ExtractYakExamples(webdoc.RenderLibMarkdown(lib, "", nil))
		for i, code := range examples {
			total++
			switch {
			case safeExecLibs[name]:
				executed++
			case strings.Contains(code, noLocalVerifyMarker):
				annotated++
			default:
				unverified++
				unverList.WriteString(fmt.Sprintf("%s #%d\n", name, i+1))
			}
		}
	}
	_ = os.WriteFile("/tmp/doc_unverified.txt", []byte(unverList.String()), 0o644)
	covered := executed + annotated
	t.Logf("VERIFIABILITY: examples=%d executed=%d annotated=%d (covered=%d, %.1f%%) unverified=%d",
		total, executed, annotated, covered, 100*float64(covered)/float64(total), unverified)
	t.Logf("unverified list (need local mock rewrite or 无法本地验证 annotation): /tmp/doc_unverified.txt")
}
