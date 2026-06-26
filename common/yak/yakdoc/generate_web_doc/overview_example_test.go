package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc/webdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// extractFencedYak 从一段 markdown 里抽取所有三反引号 ```yak 围栏块的纯代码。
// overview 用普通 ```yak 围栏(供 Docusaurus 渲染)，与函数示例的 14 反引号围栏不同，故单独抽取。
// 关键词: overview 代码抽取, ```yak 围栏
func extractFencedYak(md string) []string {
	lines := strings.Split(md, "\n")
	var blocks []string
	var cur []string
	in := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if !in {
			if t == "```yak" {
				in = true
				cur = nil
			}
			continue
		}
		if t == "```" {
			blocks = append(blocks, strings.Join(cur, "\n"))
			in = false
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return blocks
}

// TestOverviewYakExamples 校验 overviews/<lib>.md 里的 ```yak 代码块：全部做 antlr 语法检查；
// 对属于安全执行白名单(safeExecLibs)的库，再真实执行(SafeEval)，任何失败即红。这样 overview 里的
// "快速上手"示例也享有与函数示例同等的"语法正确 + 可本地运行"质量保证。
// 关键词: overview 示例校验, 语法+执行, 核心库 quickstart
func TestOverviewYakExamples(t *testing.T) {
	debug.SetGCPercent(-1)
	setupLocalExecEnv(t)

	dir := "overviews"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read overviews dir failed: %v", err)
	}

	syntaxChecker := func(code string) error {
		_, err := antlr4yak.New().FormattedAndSyntaxChecking(code)
		return err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	totalBlocks, syntaxFailed, execChecked, execFailed := 0, 0, 0, 0
	for _, fname := range names {
		lib := strings.TrimSuffix(fname, ".md")
		raw, err := os.ReadFile(filepath.Join(dir, fname))
		if err != nil {
			t.Errorf("read overview %s failed: %v", fname, err)
			continue
		}
		blocks := extractFencedYak(string(raw))
		for i, code := range blocks {
			totalBlocks++
			if err := syntaxChecker(code); err != nil {
				syntaxFailed++
				t.Errorf("overview %q block #%d syntax error: %v\n--- code ---\n%s\n--- end ---", lib, i+1, err, code)
				continue
			}
			// 只对安全执行白名单库的 overview 示例做真实执行(其余库可能涉及网络/外部依赖)。
			if !safeExecLibs[lib] || strings.Contains(code, noLocalVerifyMarker) {
				continue
			}
			execChecked++
			engine := yaklang.New()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			err := engine.SafeEval(ctx, code)
			cancel()
			if err != nil {
				execFailed++
				t.Errorf("overview %q block #%d execution error: %v\n--- code ---\n%s\n--- end ---", lib, i+1, err, code)
			}
		}
	}
	t.Logf("OVERVIEW-EXAMPLES: blocks=%d syntaxFailed=%d execChecked=%d execFailed=%d",
		totalBlocks, syntaxFailed, execChecked, execFailed)
}

// TestOverviewInjectedInvariants 校验"把 overview 注入库文档后"最终产物仍满足 Markdown 结构不变量
// (围栏配对、标题层级、锚点完整、表格列一致、无裸 < / 裸 URL)。常规渲染测试用空 overview，这里补上
// 带 overview 的最终形态，确保 overview 里的代码块/正文不会破坏文档站构建。
// 关键词: overview 注入不变量, 最终产物校验
func TestOverviewInjectedInvariants(t *testing.T) {
	debug.SetGCPercent(-1)
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())

	dir := "overviews"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read overviews dir failed: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		lib := strings.TrimSuffix(e.Name(), ".md")
		sl, ok := helper.Libs[lib]
		if !ok {
			continue // overview 对应的库未导出(如 ai 走 MDX 分支)，跳过
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Errorf("read overview %s failed: %v", e.Name(), err)
			continue
		}
		md := webdoc.RenderLibMarkdown(sl, string(raw), nil)
		if err := webdoc.CheckMarkdownInvariants(md); err != nil {
			t.Errorf("lib %q with overview violates invariants:\n%v", lib, err)
		}
	}
}
