// Package amalgamate 把 mvscan 纯 C99 运行期内核 (native/mvscan/*.c/.h/.inc) 拼为
// "宿主丢两个文件即可编" 的单文件发行: 一个自包含的 mvscan.c (仅依赖公共头 mvscan.h
// 与系统头) + 公共头 mvscan.h. 对应 IMPL 第 5/15 节的 amalgamation 目标.
//
// 拼接规则 (保持与源逐字等价, 仅消除项目内 #include):
//   - mvscan.c 中对 "mvscan_run.inc" 的两处 #include 就地替换为该文件正文 (它被用不同
//     ROW_* 宏 #include 两次, 故必须保留两份内联副本, 各自处在原有 #define/#undef 之间);
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
	"strings"
)

// 源文件名 (位于 srcDir, 即 native/mvscan).
const (
	headerName = "mvscan.h"
	runIncName = "mvscan_run.inc"
	coreName   = "mvscan.c"
)

// generatedBanner 标注产物为自动生成, 杜绝有人误改单文件而非源.
const generatedBanner = `/*
 * mvscan.c - mvscan 纯 C99 运行期内核 (AMALGAMATION 单文件发行, 自动生成, 请勿手改).
 *
 * 本文件由 common/minirehs/tools/amalgamate 从下列源拼接而成 (source of truth):
 *   native/mvscan/mvscan.c   (内核主体)
 *   native/mvscan/mvscan_run.inc  (位并行递推体, 原以两套 ROW_* 宏 #include 两次)
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
	runInc, err := os.ReadFile(filepath.Join(srcDir, runIncName))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", runIncName, err)
	}
	core, err := os.ReadFile(filepath.Join(srcDir, coreName))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", coreName, err)
	}

	inlined, n := inlineRunInc(string(core), string(runInc))
	if n != 2 {
		// mvscan.c 必须恰好两处 #include "mvscan_run.inc" (标量孪生 + SIMD 档);
		// 数目变化说明源结构改动, 拒绝静默产出错误单文件.
		return nil, nil, fmt.Errorf("expected 2 includes of %s in %s, found %d", runIncName, coreName, n)
	}

	var buf bytes.Buffer
	buf.WriteString(generatedBanner)
	buf.WriteString(inlined)
	return buf.Bytes(), header, nil
}

// inlineRunInc 把 core 中所有对 runIncName 的 #include 行替换为 incBody 正文, 返回替换次数.
// 内联段以可见标记包裹, 便于阅读产物时定位来源.
func inlineRunInc(core, incBody string) (string, int) {
	lines := strings.Split(core, "\n")
	out := make([]string, 0, len(lines)+64)
	count := 0
	for _, ln := range lines {
		if isRunIncInclude(ln) {
			count++
			out = append(out, fmt.Sprintf("/* >>> begin inlined %s (copy %d) >>> */", runIncName, count))
			out = append(out, strings.TrimRight(incBody, "\n"))
			out = append(out, fmt.Sprintf("/* <<< end inlined %s (copy %d) <<< */", runIncName, count))
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n"), count
}

// isRunIncInclude 判定一行是否是对 mvscan_run.inc 的本地 #include (忽略前后空白).
func isRunIncInclude(line string) bool {
	t := strings.TrimSpace(line)
	return t == `#include "`+runIncName+`"`
}
