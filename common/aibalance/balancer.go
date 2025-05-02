package aibalance

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

// 用于确保健康检查调度器只启动一次
var healthCheckSchedulerStarted sync.Once

type Balancer struct {
	config   *ServerConfig
	listener net.Listener
	mutex    sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewBalancerFromRawConfig(raw []byte, files ...string) (*Balancer, error) {
	var configFile = "[::memory::]"
	if len(files) > 0 {
		configFile = files[0]
	}
	var ymlConfig YamlConfig
	if err := yaml.Unmarshal(raw, &ymlConfig); err != nil {
		return nil, utils.Wrapf(err, "Unmarshal config file %s error", configFile)
	}

	serverConfig, err := ymlConfig.ToServerConfig()
	if err != nil {
		return nil, utils.Wrapf(err, "cannot convert yaml config file to server %s", configFile)
	}

	// 从数据库加载恢复 providers
	if err := LoadProvidersFromDatabase(serverConfig); err != nil {
		log.Warnf("Failed to load providers from database: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &Balancer{
		config: serverConfig,
		ctx:    ctx,
		cancel: cancel,
	}

	// 启动健康检查调度器 (确保只启动一次)
	healthCheckSchedulerStarted.Do(func() {
		StartHealthCheckScheduler(b, 5*time.Minute) // 修改为5分钟检查一次
	})

	return b, nil
}

// NewBalancer 创建一个新的平衡器实例，如果无法读取配置文件，将会创建一个默认配置并从数据库加载
func NewBalancer(configFile string) (*Balancer, error) {
	// 尝试读取配置文件
	raw, err := os.ReadFile(configFile)
	if err != nil {
		// 如果配置文件不存在，创建一个基本的服务器配置
		log.Warnf("Failed to read config file %s: %v, using default configuration and loading from database", configFile, err)

		// 创建默认配置
		serverConfig := NewServerConfig()

		// 从数据库加载恢复 providers
		if err := LoadProvidersFromDatabase(serverConfig); err != nil {
			log.Warnf("Failed to load providers from database: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		b := &Balancer{
			config: serverConfig,
			ctx:    ctx,
			cancel: cancel,
		}

		// 启动健康检查调度器 (确保只启动一次)
		healthCheckSchedulerStarted.Do(func() {
			StartHealthCheckScheduler(b, 5*time.Minute) // 修改为5分钟检查一次
		})

		return b, nil
	}

	// 如果配置文件存在，正常创建
	b, err := NewBalancerFromRawConfig(raw, configFile)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// LoadProvidersFromDatabase 从数据库加载所有提供者和API密钥
func LoadProvidersFromDatabase(config *ServerConfig) error {
	log.Infof("Starting to load AI providers and API keys from database")

	// 1. 加载 AI 提供者
	// 获取数据库中所有的提供者
	dbProviders, err := GetAllAiProviders()
	if err != nil {
		return utils.Errorf("Failed to get AI providers from database: %v", err)
	}

	log.Infof("Retrieved %d AI providers from database", len(dbProviders))

	// 按照 WrapperName 分组提供者
	modelProviders := make(map[string][]*Provider)

	for _, dbProvider := range dbProviders {
		// 跳过无效的提供者
		if dbProvider.TypeName == "" || dbProvider.ModelName == "" {
			log.Warnf("Skipping invalid provider: TypeName=%s, ModelName=%s", dbProvider.TypeName, dbProvider.ModelName)
			continue
		}

		// 创建 Provider 实例
		provider := &Provider{
			ModelName:   dbProvider.ModelName,
			TypeName:    dbProvider.TypeName,
			DomainOrURL: dbProvider.DomainOrURL,
			APIKey:      dbProvider.APIKey,
			NoHTTPS:     dbProvider.NoHTTPS,
			DbProvider:  dbProvider, // 直接设置数据库对象
		}

		// 使用 WrapperName 作为模型名称来分组
		modelName := dbProvider.WrapperName
		if modelName == "" {
			modelName = dbProvider.ModelName // 如果 WrapperName 为空，使用 ModelName
		}

		modelProviders[modelName] = append(modelProviders[modelName], provider)
	}

	// 将提供者添加到配置中
	for modelName, providers := range modelProviders {
		if len(providers) > 0 {
			log.Infof("Adding %d providers for model %s", len(providers), modelName)

			// 添加到 Models
			config.Models.models[modelName] = providers

			// 添加到 Entrypoints
			config.Entrypoints.providers[modelName] = providers

			// 打印提供者信息
			for i, p := range providers {
				log.Infof("  Provider %d: TypeName=%s, ModelName=%s, Domain=%s, HealthStatus=%v",
					i, p.TypeName, p.ModelName, p.DomainOrURL, p.DbProvider.IsHealthy)
			}
		}
	}

	log.Infof("Database AI providers loaded, added %d models in total", len(modelProviders))

	// 2. 加载 API 密钥
	log.Infof("Starting to load API keys from database")
	apiKeys, err := GetAllAiApiKeys()
	if err != nil {
		log.Warnf("Failed to load API keys from database: %v", err)
	} else {
		log.Infof("Retrieved %d API keys from database", len(apiKeys))
		for _, key := range apiKeys {
			// 解析允许的模型列表
			modelNames := strings.Split(key.AllowedModels, ",")
			modelMap := make(map[string]bool)
			for _, model := range modelNames {
				if model = strings.TrimSpace(model); model != "" {
					modelMap[model] = true
				}
			}

			// 添加到内存配置
			config.KeyAllowedModels.allowedModels[key.APIKey] = modelMap
			log.Infof("  API Key: %s, Allowed Models: %s", utils.ShrinkString(key.APIKey, 8), key.AllowedModels)
		}
		log.Infof("API keys loaded successfully")
	}

	return nil
}

func (b *Balancer) RunWithPort(port int) error {
	if port <= 0 {
		return utils.Errorf("invalid port %d", port)
	}
	return b.run(utils.HostPort("0.0.0.0", port))
}

func (b *Balancer) RunWithAddr(addr string) error {
	if addr == "" {
		return utils.Errorf("invalid address %s", addr)
	}
	return b.run(addr)
}

func (b *Balancer) Run() error {
	return b.run(utils.HostPort("127.0.0.1", 80))
}

func (b *Balancer) run(addr string) error {
	b.mutex.Lock()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		b.mutex.Unlock()
		return err
	}
	b.listener = lis
	b.mutex.Unlock()

	go func() {
		<-b.ctx.Done()
		log.Infof("Balancer context is done, closing listener...")
		if b.listener != nil {
			b.listener.Close()
		}
	}()

	for {
		conn, err := lis.Accept()
		if err != nil {
			if b.ctx.Err() != nil {
				return nil // 正常关闭
			}
			return err
		}
		go func() {
			defer conn.Close()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("Panic recovered: %v", utils.ErrorStack(err))
				}
			}()
			b.config.Serve(conn)
		}()
	}
}

// Close 关闭 balancer 并释放资源
func (b *Balancer) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.cancel != nil {
		b.cancel()
	}

	if b.listener != nil {
		return b.listener.Close()
	}

	return nil
}

// GetProviders 获取所有提供者
func (b *Balancer) GetProviders() []*Provider {
	var providers []*Provider

	// 从所有模型中获取提供者
	for _, modelProviders := range b.config.Models.models {
		providers = append(providers, modelProviders...)
	}

	return providers
}
