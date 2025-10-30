package forge

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// LoginElementExtractor 登录元素提取器
type LoginElementExtractor struct {
	*aiforge.LiteForge
}

// LoginElements 登录元素结构
type LoginElements struct {
	UsernameSelector     string  `json:"username_selector"`      // 用户名输入框选择器
	PasswordSelector     string  `json:"password_selector"`      // 密码输入框选择器
	LoginButtonSelector  string  `json:"login_button_selector"`  // 登录按钮选择器
	CaptchaImageSelector string  `json:"captcha_image_selector"` // 验证码图片选择器（可选）
	CaptchaInputSelector string  `json:"captcha_input_selector"` // 验证码输入框选择器（可选）
	FormSelector         string  `json:"form_selector"`          // 登录表单选择器（可选）
	Confidence           float64 `json:"confidence"`             // 识别置信度 0-1
	Notes                string  `json:"notes"`                  // 备注信息
}

// NewLoginElementExtractor 创建登录元素提取器
func NewLoginElementExtractor() (*LoginElementExtractor, error) {
	// 定义输出 Schema
	outputSchema := `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["@action", "username_selector", "password_selector", "login_button_selector", "confidence"],
  "properties": {
    "@action": {
      "const": "extract_login_elements",
      "description": "标识当前操作的具体类型"
    },
    "username_selector": {
      "type": "string",
      "description": "用户名输入框的CSS选择器，如 #username, input[name='username'], .login-username 等"
    },
    "password_selector": {
      "type": "string", 
      "description": "密码输入框的CSS选择器，如 #password, input[type='password'], .login-password 等"
    },
    "login_button_selector": {
      "type": "string",
      "description": "登录按钮的CSS选择器，如 #login-btn, button[type='submit'], .login-submit 等"
    },
    "captcha_image_selector": {
      "type": "string",
      "description": "验证码图片的CSS选择器（如果存在），如 #captcha-img, .captcha-image, img[alt*='captcha'] 等"
    },
    "captcha_input_selector": {
      "type": "string",
      "description": "验证码输入框的CSS选择器（如果存在），如 #captcha, input[name='captcha'], .captcha-input 等"
    },
    "form_selector": {
      "type": "string",
      "description": "登录表单的CSS选择器（如果存在），如 #login-form, form[action*='login'], .login-form 等"
    },
    "confidence": {
      "type": "number",
      "minimum": 0.0,
      "maximum": 1.0,
      "description": "识别置信度，0-1之间的数值，表示对识别结果的信心程度"
    },
    "notes": {
      "type": "string",
      "description": "备注信息，如特殊说明、识别难点、建议等"
    }
  },
  "additionalProperties": false
}`

	// 创建 LiteForge 实例
	lf, err := aiforge.NewLiteForge("LoginElementExtractor",
		aiforge.WithLiteForge_Prompt(getLoginExtractionPrompt()),
		aiforge.WithLiteForge_OutputSchemaRaw("extract_login_elements", outputSchema),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create login element extractor: %v", err)
	}

	return &LoginElementExtractor{
		LiteForge: lf,
	}, nil
}

// ExtractLoginElements 提取登录元素
func (lee *LoginElementExtractor) ExtractLoginElements(ctx context.Context, htmlContent string, opts ...aicommon.ConfigOption) (*LoginElements, error) {
	// 准备输入参数
	params := []*ypb.ExecParamItem{
		{
			Key:   "html_content",
			Value: htmlContent,
		},
	}

	opts = append(opts, aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return aiforge.GetQwenAICallback("qwen-plus")(config, req)
	}))

	// 执行提取
	result, err := lee.Execute(ctx, params, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute login element extraction: %v", err)
	}

	// 解析结果
	action := result.Action
	if action == nil {
		return nil, fmt.Errorf("no action found in result")
	}

	// 验证 Action 类型
	if action.GetString("@action") != "extract_login_elements" {
		return nil, fmt.Errorf("unexpected action type: %s", action.GetString("@action"))
	}

	// 构建结果
	loginElements := &LoginElements{
		UsernameSelector:     action.GetString("username_selector"),
		PasswordSelector:     action.GetString("password_selector"),
		LoginButtonSelector:  action.GetString("login_button_selector"),
		CaptchaImageSelector: action.GetString("captcha_image_selector"),
		CaptchaInputSelector: action.GetString("captcha_input_selector"),
		FormSelector:         action.GetString("form_selector"),
		Confidence:           action.GetFloat("confidence"),
		Notes:                action.GetString("notes"),
	}

	// 验证必需字段
	if loginElements.UsernameSelector == "" {
		return nil, fmt.Errorf("username_selector is required but empty")
	}
	if loginElements.PasswordSelector == "" {
		return nil, fmt.Errorf("password_selector is required but empty")
	}
	if loginElements.LoginButtonSelector == "" {
		return nil, fmt.Errorf("login_button_selector is required but empty")
	}

	return loginElements, nil
}

