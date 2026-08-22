package aitool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// ToolResult represents the tool-call protocol result.
//
// Success is a legacy wire field. It means that invocation/validation/callback
// protocol completed and produced a result envelope; it does NOT assert that a
// command exited with zero, an HTTP response was 2xx, or the user's goal was
// achieved. Consumers must inspect Data (normally ToolExecutionResult.Result)
// for execution semantics.
type ToolResult struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Param       any    `json:"param"`
	Success     bool   `json:"success"`
	Data        any    `json:"data,omitempty"`
	Error       string `json:"error,omitempty"`
	ToolCallID  string `json:"call_tool_id,omitempty"` // 用于标识调用的工具ID，通常是一个唯一标识符

	// shrink_similar_result 表示相似缩略信息，是相似度过高的工具调用引发的压缩。
	ShrinkSimilarResult string `json:"shrink_similar_result,omitempty"`

	// shrink_similar_result 表示缩略信息，是由于时间线内容过多引发的压缩。
	ShrinkResult string `json:"shrink_result,omitempty"`

	CallExpectations string `json:"call_expectations,omitempty"`

	// OmitParamsInTimeline indicates that the params have already been emitted as
	// a dedicated timeline item (for example [DIRECT_CALL_PARAMS]) and should not
	// be duplicated when this result is rendered into the timeline.
	OmitParamsInTimeline bool `json:"omit_params_in_timeline,omitempty"`
}

type ToolResultDumpOptions struct {
	IncludeParams bool
}

type ToolResultDumpOption func(*ToolResultDumpOptions)

func WithToolResultDumpParams(include bool) ToolResultDumpOption {
	return func(opts *ToolResultDumpOptions) {
		opts.IncludeParams = include
	}
}

func (t *ToolResult) DumpTimelineItem(buf io.Writer, options ...ToolResultDumpOption) {
	opts := &ToolResultDumpOptions{IncludeParams: true}
	for _, option := range options {
		option(opts)
	}
	if t.ID > 0 {
		fmt.Fprintf(buf, "id: %v; ", t.ID)
	}
	fmt.Fprintf(buf, "tool_name: %#v\n", t.Name)

	if opts.IncludeParams {
		t.dumpTimelineParams(buf)
	}

	t.dumpTimelineResult(buf)
}

func (t *ToolResult) dumpTimelineParams(buf io.Writer) {
	paramParsed := utils.InterfaceToGeneralMap(t.Param)
	if len(paramParsed) > 0 {
		fmt.Fprintln(buf, "param:")
		out, err := yaml.Marshal(paramParsed)
		if err != nil {
			// 旧实现给 fallback 行加 '  - ' 前缀, 配合 yaml-marshal 路径的统一
			// 缩进逻辑. 现在统一拍平不再外加 '  ', 顶头 '- key: value' 仍然
			// 是合法 yaml.
			// 关键词: ToolResult.String fallback 去外层缩进
			for k, v := range paramParsed {
				fmt.Fprintf(buf, "- %v: %s\n", k, v)
			}
		} else {
			// yaml.Marshal 自身已经产生合法相对缩进 (顶层 key 顶头, 嵌套 value
			// 缩 2/4). 历史上这里再外套一层 '  ' 是为了把 'param:' 与其下的
			// yaml body 在文本上看起来"嵌套"得更明显, 但对 LLM 而言纯属冗余
			// token, 还会让 'command: |-' 块的命令行多出一层视觉 6 空格缩
			// 进 (yaml 4 + 外套 2). 直接拼 yaml 原文, 既减 token 又仍可被
			// yaml.Unmarshal 正确解析. yaml.Marshal 输出末尾自带 '\n'.
			// 关键词: ToolResult.String yaml 顶层不再外套 '  ', timeline prompt 紧凑
			_, _ = buf.Write(out)
		}
	} else {
		fmt.Fprintf(buf, "param: %s\n", utils.Jsonify(t.Param))
	}
}

