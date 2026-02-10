package aispec

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/utils/imageutils"

	"github.com/h2non/filetype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

type AIConfig struct {
	// gateway network config
	BaseURL string
	Domain  string `json:"domain" app:"name:domain,verbose:第三方加速域名,id:4"`
	NoHttps bool   `json:"no_https" app:"name:no_https,verbose:NoHttps,desc:是否禁用使用https请求api,id:3"`

	// basic model
	Model    string  `json:"model" app:"name:model,verbose:模型名称,id:2,type:list,required:true"`
	Timeout  float64 // `app:"name:请求超时时长"`
	Deadline time.Time

	APIKey string `json:"api_key" app:"name:api_key,verbose:ApiKey,desc:APIKey / Token,required:true,id:1"`
	Proxy  string `json:"proxy" app:"name:proxy,verbose:代理地址,id:5"`
	Host   string
	Port   int

	StreamHandler       func(io.Reader)
	ReasonStreamHandler func(reader io.Reader)
	Type                string `json:"Type"`
	Context             context.Context

	FunctionCallRetryTimes int

	HTTPErrorHandler func(error)

	Images []*ImageDescription

	Headers             []*ypb.HTTPHeader
	EnableThinking      bool
	EnableThinkingField string
	EnableThinkingValue any

	// ToolCallCallback is called when the AI response contains tool_calls.
	// If set, tool_calls will NOT be converted to <|TOOL_CALL...|> format in the output stream.
	// If not set, the original behavior (converting to <|TOOL_CALL...|> format) is preserved.
	ToolCallCallback func([]*ToolCall)

	// Tools defines the available tools that the model may call
	Tools []Tool
	// ToolChoice controls which (if any) tool is called by the model
	ToolChoice any
}

func WithExtraHeader(headers ...*ypb.HTTPHeader) AIConfigOption {
	return func(c *AIConfig) {
		c.Headers = append(c.Headers, headers...)
	}
}

func WithExtraHeaderString(key string, value string) AIConfigOption {
	return func(c *AIConfig) {
		c.Headers = append(c.Headers, &ypb.HTTPHeader{
			Header: key,
			Value:  value,
		})
	}
}

func WithEnableThinkingEx(thinkField string, thinkValue any) AIConfigOption {
	return func(config *AIConfig) {
		if thinkField != "" && thinkValue != nil {
			config.EnableThinkingField = thinkField
			config.EnableThinkingValue = thinkValue
		}
	}
}

// WithEnableThinking 启用think模式，目前只有当ai模型类型为`volcengine`类型也就是豆包相关模型时此配置才生效。
//
// 参数：
// - t(any): 思维链配置
//
// 返回值：
// - r1(AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 示例 1: 基础用法 - 启用思维模式
// println("=== 示例 1: 基础思维模式 ===")
// response, err = ai.Chat(
//
//	"请分析 SQL 注入漏洞的原理和常见的防御方法",
//	ai.type("volcengine"),
//	ai.apiKey("你的API密钥"),  // 请替换为实际的 API Key
//	ai.model("doubao-lite-4k"),  // 或使用其他豆包模型
//	ai.thinking(true),  // 启用思维模式
//
// )
//
//	if err != nil {
//	   die(err)
//	}
//
// println("回答:", response)
// println()
// ```
func WithEnableThinking(t any) AIConfigOption {
	return func(config *AIConfig) {
		if utils.IsNil(t) {
			return
		}
		switch t.(type) {
		case bool:
			config.EnableThinking = t.(bool)
			return
		}

		switch utils.InterfaceToString(t) {
		case "yes", "y", "true", "1", "enable", "on", "auto", "a", "enabled":
			config.EnableThinking = true
		default:
			config.EnableThinking = false
		}

		switch config.Type {
		case "volcengine":
			config.EnableThinkingField = "thinking"
			if config.EnableThinking {
				config.EnableThinkingValue = map[string]any{
					"type": "enabled",
				}
			} else {
				config.EnableThinkingValue = map[string]any{
					"type": "disabled",
				}
			}
		}
	}
}

