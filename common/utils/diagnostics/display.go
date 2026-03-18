package diagnostics

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// DisplayPayload 统一输出载荷，支持表格或树
type DisplayPayload interface {
	Format() string
}

// TablePayload 表格数据
type TablePayload struct {
	Title   string
	Headers []string
	Rows    [][]string
	Opts    []TableOption
}

// Format 实现 DisplayPayload，生成表格字符串
func (p *TablePayload) Format() string {
	if p == nil || len(p.Rows) == 0 {
		if p != nil && p.Title != "" {
			return fmt.Sprintf("No data for: %s", p.Title)
		}
		return ""
	}
	return FormatTable(p.Title, p.Headers, p.Rows, p.Opts...)
}

// TablePayloadFromMeasurements 从 []Measurement 构建 TablePayload
func TablePayloadFromMeasurements(title string, ms []Measurement, opts ...TableOption) *TablePayload {
	headers, rows := MeasurementsToRows(ms, opts...)
	return &TablePayload{Title: title, Headers: headers, Rows: rows, Opts: opts}
}

// --- 统一打印接口：Log / LogLow / LogHigh，LogTable / LogTableLow ---

// formatWithKind 当 kind 非空时添加 [Kind] 标记前缀；多行内容仅首行加，保持表格/树格式可读
func formatWithKind(kind TrackKind, s string) string {
	if kind == "" || kind == TrackKindGeneral || s == "" {
		return s
	}
	if idx := strings.Index(s, "\n"); idx >= 0 {
		return fmt.Sprintf("[%s]%s%s", kind, s[:idx], s[idx:])
	}
	return fmt.Sprintf("[%s] %s", kind, s)
}

// LogLow 简单打印，LevelLow 时输出；kind 非空时带 [Kind] 标记；label 非空时前缀输出
func LogLow(kind TrackKind, label string, msg string) {
	if !Enabled(LevelLow) {
		return
	}
	if label != "" {
		msg = label + ": " + msg
	}
	log.Info(formatWithKind(kind, msg))
}

// Log 格式化打印（表格/树），LevelNormal 时输出；kind 非空时带 [Kind] 标记；label 非空时前缀输出
func Log(kind TrackKind, label string, content string, toStdout bool) {
	if !Enabled(LevelNormal) || content == "" {
		return
	}
	if label != "" {
		content = label + "\n" + content
	}
	out := formatWithKind(kind, content)
	if toStdout {
		fmt.Println(out)
	} else {
		log.Info(out)
	}
}

// LogHigh 关键信息打印，LevelHigh 时输出；kind 非空时带 [Kind] 标记；label 非空时前缀输出
func LogHigh(kind TrackKind, label string, msg string) {
	if !Enabled(LevelHigh) {
		return
	}
	if label != "" {
		msg = label + ": " + msg
	}
	log.Info(formatWithKind(kind, msg))
}

// logTableOutput 内部：按 payload 格式化并输出，调用方需保证 level 已通过 Enabled 检查
func logTableOutput(kind TrackKind, payload DisplayPayload, toStdout bool) {
	if payload == nil {
		return
	}
	content := payload.Format()
	if content == "" {
		return
	}
	out := formatWithKind(kind, content)
	if toStdout {
		fmt.Println(out)
	} else {
		log.Info(out)
	}
}

// LogTableLow LevelLow，输出到 log.Info；内部判断 Enabled(LevelLow)
func LogTableLow(kind TrackKind, payload DisplayPayload) {
	if !Enabled(LevelLow) {
		return
	}
	logTableOutput(kind, payload, false)
}

// LogTable 仅 LevelHigh 时输出表格/树；toStdout 为 true 输出到 stdout，否则 log.Info
func LogTable(kind TrackKind, payload DisplayPayload, toStdout bool) {
	if !Enabled(LevelHigh) {
		return
	}
	logTableOutput(kind, payload, toStdout)
}
