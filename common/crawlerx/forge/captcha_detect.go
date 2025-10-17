package forge

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/aiforge"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

type CatpchaDetector struct {
	*aiforge.LiteForge
}

type CaptchaResult struct {
	CaptchaText     string  `json:"captcha_text"`
	Confidence      float64 `json:"confidence"`
	CaptchaType     string  `json:"captcha_type"`
	ProcessingNotes string  `json:"processing_notes"`
	Suggestions     string  `json:"suggestions"`
}

func NewCaptchaDetector() (*CatpchaDetector, error) {
	// 创建 LiteForge 实例
	lf, err := aiforge.NewLiteForge("CaptchaDetector",
		aiforge.WithLiteForge_Prompt(getCaptchaDetectPrompt()),
		aiforge.WithLiteForge_OutputSchemaRaw("detect_captcha", getCaptchaDetectSchema()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create login element extractor: %v", err)
	}

	return &CatpchaDetector{
		LiteForge: lf,
	}, nil
}

func (lee *CatpchaDetector) DetectCaptcha(ctx context.Context, imgBase64 string, opts ...aid.Option) (*CaptchaResult, error) {
	imageData := []*aicommon.ImageData{
		{
			Data:     []byte(imgBase64),
			IsBase64: true,
		},
	}

	opts = append(opts, aid.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return aiforge.GetQwenAICallback("qwen-vl-max")(config, req)
	}))

	// 执行提取
	result, err := lee.ExecuteEx(ctx, nil, imageData, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute login element extraction: %v", err)
	}

	// 解析结果
	action := result.Action
	if action == nil {
		return nil, fmt.Errorf("no action found in result")
	}

	// 验证 Action 类型
	if action.GetString("@action") != "detect_captcha" {
		return nil, fmt.Errorf("unexpected action type: %s", action.GetString("@action"))
	}

	captchaResult := &CaptchaResult{
		CaptchaText:     action.GetString("captcha_text"),
		Confidence:      action.GetFloat("confidence"),
		CaptchaType:     action.GetString("captcha_type"),
		ProcessingNotes: action.GetString("processing_notes"),
		Suggestions:     action.GetString("suggestions"),
	}

	if captchaResult.CaptchaText == "" {
		return nil, fmt.Errorf("captcha_text is required but empty")
	}
	if captchaResult.Confidence == 0 {
		return nil, fmt.Errorf("confidence is required but empty")
	}

	return captchaResult, nil
}

func getCaptchaDetectPrompt() string {
	return `# 验证码识别专家

你是一个专业的验证码识别专家，擅长从图片中准确识别各种类型的验证码内容，并输出验证码结果以便于用户直接进行填写。

## 最高规则
识别出来的结果只可能包含一下几项, 不可能出现其他结果, 所以不要在结果中出现以下规定以外的字符
1. 数字0-9
2. 小写字母a-z和大写字母A-Z
3. 运算符加减乘
4. 运算结尾的=?, 其中=和?一定是在识别结尾一起出现的, 不可能出现分开的情况
5. 中文字符

## 任务目标
从提供的验证码图片中识别并提取验证码文本内容，支持多种验证码类型：
1. 数字验证码
2. 字母验证码
3. 数字+字母混合验证码
4. 中文验证码
5. 简单数学运算验证码
6. 扭曲、干扰线验证码

## 识别原则

### 1. 数字验证码识别
- 识别纯数字组合（0-9）
- 注意区分相似数字：0和O、1和l、6和9等
- 处理数字间的间距和连接
- **输出**: 直接输出识别到的数字组合

### 2. 字母验证码识别
- 识别大小写英文字母（A-Z, a-z）
- 注意区分相似字母：O和0、I和1、S和5等
- 处理字母的变形和扭曲
- **输出**: 直接输出识别到的字母组合，保持大小写

### 3. 混合验证码识别
- 识别数字+字母的组合
- 注意大小写区分
- 处理字符间的连接和重叠
- **输出**: 直接输出识别到的数字字母混合内容，保持大小写

### 4. 中文验证码识别
- 识别常见中文字符
- 注意相似汉字的区分
- 处理繁体字和简体字
- **输出**: 直接输出识别到的中文字符

### 5. 数学运算验证码
- 识别简单的加减乘运算表达式
- 此时只可能存在数字、加减乘运算符和可能出现在结果的=?
- 请特别注意不要把减号识别成下划线, 不要把乘号识别成x
- 可能在结尾处存在=?
- 注意运算符的识别

### 6. 复杂验证码处理
- 处理扭曲、旋转的字符
- 识别干扰线和噪点
- 处理背景色和前景色的对比
- **输出**: 根据验证码类型按上述规则输出

## 输出规则（重要）

### captcha_text字段输出规则：
1. **数字验证码**: 直接输出数字，如"1234"
2. **字母验证码**: 直接输出字母，保持大小写，如"AbCd"
3. **混合验证码**: 直接输出混合内容，如"A1b2"
4. **中文验证码**: 直接输出中文，如"验证码"
5. **数学运算验证码**: 直接输出运算式
6. **其他类型**: 按识别到的内容直接输出

## 置信度评估

根据以下因素评估识别置信度（0-1）：
- **1.0-0.9**: 验证码清晰，字符标准，识别非常准确
- **0.8-0.7**: 验证码较清晰，字符基本标准，识别较为准确
- **0.6-0.5**: 验证码一般，字符有轻微变形，识别基本准确
- **0.4-0.3**: 验证码模糊，字符变形严重，识别可能不准确
- **0.2-0.1**: 验证码很模糊，字符严重变形，识别很可能不准确
- **0.0**: 无法识别或识别失败

## 输出要求

1. **必须输出**: captcha_text, confidence
2. **可选输出**: captcha_type, processing_notes, suggestions
3. **置信度**: 0-1之间的数值
4. **备注**: 如有特殊情况或建议，在processing_notes字段中说明

## 注意事项

- 仔细分析验证码的每个字符
- 考虑多种可能的字符解释
- 注意区分相似字符
- 如果无法确定某个字符，在processing_notes中说明
- 如果验证码过于模糊无法识别，confidence设为0并说明原因
`
}

func getCaptchaDetectSchema() string {
	return `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["@action", "captcha_text", "confidence"],
  "properties": {
    "@action": {
      "const": "detect_captcha",
      "description": "标识当前操作的具体类型"
    },
    "captcha_text": {
      "type": "string",
      "description": "识别出的验证码文本内容，保持原始大小写和格式"
    },
    "confidence": {
      "type": "number",
      "minimum": 0.0,
      "maximum": 1.0,
      "description": "识别置信度，0-1之间的数值，表示对识别结果的信心程度"
    },
    "captcha_type": {
      "type": "string",
      "enum": ["numeric", "alphabetic", "alphanumeric", "chinese", "math", "mixed", "unknown"],
      "description": "验证码类型：numeric(纯数字), alphabetic(纯字母), alphanumeric(数字字母混合), chinese(中文), math(数学运算), mixed(混合类型), unknown(未知类型)"
    },
    "processing_notes": {
      "type": "string",
      "description": "处理备注信息，如识别难点、特殊说明、不确定的字符等"
    },
    "suggestions": {
      "type": "string",
      "description": "改进建议，如预处理建议、识别策略建议等"
    }
  },
  "additionalProperties": false
}`
}
