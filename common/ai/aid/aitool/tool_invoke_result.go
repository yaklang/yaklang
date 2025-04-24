package aitool

import (
	"bytes"
	"encoding/json"
	"fmt"
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
}

func (t *ToolResult) String() string {
	buf := bytes.NewBuffer(nil)
	if t.ID > 0 {
		buf.WriteString(fmt.Sprintf("id: %v; ", t.ID))
	}
	buf.WriteString(fmt.Sprintf("tool_name: %#v\n", t.Name))
	buf.WriteString(fmt.Sprintf("param: %s\n", utils.Jsonify(t.Param)))
	buf.WriteString(fmt.Sprintf("data: %s\n", utils.Jsonify(t.Data)))
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