func WithHost(h string) AIConfigOption {
	return func(c *AIConfig) {
		c.Host = h
	}
}

func WithPort(p int) AIConfigOption {
	return func(c *AIConfig) {
		c.Port = p
	}
}

func WithNoHTTPS(b bool) AIConfigOption {
	return func(c *AIConfig) {
		c.NoHttps = b
	}
}

func NewDefaultAIConfig(opts ...AIConfigOption) *AIConfig {
	c := &AIConfig{
		Timeout:                120,
		FunctionCallRetryTimes: 5,
		HTTPErrorHandler: func(err error) {
			log.Debugf("ai request failed: %s", err)
		},
	}
	// 加载Type参数
	for _, p := range opts {
		p(c)
	}

	// 加载默认参数
	if c.Type != "" {
		err := consts.GetThirdPartyApplicationConfig(c.Type, c)
		if err != nil {
			log.Debug(err)
		}
	}

	// 加载用户参数
	for _, p := range opts {
		p(c)
	}
	return c
}

type AIConfigOption func(*AIConfig)

func WithContext(ctx context.Context) AIConfigOption {
	return func(c *AIConfig) {
		c.Context = ctx
	}
}

// WithBaseURL 设置 AI 服务的基础 URL，用于自定义 API 端点或使用代理服务。
//
// 参数：
// - baseURL(string): API 基础 URL
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 使用自定义 API 地址
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxx"),
//	ai.baseURL("https://api.openai-proxy.com/v1"),
//
// )
// ```
func WithBaseURL(baseURL string) AIConfigOption {
	return func(c *AIConfig) {
		if baseURL != "" {
			c.BaseURL = baseURL
		}
	}
}

func WithStreamAndConfigHandler(h func(reader io.Reader, cfg *AIConfig)) AIConfigOption {
	return func(c *AIConfig) {
		c.StreamHandler = func(reader io.Reader) {
			h(reader, c)
		}
	}
}

// WithReasonStreamHandler 设置推理过程的流式输出回调，用于获取 AI 的思考过程。
//
// 参数：
// - h(func(io.Reader)): 推理流处理回调函数
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// response = ai.Chat(
//
//	"介绍一下 SQL 注入",
//	ai.onReasonStream(fn(reader) {
//	    io.ReadEvery1s(
//	        context.New(),
//	        reader,
//	        func(data) {
//	            println(string(data))
//	            return true
//	        },
//	    )
//	}),
//
// )~
// ```
func WithReasonStreamHandler(h func(io.Reader)) AIConfigOption {
	return func(c *AIConfig) {
		c.ReasonStreamHandler = h
	}
}

// WithStreamHandler 设置流式输出的回调函数，用于实时接收 AI 响应数据。
//
// 参数：
// - h(func(io.Reader)): 流式数据处理回调函数
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// response = ai.Chat(
//
//	"介绍一下 SQL 注入",
//	ai.onStream(fn(reader) {
//	    io.ReadEvery1s(
//	        context.New(),
//	        reader,
//	        func(data) {
//	            println(string(data))
//	            return true
//	        },
//	    )
//	}),
//
// )~
// ```
func WithStreamHandler(h func(io.Reader)) AIConfigOption {
	return func(c *AIConfig) {
		c.StreamHandler = h
	}
}

// WithDebugStream 启用流式输出调试模式，用于开发调试。
//
// 参数：
// - h(...bool): 是否启用调试（默认 true）
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 启用调试模式
// response = ai.Chat("测试消息", ai.debugStream(true))~
// // 输出内容将包含调试信息 TTFT(Time to First Token) first byte(token) delay in second
// println(response)
// ```
func WithDebugStream(h ...bool) AIConfigOption {
	return func(c *AIConfig) {
		if len(h) <= 0 || h[0] {
			c.StreamHandler = func(r io.Reader) {
				start := time.Now()
				reader := bufio.NewReader(r)
				_, err := reader.ReadByte()
				if err == nil {
					log.Infof("first byte(token) delay: %v", time.Since(start))
				}
				reader.UnreadByte()
				io.Copy(os.Stdout, reader)
			}
		} else {
			c.StreamHandler = nil
		}
	}
}

