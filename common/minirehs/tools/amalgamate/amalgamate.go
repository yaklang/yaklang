// Package amalgamate 把 mvscan 纯 C99 运行期内核 (native/mvscan/*.c/.h/.inc) 拼为
// "宿主丢两个文件即可编" 的单文件发行: 一个自包含的 mvscan.c (仅依赖公共头 mvscan.h
// 与系统头) + 公共头 mvscan.h. 对应 IMPL 第 5/15 节的 amalgamation 目标.
//
// 拼接规则 (保持与源逐字等价, 仅消除项目内 #include):
//   - mvscan.c 中对每个 mvscan_run*.inc (递推体模板) 的 #include 就地替换为该文件正文.
//     每个 .inc 被以不同 ROW_* 宏 #include 两次 (标量孪生 + SIMD 档), 故必须保留两份内联
//     副本, 各自处在原有 #define/#undef 之间;
//   - 保留对公共头 "mvscan.h" 的 #include (随发行一同提供);
//   - 不改动任何代码语义, 仅做文本内联, 因此 amalgamation 与源 TU 预处理后完全一致.
//
// 关键词: mvscan, amalgamation, single file, pure C99, portable kernel
package amalgamate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 源文件名 (位于 srcDir, 即 native/mvscan).
const (
	headerName = "mvscan.h"
	coreName   = "mvscan.c"
)

// incIncludePrefix / incIncludeSuffix 划定 core 中需内联的本地 .inc #include 行:
// 形如 #include "mvscan_run.inc" 或 #include "mvscan_run_anchored.inc".
const (
	incIncludePrefix = `#include "mvscan_run`
	incIncludeSuffix = `.inc"`
)

// 每个 .inc 模板被以两套 ROW_* 宏 #include 两次 (标量孪生 + SIMD 档);
// 数目偏离说明源结构改动, 拒绝静默产出错误单文件.
const expectedIncludesPerInc = 2

// generatedBanner 标注产物为自动生成, 杜绝有人误改单文件而非源.
const generatedBanner = `/*
 * mvscan.c - mvscan 纯 C99 运行期内核 (AMALGAMATION 单文件发行, 自动生成, 请勿手改).
 *
 * 本文件由 common/minirehs/tools/amalgamate 从下列源拼接而成 (source of truth):
 *   native/mvscan/mvscan.c   (内核主体)
 *   native/mvscan/mvscan_run.inc            (位并行递推体, 以两套 ROW_* 宏 #include 两次)
 *   native/mvscan/mvscan_run_anchored.inc   (锚定式递推体, 以两套 ROW_* 宏 #include 两次)
 * 公共 API 头随附 mvscan.h (与 native/mvscan/mvscan.h 逐字一致).
 *
 * 用法: 宿主工程仅需此 mvscan.c + mvscan.h 两个文件, 任意 C99 编译器一条命令即可编译,
 * 无需 CMake / 第三方库. 若需修改, 请改 native/mvscan/ 源后重新生成 (有漂移护栏测试).
 *
 * 关键词: mvscan, amalgamation, single file, pure C99, drop-in
 */
`

// Build 读取 srcDir (native/mvscan) 下的源, 返回单文件 mvscan.c 与公共头 mvscan.h 的内容.
func Build(srcDir string) (cFile []byte, hFile []byte, err error) {
	header, err := os.ReadFile(filepath.Join(srcDir, headerName))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", headerName, err)
	}
	core, err := os.ReadFile(filepath.Join(srcDir, coreName))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", coreName, err)
	}

	// 收集 core 中所有本地 .inc #include 引用的文件名 (去重 + 排序, 产物稳定).
	incNames := localIncIncludes(string(core))
	if len(incNames) == 0 {
		return nil, nil, fmt.Errorf("no %s* includes found in %s", incIncludePrefix, coreName)
	}
	sort.Strings(incNames)

	// 读入每个 .inc 正文.
	incBodies := make(map[string]string, len(incNames))
	for _, name := range incNames {
		body, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", name, err)
		}
		incBodies[name] = string(body)
	}

	inlined, _, err := inlineIncFiles(string(core), incBodies)
	if err != nil {
		return nil, nil, err
	}

	var buf bytes.Buffer
	buf.WriteString(generatedBanner)
	buf.WriteString(inlined)
	return buf.Bytes(), header, nil
}

// inlineIncFiles 把 core 中所有对 incBodies 键的 #include 行替换为对应正文.
// 每个 .inc 必须恰好出现 expectedIncludesPerInc 次 (标量孪生 + SIMD 档),
// 数目变化说明源结构改动, 拒绝静默产出错误单文件.
func inlineIncFiles(core string, incBodies map[string]string) (string, map[string]int, error) {
	lines := strings.Split(core, "\n")
	out := make([]string, 0, len(lines)+64)
	counts := make(map[string]int, len(incBodies))
	copySeen := make(map[string]int, len(incBodies))
	for _, ln := range lines {
		name, ok := parseLocalIncInclude(ln)
		if !ok {
			out = append(out, ln)
			continue
		}
		body, has := incBodies[name]
		if !has {
			return "", nil, fmt.Errorf("core includes %s but no body provided", name)
		}
		counts[name]++
		copySeen[name]++
		out = append(out, fmt.Sprintf("/* >>> begin inlined %s (copy %d) >>> */", name, copySeen[name]))
		out = append(out, strings.TrimRight(body, "\n"))
		out = append(out, fmt.Sprintf("/* <<< end inlined %s (copy %d) <<< */", name, copySeen[name]))
	}
	for name, n := range counts {
		if n != expectedIncludesPerInc {
			return "", nil, fmt.Errorf("expected %d includes of %s in %s, found %d",
				expectedIncludesPerInc, name, coreName, n)
		}
	}
	return strings.Join(out, "\n"), counts, nil
}

// localIncIncludes 扫描 core, 返回其中所有 mvscan_run*.inc 本地 #include 引用的文件名 (去重).
func localIncIncludes(core string) []string {
	seen := map[string]struct{}{}
	for _, ln := range strings.Split(core, "\n") {
		if name, ok := parseLocalIncInclude(ln); ok {
			seen[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}

// parseLocalIncInclude 判定一行是否是对某个 mvscan_run*.inc 的本地 #include (忽略前后空白);
// 若是, 返回该 .inc 文件名 (如 "mvscan_run.inc" / "mvscan_run_anchored.inc").
func parseLocalIncInclude(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, incIncludePrefix) || !strings.HasSuffix(t, incIncludeSuffix) {
		return "", false
	}
	// 取引号内的文件名: #include "mvscan_runXXX.inc"
	q := strings.Index(t, `"`)
	if q < 0 {
		return "", false
	}
	end := strings.LastIndex(t, `"`)
	if end <= q {
		return "", false
	}
	name := t[q+1 : end]
	if !strings.HasPrefix(name, "mvscan_run") || !strings.HasSuffix(name, ".inc") {
		return "", false
	}
	return name, true
}
