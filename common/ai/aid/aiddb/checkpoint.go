package aiddb

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func AiCheckPointGetRequestParams(c *schema.AiCheckpoint) aitool.InvokeParams {
	var params = make(aitool.InvokeParams)
	result, err := codec.StrConvUnquote(c.RequestQuotedJson)
	if err != nil {
		log.Warnf("unquote response params failed: %v", err)
		return params
	}
	if err := json.Unmarshal([]byte(result), &params); err != nil {
		log.Warnf("unmarshal response params failed: %v", err)
		return params
	}
	return params
}

func AiCheckPointGetResponseParams(c *schema.AiCheckpoint) aitool.InvokeParams {
	var params = make(aitool.InvokeParams)
	result, err := codec.StrConvUnquote(c.ResponseQuotedJson)
	if err != nil {
		log.Warnf("unquote response params failed: %v, sample: %v", err, utils.ShrinkString(c.ResponseQuotedJson, 100))
		return params
	}
	if err := json.Unmarshal([]byte(result), &params); err != nil {
		log.Warnf("unmarshal response params failed: %v", err)
		return params
	}
	return params
}

func AiCheckPointGetToolResult(c *schema.AiCheckpoint) *aitool.ToolResult {
	var res aitool.ToolResult
	result, err := codec.StrConvUnquote(c.ResponseQuotedJson)
	if err != nil {
		log.Warnf("unquote request params failed: %v", err)
		return nil
	}
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		log.Warnf("unmarshal request params failed: %v", err)
		return nil
	}
	return &res
}
