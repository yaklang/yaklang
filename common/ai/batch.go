package ai

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"os"
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
	clientConfigRWMutex sync.RWMutex
	invalidClientMutex  sync.Mutex

	clientConfig  []*Gateway                                                               // 客户端配置列表
	callback      func(typeName string, modelName string, isReason bool, reader io.Reader) // 回调函数，处理聊天响应
	invalidClient map[*Gateway]struct{}
	retryTimes    int  // 重试次数，默认为3
	debug         bool // 是否开启调试模式
}

func NewBatchChatter() *BatchChatter {
	return &BatchChatter{
		clientConfig:  make([]*Gateway, 0),
		callback:      nil,
		invalidClient: make(map[*Gateway]struct{}),
		retryTimes:    3, // 默认重试3次
		debug:         false,
	}
}

// Size 返回当前客户端配置的数量
func (b *BatchChatter) Size() int {
	b.clientConfigRWMutex.RLock()
	defer b.clientConfigRWMutex.RUnlock()
	return len(b.clientConfig)
}

// PushChatClient 添加一个聊天客户端到批量聊天器
func (b *BatchChatter) PushChatClient(client *Gateway) {
	b.clientConfigRWMutex.Lock()
	defer b.clientConfigRWMutex.Unlock()
	b.clientConfig = append(b.clientConfig, client)
}

// SetCallback 设置回调函数，用于处理聊天响应
func (b *BatchChatter) SetCallback(callback func(typeName string, modelName string, isReason bool, reader io.Reader)) {
	b.callback = callback
}

// SetDebug 设置调试模式，开启后会将所有回调输出转发到 stdout
func (b *BatchChatter) SetDebug(debug bool) {
	b.clientConfigRWMutex.Lock()
	defer b.clientConfigRWMutex.Unlock()
	b.debug = debug
}

// emitCallback 触发回调函数
func (b *BatchChatter) emitCallback(typeName string, modelName string, isReason bool, reader io.Reader) {
	// 如果开启了调试模式，将输出转发到 stdout
	if b.debug {
		pr, pw := utils.NewPipe()
		go func(rawReader io.Reader) {
			defer func() {
				pw.Close()
			}()
			prefix := "AI Response"
			if isReason {
				prefix = "AI Reason"
			}
			log.Infof("[%s] %s - %s", prefix, typeName, modelName)
			io.Copy(os.Stdout, io.TeeReader(rawReader, pw))
			log.Infof("--- End of %s ---", prefix)
		}(reader)
		reader = pr
	} else {
		log.Infof("callback not set, skip emit callback for %v:%v (reason:%v)", typeName, modelName, isReason)
	}

	if b.callback != nil {
		b.callback(typeName, modelName, isReason, reader)
	}
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
	b.clientConfigRWMutex.Lock()
	defer b.clientConfigRWMutex.Unlock()
	if times <= 0 {
		times = 1
	}
	b.retryTimes = times
}

// ChatWithRandomClient 使用随机一个客户端进行聊天
func (b *BatchChatter) ChatWithRandomClient(msg string) (*BatchChatResult, error) {
	b.clientConfigRWMutex.RLock()
	// 如果没有可用的客户端，直接返回错误
	if len(b.clientConfig) == 0 {
		b.clientConfigRWMutex.RUnlock()
		return nil, errors.New("no available ai clients")
	}

	// 创建一个可用客户端的列表
	availableClients := make([]*Gateway, 0)
	for _, client := range b.clientConfig {
		b.invalidClientMutex.Lock()
		if _, ok := b.invalidClient[client]; !ok {
			availableClients = append(availableClients, client)
		}
		b.invalidClientMutex.Unlock()
	}
	b.clientConfigRWMutex.RUnlock()

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
		b.invalidClientMutex.Lock()
		b.invalidClient[client] = struct{}{}
		b.invalidClientMutex.Unlock()
	}

	return nil, errors.New("all ai clients failed")
}

// Chat 使用第一个成功的客户端进行聊天
func (b *BatchChatter) Chat(msg string) (*BatchChatResult, error) {
	b.clientConfigRWMutex.RLock()
	clients := make([]*Gateway, len(b.clientConfig))
	copy(clients, b.clientConfig)
	b.clientConfigRWMutex.RUnlock()

	for _, basicClient := range clients {
		b.invalidClientMutex.Lock()
		_, ok := b.invalidClient[basicClient]
		b.invalidClientMutex.Unlock()
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
		b.invalidClientMutex.Lock()
		b.invalidClient[basicClient] = struct{}{}
		b.invalidClientMutex.Unlock()
	}
	return nil, errors.New("all ai clients failed")
}

// ChatParallel 并行使用所有客户端进行聊天
func (b *BatchChatter) ChatParallel(msg string) ([]*BatchChatResult, error) {
	b.clientConfigRWMutex.RLock()
	clients := make([]*Gateway, len(b.clientConfig))
	copy(clients, b.clientConfig)
	b.clientConfigRWMutex.RUnlock()

	wg := sync.WaitGroup{}
	mu := sync.Mutex{}
	results := make([]*BatchChatResult, 0, len(clients))

	for _, rawClient := range clients {
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

// ChatParallelDifferentModel 并行使用不同模型的客户端进行聊天，返回所有成功的结果
func (b *BatchChatter) ChatParallelDifferentModel(msg string) ([]*BatchChatResult, error) {
	b.clientConfigRWMutex.RLock()
	// 如果没有可用的客户端，直接返回错误
	if len(b.clientConfig) == 0 {
		b.clientConfigRWMutex.RUnlock()
		return nil, errors.New("no available ai clients")
	}

	// 按模型名称分组客户端
	modelGroups := make(map[string][]*Gateway)
	for _, client := range b.clientConfig {
		b.invalidClientMutex.Lock()
		if _, ok := b.invalidClient[client]; !ok {
			modelName := client.GetModelName()
			modelGroups[modelName] = append(modelGroups[modelName], client)
		}
		b.invalidClientMutex.Unlock()
	}
	b.clientConfigRWMutex.RUnlock()

	// 如果没有有效的客户端，返回错误
	if len(modelGroups) == 0 {
		return nil, errors.New("all ai clients are invalid")
	}

	// 为每个模型创建一个 goroutine
	wg := sync.WaitGroup{}
	mu := sync.Mutex{}
	results := make([]*BatchChatResult, 0)

	// 对每个模型组，随机选择一个客户端进行尝试
	for modelName, clients := range modelGroups {
		wg.Add(1)
		go func(modelName string, modelClients []*Gateway) {
			defer wg.Done()

			// 随机打乱该模型的客户端顺序
			rand.Shuffle(len(modelClients), func(i, j int) {
				modelClients[i], modelClients[j] = modelClients[j], modelClients[i]
			})

			// 尝试该模型的客户端
			for _, client := range modelClients {
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
					mu.Lock()
					results = append(results, &BatchChatResult{
						Result:    response,
						TypeName:  client.GetTypeName(),
						ModelName: client.GetModelName(),
					})
					mu.Unlock()
					return // 成功后就返回
				}

				// 重试失败后标记为无效
				log.Infof("retry %d times for %v: %v, mark invalid", b.retryTimes, client.GetTypeName(), client.GetModelName())
				b.invalidClientMutex.Lock()
				b.invalidClient[client] = struct{}{}
				b.invalidClientMutex.Unlock()
			}
		}(modelName, clients)
	}

	wg.Wait()

	// 如果没有成功的结果，返回错误
	if len(results) == 0 {
		return nil, errors.New("all ai clients failed")
	}

	return results, nil
}
