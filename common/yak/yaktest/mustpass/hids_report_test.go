package mustpass

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
)

func TestGenerateHIDSMustpassReport(t *testing.T) {
	if os.Getenv("YAK_MUSTPASS_REPORT") == "" {
		t.Skip("set YAK_MUSTPASS_REPORT=1 to generate the HIDS mustpass report")
	}

	if testDir == "" {
		t.Fatalf("HIDS test directory not available (testDir is empty)")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to locate caller file")
	}
	baseDir := filepath.Dir(thisFile)
	filesHidsDir := filepath.Join(baseDir, "files-hids")
	docPath := filepath.Join(filesHidsDir, "README.md")
	outPath := filepath.Join(filesHidsDir, "REPORT.md")

	docRaw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read mapping doc failed: %v", err)
	}

	docIndex := parseFilesHidsDoc(string(docRaw))

	var caseNames []string
	for name := range filesHids {
		caseNames = append(caseNames, name)
	}
	sort.Strings(caseNames)

	const maxOutputBytes = 10 * 1024
	results := make([]filesHidsRunResult, 0, len(caseNames))
	var failed []string

	for _, name := range caseNames {
		code := filesHids[name]
		rr := filesHidsRunResult{Name: name}

		rr.Meta = docIndex[name]
		rr.TestedFns = extractTestedFunctions(code)
		rr.Needs = detectNeededParams(code)

		vars := map[string]any{
			"TEST_DIR":      testDir,
			"VULINBOX":      vulinboxAddr,
			"VULINBOX_HOST": utils.ExtractHostPort(vulinboxAddr),
		}

		start := time.Now()
		out, retType, runErr := runYakWithCapturedOutput(func() (any, error) {
			return yak.Execute(code, vars)
		})
		rr.Duration = time.Since(start)
		rr.ReturnType = retType

		if runErr != nil {
			rr.OK = false
			rr.Err = runErr.Error()
			failed = append(failed, name)
		} else {
			rr.OK = true
		}

		if len(out) > maxOutputBytes {
			rr.Output = out[:maxOutputBytes]
			rr.Truncated = true
		} else {
			rr.Output = out
		}

		results = append(results, rr)
		time.Sleep(50 * time.Millisecond)
	}

	report := buildFilesHidsReport(results, "common/yak/yaktest/mustpass/files-hids/README.md")
	if err := os.WriteFile(outPath, []byte(report), 0644); err != nil {
		t.Fatalf("write report failed: %v", err)
	}

	if len(failed) > 0 {
		t.Fatalf("%d HIDS script(s) failed: %s (report written to %s)", len(failed), strings.Join(failed, ", "), outPath)
	}
}

type filesHidsRunResult struct {
	Name       string
	OK         bool
	Duration   time.Duration
	ReturnType string
	Err        string
	Output     string
	Truncated  bool
	Meta       []filesHidsDocEntry
	TestedFns  string
	Needs      []string
}

type filesHidsDocEntry struct {
	Module     string
	Item       string
	Desc       string
	GoFile     string
	ScriptPath string
	Line       int
}

var (
	reYakPath = regexp.MustCompile(`(?i)(common[\\/]yak[\\/]yaktest[\\/]mustpass[\\/]files-hids[\\/][^\s"'` + "`" + `<>]+\.yak)`)
	reGoPath  = regexp.MustCompile(`(?i)(common[\\/][^\s"'` + "`" + `<>]+\.go)`)
)

func parseFilesHidsDoc(md string) map[string][]filesHidsDocEntry {
	out := make(map[string][]filesHidsDocEntry)

	lines := strings.Split(md, "\n")
	for i, rawLine := range lines {
		lineNo := i + 1
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}

		row := strings.Trim(line, "|")
		cols := strings.Split(row, "|")
		for j := range cols {
			cols[j] = strings.TrimSpace(cols[j])
		}
		if len(cols) < 5 {
			continue
		}
		if strings.EqualFold(cols[0], "模块") {
			continue
		}
		if isMarkdownTableSeparator(cols) {
			continue
		}

		module := strings.TrimSpace(cols[0])
		item := strings.TrimSpace(cols[1])
		desc := strings.TrimSpace(cols[2])
		implCell := cols[3]
		scriptCell := cols[4]

		goFile := ""
		if m := reGoPath.FindStringSubmatch(implCell); len(m) == 2 {
			goFile = normalizeDocPath(m[1])
		} else {
			implNorm := normalizeDocPath(implCell)
			implNorm = strings.ReplaceAll(implNorm, "<br>", " ")
			fields := strings.FieldsFunc(implNorm, func(r rune) bool {
				return r == ' ' || r == '\t' || r == '`'
			})
			for _, f := range fields {
				if strings.HasPrefix(f, "common/") {
					goFile = f
					break
				}
			}
		}

		yakPaths := reYakPath.FindAllStringSubmatch(scriptCell, -1)
		for _, m := range yakPaths {
			if len(m) != 2 {
				continue
			}
			p := normalizeDocPath(m[1])
			base := filepath.Base(p)
			out[base] = append(out[base], filesHidsDocEntry{
				Module:     module,
				Item:       item,
				Desc:       desc,
				GoFile:     goFile,
				ScriptPath: p,
				Line:       lineNo,
			})
		}
	}

	return out
}