// getLoginExtractionPrompt 获取登录元素提取的提示词
func getLoginExtractionPrompt() string {
	return `# 登录元素提取专家

你是一个专业的网页登录元素识别专家，擅长从HTML内容中准确识别登录相关的表单元素。

## 任务目标
从提供的HTML内容中识别并提取以下登录相关元素的CSS选择器：
1. 用户名输入框
2. 密码输入框  
3. 登录按钮
4. 验证码图片（如果存在）
5. 验证码输入框（如果存在）
6. 登录表单（如果存在）

## 识别原则

### 1. 用户名输入框识别
- 查找 input 标签，类型为 text、email 或没有明确类型
- 关注 name、id、class 属性中包含以下关键词：
  - username, user, login, account, email, phone, mobile
- 优先级：id > name > class > 其他属性
- 避免选择明显不是用户名的输入框（如搜索框、验证码输入框）

### 2. 密码输入框识别
- 查找 input 标签，类型为 password
- 关注 name、id、class 属性中包含以下关键词：
  - password, pass, pwd, secret
- 优先级：id > name > class > 其他属性

### 3. 登录按钮识别
- 查找 button、input[type="submit"] 或 a 标签
- 关注按钮文本内容包含以下关键词：
  - 登录, login, sign in, 进入, 提交, submit, 确认
- 关注 id、class、name 属性中包含：
  - login, submit, signin, btn, button
- 优先级：按钮文本 > id > name > class

### 4. 验证码图片识别
- 查找 img 标签
- 关注 src、alt、title 属性中包含：
  - captcha, verify, code, 验证码, 验证
- 关注图片尺寸通常较小（验证码图片）
- 优先级：alt/title > src > class > id

### 5. 验证码输入框识别
- 查找 input 标签，类型为 text
- 关注 name、id、class 属性中包含：
  - captcha, verify, code, 验证码, 验证
- 通常位于验证码图片附近
- 优先级：id > name > class

### 6. 登录表单识别
- 查找 form 标签
- 关注 action、id、class 属性中包含：
  - login, signin, auth, 登录
- 包含用户名和密码输入框的表单
- 优先级：id > class > action

## 选择器生成规则

### CSS选择器优先级
1. **ID选择器**: #elementId （最高优先级）
2. **属性选择器**: input[name="username"]、input[type="password"]
3. **类选择器**: .className
4. **标签选择器**: input、button
5. **组合选择器**: form .login-input

### 选择器要求
- 选择器必须唯一标识目标元素
- 优先使用简洁、稳定的选择器
- 避免使用过于复杂的选择器
- 确保选择器在页面中唯一

## 置信度评估

根据以下因素评估识别置信度（0-1）：
- **1.0-0.9**: 元素特征非常明显，选择器非常精确
- **0.8-0.7**: 元素特征明显，选择器较为精确
- **0.6-0.5**: 元素特征一般，选择器基本准确
- **0.4-0.3**: 元素特征模糊，选择器可能不准确
- **0.2-0.1**: 元素特征很模糊，选择器很可能不准确
- **0.0**: 无法识别或识别失败

## 输出要求

1. **必须输出**: username_selector, password_selector, login_button_selector, confidence
2. **可选输出**: captcha_image_selector, captcha_input_selector, form_selector, notes
3. **选择器格式**: 标准的CSS选择器格式
4. **置信度**: 0-1之间的数值
5. **备注**: 如有特殊情况或建议，在notes字段中说明

## 注意事项

- 仔细分析HTML结构，不要遗漏任何可能的登录元素
- 考虑多种可能的命名方式和结构
- 注意区分真正的登录元素和其他相似元素
- 如果存在多个候选元素，选择最符合登录场景的
- 对于验证码元素，如果不存在则不要输出对应字段
- 确保输出的选择器在实际页面中能够正确定位元素`
}

// ValidateSelectors 验证选择器的有效性（可选功能）
func (lee *LoginElementExtractor) ValidateSelectors(htmlContent string, elements *LoginElements) []string {
	var errors []string

	// 这里可以添加选择器验证逻辑
	// 例如：检查选择器是否在HTML中存在
	// 可以使用 goquery 或其他HTML解析库来验证

	if elements.UsernameSelector == "" {
		errors = append(errors, "username_selector is empty")
	}
	if elements.PasswordSelector == "" {
		errors = append(errors, "password_selector is empty")
	}
	if elements.LoginButtonSelector == "" {
		errors = append(errors, "login_button_selector is empty")
	}

	return errors
}

// GetSelectorSuggestions 获取选择器建议（可选功能）
func (lee *LoginElementExtractor) GetSelectorSuggestions(htmlContent string) map[string][]string {
	suggestions := make(map[string][]string)

	// 这里可以添加选择器建议逻辑
	// 例如：分析HTML结构，提供备选选择器

	return suggestions
}
