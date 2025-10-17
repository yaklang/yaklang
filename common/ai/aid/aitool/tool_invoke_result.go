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
}

func (t *ToolResult) DumpTimelineItem(buf io.Writer) {

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
	if t.ID > 0 {
		buf.WriteString(fmt.Sprintf("id: %v; ", t.ID))
	}
	buf.WriteString(fmt.Sprintf("tool_name: %#v\n", t.Name))

	paramParsed := utils.InterfaceToGeneralMap(t.Param)
	if len(paramParsed) > 0 {
		buf.WriteString("param: \n")
		out, err := yaml.Marshal(paramParsed)
		if err != nil {
			for k, v := range paramParsed {
				buf.WriteString(fmt.Sprintf("  - %v: %s\n", k, v))
			}
		} else {
			for _, line := range utils.ParseStringToRawLines(string(out)) {
				buf.WriteString(fmt.Sprintf("  %s\n", string(line)))
			}
		}
	} else {
		buf.WriteString(fmt.Sprintf("param: %s\n", utils.Jsonify(t.Param)))
	}

	if t.ShrinkResult != "" { // shrink result preface
		buf.WriteString(fmt.Sprintf("shrink_result: %#v\n", t.ShrinkResult))
	} else if t.ShrinkSimilarResult != "" { //  shrink similar result second
		buf.WriteString(fmt.Sprintf("shrink_similar_result: %#v\n", t.ShrinkSimilarResult))
	} else {
		// 处理工具执行结果
		switch ret := t.Data.(type) {
		case *ToolExecutionResult:
			// 处理标准输出
			if ret.Stdout != "" {
				buf.WriteString(fmt.Sprintf("stdout: \n%v\n", string(ret.Stdout)))
			}

			// 处理标准错误
			if ret.Stderr != "" {
				buf.WriteString(fmt.Sprintf("stderr: \n%v\n", string(ret.Stderr)))
			}

			// 处理结果
			result := utils.InterfaceToString(ret.Result)
			if result != "" {
				buf.WriteString(fmt.Sprintf("result: \n%v\n", result))
			}

			// 如果没有任何输出，显示提示信息
			if ret.Stdout == "" && ret.Stderr == "" && result == "" {
				buf.WriteString("no output\n")
			}
		default:
			// 处理其他类型的数据
			rawMap := utils.InterfaceToGeneralMap(t.Data)
			if len(rawMap) > 0 {
				// 处理标准输出
				if stdout := utils.MapGetString(rawMap, "stdout"); stdout != "" {
					buf.WriteString(fmt.Sprintf("stdout: \n%v\n", stdout))
					delete(rawMap, "stdout")
				}

				// 处理标准错误
				if stderr := utils.MapGetString(rawMap, "stderr"); stderr != "" {
					buf.WriteString(fmt.Sprintf("stderr: \n%v\n", stderr))
					delete(rawMap, "stderr")
				}

				// 处理结果
				if result := utils.MapGetString(rawMap, "result"); result != "" {
					buf.WriteString(fmt.Sprintf("result: \n%v\n", result))
					delete(rawMap, "result")
				}

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