func (t *ToolResult) dumpTimelineResult(writer io.Writer) {
	buf := bytes.NewBuffer(nil)

	if t.ShrinkResult != "" { // shrink result preface
		buf.WriteString(t.ShrinkResult)
		if !strings.HasSuffix(t.ShrinkResult, "\n") {
			buf.WriteByte('\n')
		}
	} else if t.ShrinkSimilarResult != "" { //  shrink similar result second
		buf.WriteString(t.ShrinkSimilarResult)
		if !strings.HasSuffix(t.ShrinkSimilarResult, "\n") {
			buf.WriteByte('\n')
		}
	} else {
		// Render semantic execution facts before observation logs. A model must
		// not infer execution success from the wording of stdout/stderr.
		switch ret := t.Data.(type) {
		case string:
			if ret == "" {
				buf.WriteString("execution_result: null\n")
			} else if strings.HasPrefix(ret, "RESULT:") || strings.HasPrefix(ret, "OBSERVATIONS:") || strings.HasPrefix(ret, "COMBINED OUTPUT:") {
				// Data already has framework packaging (RESULT / OBSERVATIONS / ARTIFACT),
				// no need for an extra "data:" prefix that just duplicates semantics.
				buf.WriteString(ret)
				if !strings.HasSuffix(ret, "\n") {
					buf.WriteByte('\n')
				}
			} else {
				buf.WriteString("execution_result:\n")
				buf.WriteString(ret)
				if !strings.HasSuffix(ret, "\n") {
					buf.WriteByte('\n')
				}
			}
		case *ToolExecutionResult:
			result := utils.InterfaceToString(ret.Result)
			if result != "" {
				buf.WriteString(fmt.Sprintf("execution_result:\n%v\n", result))
			} else {
				buf.WriteString("execution_result: null\n")
			}

			// Prefer CombinedOutput; retain stdout/stderr fallback for historical
			// envelopes. These are observations, not an outcome verdict.
			combined := ret.CombinedOutput
			if combined == "" {
				combined = ret.Stdout + ret.Stderr
			}
			if combined != "" {
				buf.WriteString(fmt.Sprintf("observations:\n%v\n", combined))
			}
		default:
			// Handle legacy map envelopes without treating log fields as verdicts.
			rawMap := utils.InterfaceToGeneralMap(t.Data)
			if len(rawMap) > 0 {
				if result, ok := rawMap["result"]; ok {
					buf.WriteString(fmt.Sprintf("execution_result:\n%v\n", utils.InterfaceToString(result)))
				} else {
					executionFields := make(map[string]any, len(rawMap))
					for key, value := range rawMap {
						if key != "stdout" && key != "stderr" && key != "combined_output" {
							executionFields[key] = value
						}
					}
					if len(executionFields) == 0 {
						buf.WriteString("execution_result: null\n")
					} else {
						buf.WriteString(fmt.Sprintf("execution_result: %s\n", utils.Jsonify(executionFields)))
					}
				}

				observationParts := bytes.NewBuffer(nil)
				if stdout := utils.MapGetString(rawMap, "stdout"); stdout != "" {
					observationParts.WriteString(fmt.Sprintf("stdout:\n%v\n", stdout))
				}
				if stderr := utils.MapGetString(rawMap, "stderr"); stderr != "" {
					observationParts.WriteString(fmt.Sprintf("stderr:\n%v\n", stderr))
				}
				if observationParts.Len() > 0 {
					buf.WriteString("observations:\n")
					buf.Write(observationParts.Bytes())
				}
			} else {
				buf.WriteString(fmt.Sprintf("execution_result: %s\n", utils.Jsonify(t.Data)))
			}
		}
	}

	// Error is reserved for invocation/protocol failures. Domain execution
	// errors belong in ToolExecutionResult.Result.
	if t.Error != "" {
		buf.WriteString(fmt.Sprintf("protocol_error: %s\n", t.Error))
	} else if !t.Success {
		buf.WriteString("protocol_error: tool call did not complete\n")
	}

	_, _ = writer.Write(buf.Bytes())
}

func (t *ToolResult) GetShrinkResult() string {
	if t.ShrinkResult != "" {
		return t.ShrinkResult
	}
	return t.ShrinkSimilarResult
}

func (t *ToolResult) SetShrinkResult(i string) {
	t.ShrinkResult = i
}

func (t *ToolResult) GetShrinkSimilarResult() string {
	if t.ShrinkSimilarResult != "" {
		return t.ShrinkSimilarResult
	}
	return t.ShrinkResult
}

func (t *ToolResult) String() string {
	buf := bytes.NewBuffer(nil)
	t.DumpTimelineItem(buf, WithToolResultDumpParams(!t.OmitParamsInTimeline))
	return buf.String()
}

func (t *ToolResult) StringWithoutID() string {
	buf := bytes.NewBuffer(nil)
	buf.WriteString(fmt.Sprintf("tool_name: %#v\n", t.Name))
	buf.WriteString(fmt.Sprintf("param: %s\n", utils.Jsonify(t.Param)))
	t.dumpTimelineResult(buf)
	return buf.String()
}

func (t *ToolResult) GetID() int64 {
	return t.ID
}

func (t *ToolResult) QuoteName() string {
	return strconv.Quote(t.Name)
}

func (t *ToolResult) QuoteDescription() string {
	return strconv.Quote(t.Description)
}

func (t *ToolResult) QuoteError() string {
	return strconv.Quote(t.Error)
}

func (t *ToolResult) QuoteResult() string {
	raw, _ := json.Marshal(t.Data)
	return string(raw)
}

func (t *ToolResult) QuoteParams() string {
	raw, _ := json.Marshal(t.Param)
	return string(raw)
}

func (t *ToolResult) Dump() string {
	type observations struct {
		Stdout         string `json:"stdout,omitempty"`
		Stderr         string `json:"stderr,omitempty"`
		CombinedOutput string `json:"combined_output,omitempty"`
	}
	type dumpView struct {
		ID                int64         `json:"id,omitempty"`
		Name              string        `json:"name"`
		Description       string        `json:"description,omitempty"`
		Param             any           `json:"param,omitempty"`
		ExecutionResult   any           `json:"execution_result"`
		Observations      *observations `json:"observations,omitempty"`
		ProtocolCompleted bool          `json:"protocol_completed"`
		ProtocolError     string        `json:"protocol_error,omitempty"`
		ToolCallID        string        `json:"call_tool_id,omitempty"`
	}

	view := dumpView{
		ID:                t.ID,
		Name:              t.Name,
		Description:       t.Description,
		Param:             t.Param,
		ExecutionResult:   t.Data,
		ProtocolCompleted: t.Success,
		ProtocolError:     t.Error,
		ToolCallID:        t.ToolCallID,
	}
	if execution, ok := t.Data.(*ToolExecutionResult); ok {
		view.ExecutionResult = execution.Result
		if execution.Stdout != "" || execution.Stderr != "" || execution.CombinedOutput != "" {
			view.Observations = &observations{
				Stdout:         execution.Stdout,
				Stderr:         execution.Stderr,
				CombinedOutput: execution.CombinedOutput,
			}
		}
	}
	raw, _ := json.Marshal(view)
	return string(raw)
}
