package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// normalizeErr 把错误信息里的可变部分（数字、hash、地址、具体类型名）抽象掉，便于聚类
// 重点提取错误链的"叶子原因"，去掉方法名等高变化标识符
func normalizeErr(s string) string {
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[:idx]
	}
	// 取最后一个 "failed: " 或 "failed, " 之后的叶子原因
	for _, sep := range []string{"failed: ", "failed, ", "error: "} {
		if idx := strings.LastIndex(s, sep); idx >= 0 {
			s = s[idx+len(sep):]
		}
	}
	// 去掉 "dump method xxx failed" 之类前缀里的方法名
	s = regexp.MustCompile(`dump method \w+`).ReplaceAllString(s, "dump method NAME")
	s = regexp.MustCompile(`0x[0-9a-fA-F]+`).ReplaceAllString(s, "0xADDR")
	s = regexp.MustCompile(`\b\d+\b`).ReplaceAllString(s, "N")
	s = regexp.MustCompile(`[a-zA-Z0-9_./]+\.go:N`).ReplaceAllString(s, "FILE")
	// 去掉引号内容
	s = regexp.MustCompile(`"[^"]*"`).ReplaceAllString(s, `"X"`)
	s = regexp.MustCompile(`'[^']*'`).ReplaceAllString(s, `'X'`)
	return strings.TrimSpace(s)
}

// normalizeSyntaxErr 提取 ANTLR 语法错误的第一条 reason 并归一化
func normalizeSyntaxErr(s string) string {
	lines := strings.Split(s, "\n")
	var first string
	for _, ln := range lines {
		if idx := strings.Index(ln, "reason: "); idx >= 0 {
			first = strings.TrimSpace(ln[idx+len("reason: "):])
			break
		}
	}
	if first == "" {
		return normalizeErr(s)
	}
	// 抽象具体 token 内容，保留错误结构
	first = regexp.MustCompile(`'[^']*'`).ReplaceAllString(first, "'X'")
	// 抽象 expecting {大括号集合}
	first = regexp.MustCompile(`expecting \{[^}]*\}`).ReplaceAllString(first, "expecting {SET}")
	first = regexp.MustCompile(`\b\d+\b`).ReplaceAllString(first, "N")
	return strings.TrimSpace(first)
}

// TestCategorizeDecompileErrors 对 jdsc 输出的 decompile-err-*.class 重跑反编译并聚类错误
func TestCategorizeDecompileErrors(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/error-jdsc"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}
	type sample struct {
		file string
		raw  []byte
	}
	histogram := map[string]int{}
	example := map[string]string{}
	count := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "decompile-err-") || !strings.HasSuffix(name, ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		count++
		func() {
			defer func() {
				if r := recover(); r != nil {
					key := normalizeErr("PANIC: " + fmt.Sprint(r))
					histogram[key]++
					if _, ok := example[key]; !ok {
						example[key] = name
					}
				}
			}()
			_, derr := javaclassparser.Decompile(raw)
			if derr != nil {
				key := normalizeErr(derr.Error())
				histogram[key]++
				if _, ok := example[key]; !ok {
					example[key] = name
				}
			} else {
				histogram["(now-success)"]++
			}
		}()
		_ = sample{}
	}
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range histogram {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	t.Logf("==== DECOMPILE ERROR CATEGORIES (total files=%d, unique=%d) ====", count, len(list))
	for _, item := range list {
		t.Logf("[%4d] %s   (e.g. %s)", item.v, item.k, example[item.k])
	}
}

// TestSmallestDecompileErr 找出指定错误子串（默认 multiple next）下最小的若干 class 文件，便于诊断
// 用 DEC_ERR_FILTER 环境变量指定错误子串
func TestSmallestDecompileErr(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/error-jdsc"
	}
	filter := os.Getenv("DEC_ERR_FILTER")
	if filter == "" {
		filter = "multiple next"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}
	type item struct {
		name string
		size int
	}
	var matched []item
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "decompile-err-") || !strings.HasSuffix(name, ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var derr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					derr = fmt.Errorf("PANIC: %v", r)
				}
			}()
			_, derr = javaclassparser.Decompile(raw)
		}()
		if derr != nil && strings.Contains(derr.Error(), filter) {
			matched = append(matched, item{name, len(raw)})
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].size < matched[j].size })
	t.Logf("==== smallest classes failing with %q (total=%d) ====", filter, len(matched))
	for i, it := range matched {
		if i >= 12 {
			break
		}
		t.Logf("[%6d bytes] %s", it.size, it.name)
	}
}