// WithDomain 设置服务域名，用于某些特定的 AI 服务提供商。
//
// 参数：
// - domain(string): 域名字符串
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxx"),
//	ai.domain("api.openai.com"),
//
// )
// ```
func WithDomain(domain string) AIConfigOption {
	return func(c *AIConfig) {
		c.Domain = domain
	}
}

// WithModel 指定要使用的 AI 模型名称。
//
// 参数：
// - model(string): 模型名称（如 "gpt-4"、"gpt-3.5-turbo"）
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 使用 GPT-4
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxx"),
//	ai.model("gpt-4"),
//
// )
//
// // 使用 GPT-3.5
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxx"),
//	ai.model("gpt-3.5-turbo"),
//
// )
// ```
func WithModel(model string) AIConfigOption {
	return func(c *AIConfig) {
		c.Model = model
	}
}

func WithChatImageContent(image ...any) AIConfigOption {
	return func(c *AIConfig) {
		for _, i := range image {
			switch v := i.(type) {
			case string:
				if utils.GetFirstExistedFile(v) != "" {
					log.Infof("add image_url.url with: %v", utils.ShrinkString(v, 200))
					WithImageFile(v)(c)
				} else if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
					log.Infof("add image_url.url with: %v", utils.ShrinkString(v, 200))
					c.Images = append(c.Images, &ImageDescription{
						Url: v,
					})
				} else if utils.MatchAllOfGlob(v, `data:image/*;base64*`) {
					log.Infof("add image_url.url with: %v", utils.ShrinkString(v, 200))
					c.Images = append(c.Images, &ImageDescription{
						Url: v,
					})
				} else {
					log.Warnf("invalid image: %s", v)
				}
			case *ImageDescription:
				if v.Url != "" {
					log.Infof("add image_url.url with: %v", utils.ShrinkString(v.Url, 200))
					c.Images = append(c.Images, v)
				} else {
					log.Warnf("invalid image %v", v)
				}
			case *ChatContent:
				if v.Type == "image_url" {
					log.Infof("add image_url.url with: %v", utils.ShrinkString(v.ImageUrl, 200))
					c.Images = append(c.Images, &ImageDescription{
						Url: utils.MapGetString(utils.InterfaceToGeneralMap(v.ImageUrl), "url"),
					})
				} else {
					log.Warnf("invalid chat content image: %v", v)
				}
			case ChatContent:
				if v.Type == "image_url" {
					c.Images = append(c.Images, &ImageDescription{
						Url: utils.MapGetString(utils.InterfaceToGeneralMap(v.ImageUrl), "url"),
					})
				} else {
					log.Warnf("invalid chat content image: %v", v)
				}
			default:
				log.Warnf("unsupported image type: %T, value: %v", i, i)
			}
		}

	}
}

// WithType 指定 AI 服务提供商类型。
//
// 参数：
// - t(string): 服务类型（如 "openai"、"chatglm"、"moonshot" 等）
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 使用 OpenAI
// response, err = ai.Chat("你好",
//
//	ai.apiKey("sk-xxx"),
//	ai.type("openai"),
//
// )
//
// // 使用 ChatGLM
// response, err = ai.Chat("你好",
//
//	ai.apiKey("your-key"),
//	ai.type("chatglm"),
//
// )
// ```
func WithType(t string) AIConfigOption {
	return func(config *AIConfig) {
		config.Type = t
	}
}

// WithTimeout 设置请求超时时间（单位：秒）。
//
// 参数：
// - timeout(float64): 超时时间（秒）
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 设置 60 秒超时
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxx"),
//	ai.timeout(60),
//
// )
//
// // 长时间任务设置更长超时
// response = ai.Chat(
//
//	"生成一个完整的渗透测试报告",
//	ai.apiKey("sk-xxx"),
//	ai.timeout(180),  // 3 分钟
//
// )~
// ```
func WithTimeout(timeout float64) AIConfigOption {
	return func(c *AIConfig) {
		c.Timeout = timeout
	}
}

