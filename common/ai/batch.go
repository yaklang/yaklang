package ai

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ChatClientDescription 描述了聊天客户端的配置信息
type ChatClientDescription struct {
	TypeName string                  // AI 类型名称
	APIKey   string                  // API 密钥
	Opts     []aispec.AIConfigOption // 配置选项
}

// BatchChatter 批量聊天器，用于管理多个聊天客户端
type BatchChatter struct {
	mu            sync.Mutex                                                               // 互斥锁，保护并发访问
	clientConfig  []*Gateway                                                               // 客户端配置列表
	callback      func(typeName string, modelName string, isReason bool, reader io.Reader) // 回调函数，处理聊天响应
	invalidClient map[*Gateway]struct{}
	retryTimes    int // 重试次数，默认为3
}

func NewBatchChatter() *BatchChatter {
	return &BatchChatter{
		clientConfig:  make([]*Gateway, 0),
		callback:      nil,
		invalidClient: make(map[*Gateway]struct{}),
		retryTimes:    3, // 默认重试3次
	}
}

// PushChatClient 添加一个聊天客户端到批量聊天器
func (b *BatchChatter) PushChatClient(client *Gateway) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clientConfig = append(b.clientConfig, client)
}

// SetCallback 设置回调函数，用于处理聊天响应
func (b *BatchChatter) SetCallback(callback func(typeName string, modelName string, isReason bool, reader io.Reader)) {
	b.callback = callback
}

// emitCallback 触发回调函数
func (b *BatchChatter) emitCallback(typeName string, modelName string, isReason bool, reader io.Reader) {
	if b.callback != nil {
		b.callback(typeName, modelName, isReason, reader)
		return
	}
	log.Infof("callback not set, skip emit callback")
}

// AddChatClient 添加一个聊天客户端，指定类型、API密钥和模型名称
func (b *BatchChatter) AddChatClient(typeName string, apikey string, modelName string, opts ...aispec.AIConfigOption) error {
	aic := &aispec.AIConfig{}
	for _, opt := range opts {
		opt(aic)
	}
	if aic.ReasonStreamHandler != nil || aic.StreamHandler != nil {
		log.Warnf("reason stream handler or stream handler should not be set in AddChatClient")
		return errors.New("reason stream handler or stream handler should not be set in AddChatClient")
	}
	_ = aic

	client := createAIGateway(typeName)
	if client == nil {
		log.Warnf("create ai client by type %s failed", typeName)
		return errors.New("create ai client by type " + typeName + " failed")
	}
	opts = append(opts, aispec.WithAPIKey(apikey), aispec.WithModel(modelName))
	opts = append(opts, aispec.WithStreamHandler(func(reader io.Reader) {
		var prefetch = make([]byte, 1)
		n, _ := io.ReadFull(reader, prefetch)
		if n > 0 {
			pr, pw := utils.NewPipe()
			go func() {
				defer pw.Close()
				io.Copy(pw, io.MultiReader(io.MultiReader(bytes.NewReader(prefetch), reader)))
			}()
			b.emitCallback(typeName, modelName, false, pr)
		}
	}), aispec.WithReasonStreamHandler(func(reader io.Reader) {
		var prefetch = make([]byte, 1)
		n, _ := io.ReadFull(reader, prefetch)
		if n > 0 {
			pr, pw := utils.NewPipe()
			go func() {
				defer pw.Close()
				io.Copy(pw, io.MultiReader(io.MultiReader(bytes.NewReader(prefetch), reader)))
			}()
			b.emitCallback(typeName, modelName, true, pr)
		}
	}))
	client.LoadOption(opts...)
	b.PushChatClient(&Gateway{
		Config: &aispec.AIConfig{
			Type:  typeName,
			Model: modelName,
		},
		AIClient: client,
	})
	return nil
}

// AddChatClientWithManyAPIKeys 使用多个API密钥添加聊天客户端
func (b *BatchChatter) AddChatClientWithManyAPIKeys(typeName string, apiKeys []string, modelName string, opts ...aispec.AIConfigOption) error {
	for _, apiKey := range apiKeys {
		err := b.AddChatClient(typeName, apiKey, modelName, opts...)
		if err != nil {
			return err
		}
	}
	return nil
}

// AddChatClientWithModels 使用多个模型添加聊天客户端
func (b *BatchChatter) AddChatClientWithModels(typeName string, apiKey string, modelNames []string, opts ...aispec.AIConfigOption) error {
	for _, modelName := range modelNames {
		err := b.AddChatClient(typeName, apiKey, modelName, opts...)
		if err != nil {
			return err
		}
	}
	return nil
}