// TestRawDecompileErr 打印指定文件（DEC_ERR_FILE）的完整反编译错误（含方法名）
func TestRawDecompileErr(t *testing.T) {
	file := os.Getenv("DEC_ERR_FILE")
	if file == "" {
		t.Skip("set DEC_ERR_FILE")
	}
	if !filepath.IsAbs(file) {
		dir := os.Getenv("JDSC_DIR")
		if dir == "" {
			dir = "/tmp/error-jdsc"
		}
		file = filepath.Join(dir, file)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var out string
	var derr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				derr = fmt.Errorf("PANIC: %v", r)
			}
		}()
		out, derr = javaclassparser.Decompile(raw)
	}()
	if derr != nil {
		t.Logf("DECOMPILE ERROR: %v", derr)
	} else {
		t.Logf("DECOMPILE OK, len=%d\n%s", len(out), out)
	}
}

// TestCategorizeSyntaxErrors 对 jdsc 输出的 syntax-error--*.class 重跑反编译+前端解析并聚类语法错误
func TestCategorizeSyntaxErrors(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/error-jdsc"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}
	histogram := map[string]int{}
	example := map[string]string{}
	count := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "syntax-error--") || !strings.HasSuffix(name, ".java") {
			continue
		}
		results, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		count++
		_, ferr := java2ssa.Frontend(string(results))
		if ferr != nil {
			key := normalizeSyntaxErr(ferr.Error())
			histogram[key]++
			if _, ok := example[key]; !ok {
				example[key] = name
			}
		} else {
			histogram["(now-success)"]++
		}
	}
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range histogram {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	t.Logf("==== SYNTAX ERROR CATEGORIES (total files=%d, unique=%d) ====", count, len(list))
	for _, item := range list {
		t.Logf("[%4d] %s   (e.g. %s)", item.v, item.k, example[item.k])
	}
}

// TestVerifyDecompileFix 重新反编译所有 syntax-error 和 decompile-err 样本，统计修复后的改善
func TestVerifyDecompileFix(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/error-jdsc"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}

	// 1) syntax-error 样本：重新反编译 + 重新 Frontend
	var syntaxTotal, syntaxFixed, syntaxStill, syntaxNowDecompileFail int
	stillCat := map[string]int{}
	stillExample := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "syntax-error--") || !strings.HasSuffix(name, ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		syntaxTotal++
		results, derr := javaclassparser.Decompile(raw)
		if derr != nil {
			syntaxNowDecompileFail++
			continue
		}
		_, ferr := java2ssa.Frontend(results)
		if ferr == nil {
			syntaxFixed++
		} else {
			syntaxStill++
			cat := normalizeSyntaxErr(ferr.Error())
			stillCat[cat]++
			if _, ok := stillExample[cat]; !ok {
				stillExample[cat] = name
			}
		}
	}

	// 2) decompile-err 样本：重新反编译；不仅看是否报错，还要看输出是否语法合法
	var decTotal, decFixed, decStill, decFixedValid, decFixedSyntaxBad int
	decStillCat := map[string]int{}
	decExample := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "decompile-err-") || !strings.HasSuffix(name, ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		decTotal++
		var out string
		var derr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					derr = fmt.Errorf("PANIC: %v", r)
				}
			}()
			out, derr = javaclassparser.Decompile(raw)
		}()
		if derr == nil {
			decFixed++
			if _, ferr := java2ssa.Frontend(out); ferr == nil {
				decFixedValid++
			} else {
				decFixedSyntaxBad++
			}
		} else {
			decStill++
			cat := normalizeErr(derr.Error())
			decStillCat[cat]++
			if _, ok := decExample[cat]; !ok {
				decExample[cat] = name
			}
		}
	}

	t.Logf("==== SYNTAX: total=%d, FIXED=%d, still=%d, nowDecompileFail=%d ====", syntaxTotal, syntaxFixed, syntaxStill, syntaxNowDecompileFail)
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range stillCat {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	for _, item := range list {
		t.Logf("[STILL-SYNTAX %4d] %s  (e.g. %s)", item.v, item.k, stillExample[item.k])
	}

	t.Logf("==== DECOMPILE: total=%d, FIXED(no-error)=%d (valid-syntax=%d, syntax-bad=%d), still-error=%d ====", decTotal, decFixed, decFixedValid, decFixedSyntaxBad, decStill)
	var dlist []kv
	for k, v := range decStillCat {
		dlist = append(dlist, kv{k, v})
	}
	sort.Slice(dlist, func(i, j int) bool { return dlist[i].v > dlist[j].v })
	for _, item := range dlist {
		t.Logf("[STILL-DEC %4d] %s  (e.g. %s)", item.v, item.k, decExample[item.k])
	}
}

