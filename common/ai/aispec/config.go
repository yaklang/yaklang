package aispec

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

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
	Model    string  `json:"model" app:"name:model,verbose:模型名称,id:2,type:list"`
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
			log.Errorf("ai request failed: %s", err)
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

func WithReasonStreamHandler(h func(io.Reader)) AIConfigOption {
	return func(c *AIConfig) {
		c.ReasonStreamHandler = h
	}
}

func WithStreamHandler(h func(io.Reader)) AIConfigOption {
	return func(c *AIConfig) {
		c.StreamHandler = h
	}
}

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

func WithDomain(domain string) AIConfigOption {
	return func(c *AIConfig) {
		c.Domain = domain
	}
}

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
					log.Warnf("invalid image description: %v", v)
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

func WithType(t string) AIConfigOption {
	return func(config *AIConfig) {
		config.Type = t
	}
}

func WithTimeout(timeout float64) AIConfigOption {
	return func(c *AIConfig) {
		c.Timeout = timeout
	}
}

func WithProxy(p string) AIConfigOption {
	return func(c *AIConfig) {
		c.Proxy = p
	}
}

func WithAPIKey(k string) AIConfigOption {
	return func(c *AIConfig) {
		c.APIKey = strings.TrimSpace(k)
	}
}

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

func WithNoHttps(b bool) AIConfigOption {
	return func(c *AIConfig) {
		c.NoHttps = b
	}
}

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