// AddChatClientWithManyAPIKeysAndModels 使用多个API密钥和多个模型添加聊天客户端
func (b *BatchChatter) AddChatClientWithManyAPIKeysAndModels(typeName string, apiKeys []string, modelNames []string, opts ...aispec.AIConfigOption) error {
	for _, apiKey := range apiKeys {
		err := b.AddChatClientWithModels(typeName, apiKey, modelNames, opts...)
		if err != nil {
			return err
		}
	}
	return nil
}

// BatchChatResult 批量聊天结果
type BatchChatResult struct {
	Result    string // 聊天结果
	TypeName  string // AI 类型名称
	ModelName string // 模型名称
}

// SetRetryTimes 设置重试次数，如果设置为0或负数，则默认为1
func (b *BatchChatter) SetRetryTimes(times int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if times <= 0 {
		times = 1
	}
	b.retryTimes = times
}

// ChatWithRandomClient 使用随机一个客户端进行聊天
func (b *BatchChatter) ChatWithRandomClient(msg string) (*BatchChatResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 如果没有可用的客户端，直接返回错误
	if len(b.clientConfig) == 0 {
		return nil, errors.New("no available ai clients")
	}

	// 创建一个可用客户端的列表
	availableClients := make([]*Gateway, 0)
	for _, client := range b.clientConfig {
		if _, ok := b.invalidClient[client]; !ok {
			availableClients = append(availableClients, client)
		}
	}

	// 如果没有有效的客户端，返回错误
	if len(availableClients) == 0 {
		return nil, errors.New("all ai clients are invalid")
	}

	// 随机打乱客户端顺序
	rand.Shuffle(len(availableClients), func(i, j int) {
		availableClients[i], availableClients[j] = availableClients[j], availableClients[i]
	})

	// 尝试每个客户端
	for _, client := range availableClients {
		for i := 0; i < b.retryTimes; i++ {
			response, err := client.AIClient.Chat(msg)
			if err != nil {
				if utils.IsErrorNetOpTimeout(err) {
					log.Infof("met timeout error: %v, retry idx: %v", err, i+1)
					continue
				}
				log.Errorf("chat with ai client[%s] failed: %s", client.GetTypeName(), err)
				break
			}
			return &BatchChatResult{
				Result:    response,
				TypeName:  client.GetTypeName(),
				ModelName: client.GetModelName(),
			}, nil
		}

		// 重试失败后标记为无效
		log.Infof("retry %d times for %v: %v, mark invalid", b.retryTimes, client.GetTypeName(), client.GetModelName())
		b.invalidClient[client] = struct{}{}
	}

	return nil, errors.New("all ai clients failed")
}

// Chat 使用第一个成功的客户端进行聊天
func (b *BatchChatter) Chat(msg string) (*BatchChatResult, error) {
	for _, basicClient := range b.clientConfig {
		_, ok := b.invalidClient[basicClient]
		if ok {
			continue
		}

		for i := 0; i < b.retryTimes; i++ {
			response, err := basicClient.AIClient.Chat(msg)
			if err != nil {
				if utils.IsErrorNetOpTimeout(err) {
					log.Infof("met timeout error: %v, retry idx: %v", err, i+1)
					continue
				}
				log.Errorf("chat with ai client[%s] failed: %s", basicClient.GetTypeName(), err)
				break // next
			}
			return &BatchChatResult{
				Result:    response,
				TypeName:  basicClient.GetTypeName(),
				ModelName: basicClient.GetModelName(),
			}, nil
		}

		// retry failed
		log.Infof("retry %d times for %v: %v, mark invalid", b.retryTimes, basicClient.GetTypeName(), basicClient.GetModelName())
		b.invalidClient[basicClient] = struct{}{}
	}
	return nil, errors.New("all ai clients failed")
}

// ChatParallel 并行使用所有客户端进行聊天
func (b *BatchChatter) ChatParallel(msg string) ([]*BatchChatResult, error) {
	wg := sync.WaitGroup{}
	mu := sync.Mutex{}
	results := make([]*BatchChatResult, 0, len(b.clientConfig))

	for _, rawClient := range b.clientConfig {
		wg.Add(1)
		go func(basicClient *Gateway) {
			defer wg.Done()
			response, err := basicClient.AIClient.Chat(msg)
			if err != nil {
				log.Errorf("chat with ai client failed: %s", err)
				return
			}
			mu.Lock()
			results = append(results, &BatchChatResult{
				Result:    response,
				TypeName:  basicClient.GetTypeName(),
				ModelName: basicClient.GetModelName(),
			})
			mu.Unlock()
		}(rawClient)
	}

	wg.Wait()

	for _, result := range results {
		if result != nil {
			return results, nil
		}
	}
	return nil, errors.New("all ai clients failed")
}