// TestDiagnoseRemaining 重新反编译当前仍失败的 syntax-error 样本，按类别打印最新输出+错误上下文
func TestDiagnoseRemaining(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/error-jdsc"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}
	perCat := map[string]int{}
	maxPerCat := 3
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "syntax-error--") || !strings.HasSuffix(name, ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		results, derr := javaclassparser.Decompile(raw)
		if derr != nil {
			continue
		}
		_, ferr := java2ssa.Frontend(results)
		if ferr == nil {
			continue
		}
		cat := normalizeSyntaxErr(ferr.Error())
		if perCat[cat] >= maxPerCat {
			continue
		}
		perCat[cat]++
		reasonLine := ""
		for _, ln := range strings.Split(ferr.Error(), "\n") {
			if strings.Contains(ln, "reason: ") {
				reasonLine = strings.TrimSpace(ln)
				break
			}
		}
		t.Logf("\n######## CAT: %s\n# FILE: %s\n# REASON: %s", cat, name, reasonLine)
		ctxLines := []string{}
		inCtx := false
		for _, ln := range strings.Split(ferr.Error(), "\n") {
			if strings.HasPrefix(ln, "----") && strings.HasSuffix(strings.TrimSpace(ln), "----") {
				inCtx = !inCtx
				continue
			}
			if inCtx {
				ctxLines = append(ctxLines, ln)
			}
		}
		for _, ln := range ctxLines {
			t.Logf("    %s", ln)
		}
	}
}

// TestSyntaxErrorSamples 对每个语法错误类别打印若干真实样本（真实 reason + 出错源码上下文）
func TestSyntaxErrorSamples(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/error-jdsc"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}
	perCat := map[string]int{}
	maxPerCat := 4
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "syntax-error--") || !strings.HasSuffix(name, ".java") {
			continue
		}
		results, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		_, ferr := java2ssa.Frontend(string(results))
		if ferr == nil {
			continue
		}
		cat := normalizeSyntaxErr(ferr.Error())
		if perCat[cat] >= maxPerCat {
			continue
		}
		perCat[cat]++
		// 提取第一条 reason 行和上下文
		full := ferr.Error()
		reasonLine := ""
		for _, ln := range strings.Split(full, "\n") {
			if strings.Contains(ln, "reason: ") {
				reasonLine = strings.TrimSpace(ln)
				break
			}
		}
		t.Logf("\n######## CATEGORY: %s\n# FILE: %s\n# REASON: %s", cat, name, reasonLine)
		// 打印出错点附近的反编译源码（从 context 块里取）
		ctxLines := []string{}
		inCtx := false
		for _, ln := range strings.Split(full, "\n") {
			if strings.HasPrefix(ln, "----") && strings.HasSuffix(strings.TrimSpace(ln), "----") {
				inCtx = !inCtx
				continue
			}
			if inCtx {
				ctxLines = append(ctxLines, ln)
			}
		}
		for _, ln := range ctxLines {
			t.Logf("    %s", ln)
		}
	}
}