func isMarkdownTableSeparator(cols []string) bool {
	for _, c := range cols {
		s := strings.TrimSpace(c)
		if s == "" {
			continue
		}
		hasDash := false
		for _, r := range s {
			if r == '-' {
				hasDash = true
				continue
			}
			if r == ':' {
				continue
			}
			return false
		}
		if !hasDash {
			return false
		}
	}
	return true
}

func normalizeDocPath(p string) string {
	p = strings.Trim(p, "\"'`")
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "./") {
		p = strings.TrimPrefix(p, "./")
	}
	return p
}

func extractTestedFunctions(code string) string {
	lines := strings.Split(code, "\n")
	for _, ln := range lines {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, "//") && strings.Contains(s, "测试函数") {
			idx := strings.Index(s, "：")
			if idx < 0 {
				idx = strings.Index(s, ":")
			}
			if idx >= 0 && idx+len("：") < len(s) {
				return strings.TrimSpace(s[idx+len("："):])
			}
			return strings.TrimSpace(strings.TrimPrefix(s, "//"))
		}
		if !strings.HasPrefix(s, "//") && s != "" {
			break
		}
	}
	return ""
}

func detectNeededParams(code string) []string {
	needed := make(map[string]struct{})
	re := regexp.MustCompile(`getParam\(\s*\"([^\"]+)\"\s*\)`)
	for _, m := range re.FindAllStringSubmatch(code, -1) {
		if len(m) == 2 {
			needed[m[1]] = struct{}{}
		}
	}
	for _, k := range []string{"TEST_DIR", "VULINBOX", "VULINBOX_HOST"} {
		if strings.Contains(code, k) {
			needed[k] = struct{}{}
		}
	}
	var keys []string
	for k := range needed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func runYakWithCapturedOutput(fn func() (any, error)) (out string, retType string, runErr error) {
	oldOut := os.Stdout
	oldErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", "", fmt.Errorf("pipe stdout failed: %w", err)
	}

	os.Stdout = w
	os.Stderr = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	defer func() {
		_ = w.Close()
		os.Stdout = oldOut
		os.Stderr = oldErr
		_ = r.Close()
		<-done

		out = buf.String()
		if rec := recover(); rec != nil {
			runErr = fmt.Errorf("panic: %v", rec)
		}
	}()

	ret, err := fn()
	if ret != nil {
		retType = fmt.Sprintf("%T", ret)
	}
	if err != nil {
		runErr = err
	}
	return
}

