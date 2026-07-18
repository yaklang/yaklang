package aitool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"strconv"

	"github.com/yaklang/yaklang/common/utils"
)

// ToolResult 表示工具调用的结果
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

	if t.CallExpectations != "" {
		fmt.Fprintf(buf, "call_expectations: %s\n", t.CallExpectations)
	}

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
		buf.WriteString(fmt.Sprintf("shrink_result: %#v\n", t.ShrinkResult))
	} else if t.ShrinkSimilarResult != "" { //  shrink similar result second
		buf.WriteString(fmt.Sprintf("shrink_similar_result: %#v\n", t.ShrinkSimilarResult))
	} else {
		// 处理工具执行结果
		switch ret := t.Data.(type) {
		case *ToolExecutionResult:
			// 优先使用 CombinedOutput；兼容旧消费者回退到 stdout/stderr
			combined := ret.CombinedOutput
			if combined == "" {
				combined = ret.Stdout + ret.Stderr
			}
			if combined != "" {
				buf.WriteString(fmt.Sprintf("output: \n%v\n", combined))
			}

			// 处理结果
			result := utils.InterfaceToString(ret.Result)
			if result != "" {
				buf.WriteString(fmt.Sprintf("result: \n%v\n", result))
			}

			// 如果没有任何输出，显示提示信息
			if combined == "" && result == "" {
				buf.WriteString("no output\n")
			}
		default:
			// 处理其他类型的数据
			rawMap := utils.InterfaceToGeneralMap(t.Data)
			if len(rawMap) > 0 {
				// 处理标准输出
				if stdout := utils.MapGetString(rawMap, "stdout"); stdout != "" {
					buf.WriteString(fmt.Sprintf("stdout: \n%v\n", stdout))
				}
				delete(rawMap, "stdout")

				// 处理标准错误
				if stderr := utils.MapGetString(rawMap, "stderr"); stderr != "" {
					buf.WriteString(fmt.Sprintf("stderr: \n%v\n", stderr))
				}
				delete(rawMap, "stderr")

				// 处理结果
				if result := utils.MapGetString(rawMap, "result"); result != "" {
					buf.WriteString(fmt.Sprintf("result: \n%v\n", result))
				}
				delete(rawMap, "result")

				// 处理额外信息
				if len(rawMap) > 0 {
					buf.WriteString(fmt.Sprintf("extra: %s\n", utils.Jsonify(rawMap)))
				}
			} else {
				buf.WriteString(fmt.Sprintf("data: %s\n", utils.Jsonify(t.Data)))
			}
		}
	}

	// 处理错误信息
	if t.Error != "" {
		buf.WriteString(fmt.Sprintf("err: %s\n", t.Error))
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
	buf.WriteString(fmt.Sprintf("data: %s\n", utils.Jsonify(t.Data)))
	if t.Error != "" {
		buf.WriteString(fmt.Sprintf("err: %s\n", t.Error))
	}
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
	bytes, _ := json.Marshal(t)
	return string(bytes)
}
