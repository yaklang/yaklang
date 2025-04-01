package ai

import (
	"errors"
	"io"
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
	mu           sync.Mutex                                                               // 互斥锁，保护并发访问
	clientConfig []*Gateway                                                               // 客户端配置列表
	callback     func(typeName string, modelName string, isReason bool, reader io.Reader) // 回调函数，处理聊天响应
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

	client := createAIGateway(typeName)
	if client == nil {
		log.Warnf("create ai client by type %s failed", typeName)
		return errors.New("create ai client by type " + typeName + " failed")
	}
	opts = append(opts, aispec.WithAPIKey(apikey), aispec.WithModel(modelName))
	opts = append(opts, aispec.WithStreamHandler(func(reader io.Reader) {
		pr, pw := utils.NewPipe()
		go io.Copy(pw, reader)
		b.emitCallback(typeName, modelName, false, pr)
	}), aispec.WithReasonStreamHandler(func(reader io.Reader) {
		pr, pw := utils.NewPipe()
		go io.Copy(pw, reader)
		b.emitCallback(typeName, modelName, true, pr)
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

// Chat 使用第一个成功的客户端进行聊天
func (b *BatchChatter) Chat(msg string) (*BatchChatResult, error) {
	for _, client := range b.clientConfig {
		response, err := client.Chat(msg)
		if err != nil {
			log.Errorf("chat with ai client failed: %s", err)
			continue
		}
		return &BatchChatResult{
			Result:    response,
			TypeName:  client.GetTypeName(),
			ModelName: client.GetModelName(),
		}, nil
	}
	return nil, errors.New("all ai clients failed")
}

// ChatParallel 并行使用所有客户端进行聊天
func (b *BatchChatter) ChatParallel(msg string) ([]*BatchChatResult, error) {
	wg := sync.WaitGroup{}
	mu := sync.Mutex{}
	results := make([]*BatchChatResult, 0, len(b.clientConfig))

	for _, client := range b.clientConfig {
		wg.Add(1)
		go func(client *Gateway) {
			defer wg.Done()
			response, err := client.Chat(msg)
			if err != nil {
				log.Errorf("chat with ai client failed: %s", err)
				return
			}
			mu.Lock()
			results = append(results, &BatchChatResult{
				Result:    response,
				TypeName:  client.GetTypeName(),
				ModelName: client.GetModelName(),
			})
			mu.Unlock()
		}(client)
	}

	wg.Wait()

	for _, result := range results {
		if result != nil {
			return results, nil
		}
	}
	return nil, errors.New("all ai clients failed")
}