// WithProxy 设置 HTTP 代理服务器，用于网络请求。
//
// 参数：
// - p(string): 代理服务器地址（支持 http/https/socks5）
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // HTTP 代理
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxx"),
//	ai.proxy("http://127.0.0.1:7890"),
//
// )
//
// // SOCKS5 代理
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxx"),
//	ai.proxy("socks5://127.0.0.1:1080"),
//
// )
// ```
func WithProxy(p string) AIConfigOption {
	return func(c *AIConfig) {
		c.Proxy = p
	}
}

// WithAPIKey 设置 AI 服务的 API 密钥，这是访问 AI 服务的必需凭证。
//
// 参数：
// - k(string): API 密钥字符串
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxxxxxxxxxxxxxxx"),
//
// )
// ```
func WithAPIKey(k string) AIConfigOption {
	return func(c *AIConfig) {
		c.APIKey = strings.TrimSpace(k)
	}
}

// WithImageFile 传入图片文件路径，自动读取并发送给 AI 进行分析。
//
// 参数：
// - i(string): 图片文件路径
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 分析本地图片
// response = ai.Chat(
//
//	"这张图片中有什么漏洞特征？",
//	ai.imageFile("/path/to/screenshot.png"),
//
// )~
// println(response)
//
// // 分析验证码
// response = ai.Chat(
//
//	   "识别这个验证码",
//		ai.imageFile("./captcha.jpg"),
//
// )~
// println(response)
// ```
func WithImageFile(i string) AIConfigOption {
	return func(config *AIConfig) {
		if utils.GetFirstExistedFile(i) == "" {
			log.Warnf("file: %v is not existed", i)
			return
		}

		data, err := os.ReadFile(i)
		if err != nil {
			log.Warnf("file: %v read error: %v", i, err)
			return
		}

		name, err := filetype.Image(data)
		if err != nil {
			log.Warnf("file: %v is not image: %v", i, err)
			return
		}

		var buf bytes.Buffer
		buf.WriteString("data:")
		buf.WriteString(name.MIME.Value)
		buf.WriteString(";")
		buf.WriteString("base64,")
		buf.WriteString(codec.EncodeBase64(data))
		config.Images = append(config.Images, &ImageDescription{
			Url: buf.String(),
		})
	}
}

// WithImageBase64 传入 Base64 编码的图片数据，用于图像识别场景。
//
// 参数：
// - b64(string): 传入 Base64 编码的图片数据，支持纯 base64 字符串或data:image/ 格式。
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 从网络获取图片并分析
// rsp, req, err = poc.Get("https://help-static-aliyun-doc.aliyuncs.com/assets/img/zh-CN/5929730761/p529185.jpg")
//
//	if err != nil{
//	   die(err)
//	}
//
// imageBytes = rsp.GetBody()
// dump(imageBytes)
// b64ImageBytes = codec.EncodeBase64(imageBytes)
//
// response, err = ai.Chat("分析这张图片", ai.imageBase64(b64ImageBytes))
//
//	if err != nil{
//	   die(err)
//	}
//
// println(response)
// ```
func WithImageBase64(b64 string) AIConfigOption {
	return func(config *AIConfig) {
		if strings.HasPrefix(b64, "data:image/") {
			for img := range imageutils.ExtractImage(b64) {
				b64 = img.Base64()
			}
		}

		raw, err := codec.DecodeBase64(b64)
		if err != nil {
			log.Warnf("decode error: %v", err)
			return
		}
		name, err := filetype.Image(raw)
		if err != nil {
			log.Warnf("input is not image: %v", err)
			return
		}

		var buf bytes.Buffer
		buf.WriteString("data:")
		buf.WriteString(name.MIME.Value)
		buf.WriteString(";")
		buf.WriteString("base64,")
		buf.WriteString(b64)
		config.Images = append(config.Images, &ImageDescription{
			Url: buf.String(),
		})
	}
}