func buildFilesHidsReport(results []filesHidsRunResult, mappingDoc string) string {
	var b strings.Builder
	now := time.Now().Format("2006-01-02 15:04:05")

	b.WriteString("# HIDS MustPass 脚本报告 (files-hids)\n\n")
	b.WriteString(fmt.Sprintf("生成时间: `%s`\n\n", now))
	b.WriteString(fmt.Sprintf("运行环境: `%s/%s` Go `%s`\n\n", runtime.GOOS, runtime.GOARCH, runtime.Version()))
	if testDir != "" {
		b.WriteString(fmt.Sprintf("测试数据目录(TEST_DIR): `%s`\n\n", testDir))
	}
	b.WriteString(fmt.Sprintf("本报告基于 `%s` 的功能清单，并通过 Go mustpass harness 顺序执行每个 `.yak` 脚本来采集真实输出。\n\n", mappingDoc))

	b.WriteString("## 如何运行\n\n")
	b.WriteString("### 1) 运行全部 HIDS mustpass 用例\n\n")
	b.WriteString("```bash\n")
	b.WriteString("go test ./common/yak/yaktest/mustpass -run TestMustPassHIDS -count=1 -v\n")
	b.WriteString("```\n\n")

	b.WriteString("### 2) 仅运行单个脚本(子测试)\n\n")
	b.WriteString("Go 的 `-run` 是正则匹配，注意 `.` 需要转义：\n\n")
	b.WriteString("```bash\n")
	b.WriteString("go test ./common/yak/yaktest/mustpass -run 'TestMustPassHIDS/elf_header\\.yak' -count=1 -v\n")
	b.WriteString("```\n\n")

	b.WriteString("### 3) 生成本报告(采集输出)\n\n")
	b.WriteString("```bash\n")
	b.WriteString("YAK_MUSTPASS_REPORT=1 go test ./common/yak/yaktest/mustpass -run TestGenerateHIDSMustpassReport -count=1 -timeout 20m\n")
	b.WriteString("```\n\n")
	b.WriteString("报告输出: `common/yak/yaktest/mustpass/files-hids/REPORT.md`\n\n")

	b.WriteString("### 4) 用 yak CLI 直接执行脚本\n\n")
	b.WriteString("yak CLI 的默认行为是: 参数跟一个文件路径时直接执行该 `.yak` 文件(参见 `common/yak/cmd/yak.go`).\n\n")
	b.WriteString("```bash\n")
	b.WriteString("go build -o yak common/yak/cmd/yak.go\n")
	b.WriteString("./yak common/yak/yaktest/mustpass/files-hids/connection_list.yak\n")
	b.WriteString("```\n\n")
	b.WriteString("注意: mustpass harness 会注入 `TEST_DIR` 等参数；CLI 直接跑脚本时如果脚本依赖 `getParam(\"TEST_DIR\")`，通常会走脚本内置 fallback 路径(例如从 `/bin/ls` 取 ELF)。\n\n")

	b.WriteString("## 功能清单\n\n")
	b.WriteString(fmt.Sprintf("详见: `%s`\n\n", mappingDoc))

	b.WriteString("## 执行汇总\n\n")
	b.WriteString("| Script | Status | Duration | Return | Notes |\n")
	b.WriteString("|---|---:|---:|---|---|\n")
	for _, r := range results {
		status := "PASS"
		if !r.OK {
			status = "FAIL"
		}
		notes := ""
		if r.Truncated {
			notes = "output truncated"
		}
		dur := r.Duration.String()
		if dur == "0s" {
			dur = fmt.Sprintf("%dms", r.Duration.Milliseconds())
		}
		b.WriteString(fmt.Sprintf("| `%s` | %s | `%s` | `%s` | %s |\n", r.Name, status, dur, r.ReturnType, notes))
	}
	b.WriteString("\n")

	b.WriteString("## 逐脚本结果\n\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("### %s\n\n", r.Name))
		if len(r.Meta) > 0 {
			for _, m := range r.Meta {
				b.WriteString("- 来自功能清单:\n")
				b.WriteString(fmt.Sprintf("  - 模块: %s\n", safeText(m.Module)))
				b.WriteString(fmt.Sprintf("  - 条目: %s\n", safeText(m.Item)))
				b.WriteString(fmt.Sprintf("  - 描述: %s\n", safeText(m.Desc)))
				if m.GoFile != "" {
					b.WriteString(fmt.Sprintf("  - 关联实现: `%s`\n", m.GoFile))
				}
				if m.ScriptPath != "" {
					b.WriteString(fmt.Sprintf("  - 脚本路径: `%s`\n", m.ScriptPath))
				}
				b.WriteString(fmt.Sprintf("  - 清单行号: %d\n", m.Line))
			}
		} else {
			b.WriteString("- 来自功能清单: (未找到对应条目)\n")
		}

		if r.TestedFns != "" {
			b.WriteString(fmt.Sprintf("- 测试函数: %s\n", safeText(r.TestedFns)))
		}
		if len(r.Needs) > 0 {
			b.WriteString(fmt.Sprintf("- 可能依赖参数: `%s`\n", strings.Join(r.Needs, "`, `")))
		}

		b.WriteString(fmt.Sprintf("- 执行结果: %s\n", map[bool]string{true: "PASS", false: "FAIL"}[r.OK]))
		if !r.OK {
			b.WriteString(fmt.Sprintf("- 错误: `%s`\n", safeText(r.Err)))
		}
		b.WriteString("\n")

		if strings.TrimSpace(r.Output) != "" {
			b.WriteString("输出(部分):\n\n")
			b.WriteString("```text\n")
			b.WriteString(r.Output)
			if !strings.HasSuffix(r.Output, "\n") {
				b.WriteString("\n")
			}
			if r.Truncated {
				b.WriteString("...(truncated)\n")
			}
			b.WriteString("```\n\n")
		}
	}

	b.WriteString("## 备注\n\n")
	b.WriteString("- HIDS mustpass 在 `common/yak/yaktest/mustpass/mustpass_base_test.go` 中明确**不并行**执行，避免文件系统监控/临时目录冲突。\n")
	b.WriteString("- `TEST_DIR` 来自 `common/yak/yaktest/mustpass/test-hids/`，运行时会复制到系统临时目录。\n")
	b.WriteString("- 连接/进程相关脚本依赖宿主 OS 能够枚举进程与连接信息；在受限容器环境下可能失败。\n")

	return b.String()
}

func safeText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "`", "'")
	return s
}
