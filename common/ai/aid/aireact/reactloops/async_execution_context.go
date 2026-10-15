package reactloops

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
)

const (
	asyncExecutionContextMaxRows         = 8
	asyncExecutionContextMaxTerminalRows = 4
	asyncExecutionContextMaxBytes        = 4096
)

type asyncExecutionContextEntry struct {
	ID             string                 `yaml:"execute_id"`
	Name           string                 `yaml:"name"`
	State          asyncexecmanager.State `yaml:"state"`
	ElapsedSeconds int64                  `yaml:"elapsed_seconds"`
	Error          string                 `yaml:"error,omitempty"`
}

// renderAsyncExecutionContext samples only this loop's existing manager.
// Rendering never creates a manager, invokes an inspector, or retains outputs.
func (r *ReActLoop) renderAsyncExecutionContext(nonce string) string {
	if r == nil {
		return ""
	}
	r.onReleaseMutex.Lock()
	manager := r.asyncExecutionManager
	r.onReleaseMutex.Unlock()
	if manager == nil {
		return ""
	}
	entries := manager.List()
	if len(entries) == 0 {
		return ""
	}
	var active, terminal []asyncexecmanager.Summary
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.State.IsTerminal() {
			terminal = append(terminal, entry)
		} else {
			active = append(active, entry)
		}
	}
	// Recent completion matters even when an execution was submitted long ago.
	sort.SliceStable(terminal, func(i, j int) bool {
		return terminal[i].EndedAt != nil && (terminal[j].EndedAt == nil || terminal[i].EndedAt.After(*terminal[j].EndedAt))
	})
	if len(terminal) > asyncExecutionContextMaxTerminalRows {
		terminal = terminal[:asyncExecutionContextMaxTerminalRows]
	}
	selected := append(active, terminal...)
	now := time.Now()
	rows := make([]asyncExecutionContextEntry, 0, asyncExecutionContextMaxRows)
	for _, entry := range selected {
		// Keep IDs usable verbatim; unusually large resource IDs remain available
		// through list rather than publishing a truncated, unresolvable ID.
		if len(entry.ID) > 256 {
			continue
		}
		start, end := entry.CreatedAt, now
		if entry.StartedAt != nil {
			start = *entry.StartedAt
		}
		if entry.EndedAt != nil {
			end = *entry.EndedAt
		}
		elapsed := int64(end.Sub(start) / time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		rows = append(rows, asyncExecutionContextEntry{
			ID: entry.ID, Name: asyncExecutionContextText(entry.Metadata.Name, 64), State: entry.State,
			ElapsedSeconds: elapsed, Error: asyncExecutionContextText(entry.Error, 160),
		})
		if len(rows) == asyncExecutionContextMaxRows {
			break
		}
	}
	header := fmt.Sprintf("<|ASYNC_EXECUTIONS_%s|>\n当前循环的异步执行状态（只读快照）。\n"+
		"直接选择顶层 action manage_async_execution_runtime：list 查看列表，inspect 读取输出和结果，wait 等待结束，stop/stop_all 请求停止。它不是 directly_call_tool 的目标。\n", nonce)
	footer := fmt.Sprintf("\n<|ASYNC_EXECUTIONS_END_%s|>", nonce)
	for {
		payload := dumpAsyncExecutionContext(rows, len(entries)-len(rows))
		if len(header)+len(payload)+len(footer) <= asyncExecutionContextMaxBytes {
			return header + payload + footer
		}
		if len(rows) == 0 {
			return ""
		}
		rows = rows[:len(rows)-1]
	}
}

// dumpAsyncExecutionContext renders a human-readable, YAML-style snapshot.
// Quote resource text so newlines cannot introduce fields or prompt delimiters.
func dumpAsyncExecutionContext(rows []asyncExecutionContextEntry, omitted int) string {
	quote := func(value string) string {
		return strings.NewReplacer("<", `\u003c`, ">", `\u003e`).Replace(strconv.Quote(value))
	}
	var dump strings.Builder
	if len(rows) == 0 {
		dump.WriteString("executions: []\n")
	} else {
		dump.WriteString("executions:\n")
		for _, row := range rows {
			fmt.Fprintf(&dump, "  - execute_id: %s\n    name: %s\n    state: %s\n    elapsed_seconds: %d\n",
				quote(row.ID), quote(row.Name), row.State, row.ElapsedSeconds)
			if row.Error != "" {
				fmt.Fprintf(&dump, "    error: %s\n", quote(row.Error))
			}
		}
	}
	fmt.Fprintf(&dump, "omitted: %d", omitted)
	return dump.String()
}

func asyncExecutionContextText(value string, limit int) string {
	for offset := range value {
		if limit == 0 {
			return value[:offset] + "…"
		}
		limit--
	}
	return value
}