// WithImageRaw 传入原始图片字节数据，用于图像识别场景。
//
// 参数：
// - raw([]byte): 图片的原始字节数据
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 从网络获取图片并分析
// rsp, req, err = poc.Get("https://help-static-aliyun-doc.aliyuncs.com/assets/img/zh-CN/5929730761/p529185.jpg")
//
//	if err != nil{
//	   die(err)
//	}
//
// imageBytes = rsp.GetBody()
// dump(imageBytes)
//
// response, err = ai.Chat("分析这张图片", ai.imageRaw(imageBytes))
//
//	if err != nil{
//	   die(err)
//	}
//
// println(response)
// ```
func WithImageRaw(raw []byte) AIConfigOption {
	return func(config *AIConfig) {
		name, err := filetype.Image(raw)
		if err != nil {
			log.Warnf("input is not image: %v", err)
			return
		}

		var buf bytes.Buffer
		buf.WriteString("data:")
		buf.WriteString(name.MIME.Value)
		buf.WriteString(";")
		buf.WriteString("base64,")
		buf.WriteString(codec.EncodeBase64(raw))
		config.Images = append(config.Images, &ImageDescription{
			Url: buf.String(),
		})
	}
}

// WithNoHttps 禁用 HTTPS，使用 HTTP 协议进行通信。
//
// 参数：
// - b(bool): true 表示禁用 HTTPS
//
// 返回值：
// - r1(AIConfigOption): AI 配置选项
//
// Example:
// ```go
// // 本地测试环境使用 HTTP
// client = ai.OpenAI(
//
//	ai.apiKey("test-key"),
//	ai.baseURL("localhost:8080"),
//	ai.noHttps(true),
//
// )
// ```
func WithNoHttps(b bool) AIConfigOption {
	return func(c *AIConfig) {
		c.NoHttps = b
	}
}

// WithFunctionCallRetryTimes 设置函数调用失败时的重试次数。
//
// 参数：
// - times(int): 重试次数
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
//
// Example:
// ```go
//
//	funcs = {
//	   "searchVulnerability": func(keyword) {
//	      return {"result": sprintf("搜索漏洞: %s", keyword)}
//	    },
//	   "scanTarget": func(target, port) {
//	     return {"target": target, "port": port, "status": "scanning"}
//	  },
//	}
//
// // 设置重试 3 次
// result = ai.FunctionCall(
//
//	"扫描目标 127.0.0.1:8080",
//	funcs,
//	ai.funcCallRetryTimes(3),
//
// )~
// dump(result)
// ```
func WithFunctionCallRetryTimes(times int) AIConfigOption {
	return func(c *AIConfig) {
		c.FunctionCallRetryTimes = times
	}
}

func WithHTTPErrorHandler(h func(error)) AIConfigOption {
	return func(c *AIConfig) {
		c.HTTPErrorHandler = h
	}
}

// WithToolCallCallback 设置 tool_calls 回调函数，用于在 AI 响应中包含 tool_calls 时接管其处理逻辑。启用后，tool_calls 将不再以默认的占位标记形式输出，而是直接通过回调函数返回解析后的 ToolCall 对象。 目前只在项目内部测试脚本使用
// 参数：
// - cb(func([]*ToolCall)): 当 AI 响应中包含 tool_calls 时触发的回调函数
//
// 返回值：
// - r1(aispec.AIConfigOption): AI 配置选项
func WithToolCallCallback(cb func([]*ToolCall)) AIConfigOption {
	return func(c *AIConfig) {
		c.ToolCallCallback = cb
	}
}

// WithTools sets the available tools that the model may call
func WithTools(tools []Tool) AIConfigOption {
	return func(c *AIConfig) {
		c.Tools = tools
	}
}

// WithToolChoice controls which (if any) tool is called by the model
// Can be "none", "auto", "required", or a specific function: {"type": "function", "function": {"name": "my_function"}}
func WithToolChoice(choice any) AIConfigOption {
	return func(c *AIConfig) {
		c.ToolChoice = choice
	}
}
