package aibalance

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// HealthCheckResult stores health check results
type HealthCheckResult struct {
	Provider     *schema.AiProvider
	IsHealthy    bool
	ResponseTime int64 // milliseconds
	Error        error
}

// Use a simple ping message for health check
const healthCheckPrompt = "Ping. Please respond with 'Pong'."

// HealthCheckManager manages provider health checks
type HealthCheckManager struct {
	Balancer      *Balancer                  // Load balancer
	checkInterval time.Duration              // Health check interval
	checkResults  map[int]*HealthCheckResult // Store latest health check results
	lastCheckTime map[uint]time.Time         // Store last check time to avoid frequent checks
	mutex         sync.RWMutex               // Mutex to protect checkResults and lastCheckTime
	stopChan      chan struct{}              // Signal to stop health checks
}

// NewHealthCheckManager creates a new health check manager
func NewHealthCheckManager(balancer *Balancer) *HealthCheckManager {
	return &HealthCheckManager{
		Balancer:      balancer,
		checkInterval: 5 * time.Minute,
		checkResults:  make(map[int]*HealthCheckResult),
		lastCheckTime: make(map[uint]time.Time),
		stopChan:      make(chan struct{}),
	}
}

// SetCheckInterval sets the health check interval
func (m *HealthCheckManager) SetCheckInterval(interval time.Duration) {
	if interval > 0 {
		m.checkInterval = interval
	}
}

// ShouldCheck determines if a provider should be checked
func (m *HealthCheckManager) ShouldCheck(providerID uint) bool {
	m.mutex.RLock()
	lastTime, exists := m.lastCheckTime[providerID]
	m.mutex.RUnlock()

	// 如果是新的provider（没有检查记录），立即检查
	if !exists {
		return true
	}

	// 检查数据库中provider的首次检查完成状态
	// 如果首次检查未完成，应该立即检查而不考虑时间间隔
	dbProvider, err := GetAiProviderByID(providerID)
	if err == nil && dbProvider != nil && !dbProvider.IsFirstCheckCompleted {
		log.Debugf("Provider %d has not completed first health check, should check immediately", providerID)
		return true
	}

	// 对于已完成首次检查的provider，按间隔检查
	return time.Since(lastTime) > m.checkInterval
}

// RecordCheck records the check time
func (m *HealthCheckManager) RecordCheck(providerID uint) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.lastCheckTime[providerID] = time.Now()
}

// ExecuteHealthCheckLogic performs the core health check on a given provider instance.
// It does not interact with the database or the HealthCheckResult struct directly.
// providerIdentifierForLog is used for logging purposes (e.g., wrapper name or model name).
func ExecuteHealthCheckLogic(p *Provider, providerIdentifierForLog string) (isHealthy bool, latencyMs int64, checkErr error) {
	log.Debugf("Executing health check logic for provider: %s (mode: %s)", providerIdentifierForLog, p.ProviderMode)

	startTime := time.Now()
	var firstByteDuration time.Duration
	rspOnce := new(sync.Once)
	succeededChan := make(chan bool, 1)
	var respErr error

	// 根据 provider mode 选择不同的健康检查方式
	if p.ProviderMode == "embedding" {
		// Embedding 模式：使用 embedding 接口进行健康检查
		log.Debugf("Using embedding health check for provider: %s", providerIdentifierForLog)

		embClient, err := p.GetEmbeddingClient()
		if err != nil {
			errMsg := fmt.Errorf("failed to get embedding client for %s: %v", providerIdentifierForLog, err)
			log.Warnf("Embedding health check preparation failed for %s: %v", providerIdentifierForLog, err)
			return false, 0, errMsg
		}

		log.Debugf("Initiating embedding health check for: %s", providerIdentifierForLog)

		go func() {
			_, embErr := embClient.Embedding("health check test")
			if embErr != nil {
				respErr = embErr
				select {
				case succeededChan <- false:
				default:
				}
			} else {
				rspOnce.Do(func() {
					firstByteDuration = time.Since(startTime)
					succeededChan <- true
				})
			}
		}()
	} else {
		// Chat 模式（默认）：使用 chat 接口进行健康检查
		log.Debugf("Using chat health check for provider: %s", providerIdentifierForLog)

		// Create AI client using the provider's GetAIClient method
		// GetAIClient is assumed to handle its own HTTP client requirements.
		client, err := p.GetAIClient(
			func(reader io.Reader) {
				io.Copy(utils.FirstWriter(func(b []byte) {
					rspOnce.Do(func() {
						firstByteDuration = time.Since(startTime)
						succeededChan <- true
					})
				}), reader)
			},
			func(reader io.Reader) {
				io.Copy(utils.FirstWriter(func(b []byte) {
					rspOnce.Do(func() {
						firstByteDuration = time.Since(startTime)
						succeededChan <- true
					})
				}), reader)
			},
		)

		if err != nil {
			errMsg := fmt.Errorf("failed to get AI client for %s: %v", providerIdentifierForLog, err)
			log.Warnf("Health check preparation failed for %s: %v", providerIdentifierForLog, err)
			return false, 0, errMsg
		}

		log.Debugf("Initiating health check (ping) for: %s", providerIdentifierForLog)

		go func() {
			_, chatErr := client.Chat(healthCheckPrompt)
			if chatErr != nil {
				respErr = chatErr
				select {
				case succeededChan <- false:
				default:
				}
			}
		}()
	}

	var succeeded bool
	select {
	case succeeded = <-succeededChan:
	case <-time.After(20 * time.Second): // 20 seconds timeout
		succeeded = false
		checkErr = fmt.Errorf("health check timeout after 20 seconds for %s", providerIdentifierForLog)
		log.Warnf("Health check timed out (20s) for: %s", providerIdentifierForLog)
	}

	if firstByteDuration == 0 && succeeded { // If succeeded but no bytes read (e.g. empty successful response)
		firstByteDuration = time.Since(startTime)
	} else if firstByteDuration == 0 && !succeeded { // If failed and no bytes, take full duration
		firstByteDuration = time.Since(startTime)
	}

	latencyMs = firstByteDuration.Milliseconds()
	isHealthy = succeeded && latencyMs < 10000 // 10 seconds latency threshold

	if !succeeded && checkErr == nil { // If failed but no explicit timeout error, use respErr
		if respErr != nil {
			checkErr = fmt.Errorf("health check failed for %s: %v", providerIdentifierForLog, respErr)
		} else {
			checkErr = fmt.Errorf("health check failed for %s due to an unknown error", providerIdentifierForLog)
		}
	}

	if isHealthy {
		log.Debugf("Health check successful for %s, Latency: %dms", providerIdentifierForLog, latencyMs)
	} else {
		errMsgLog := "Unknown error"
		if checkErr != nil {
			errMsgLog = checkErr.Error()
		}
		log.Warnf("Health check failed for %s, Latency: %dms, Error: %s", providerIdentifierForLog, latencyMs, errMsgLog)
	}

	return isHealthy, latencyMs, checkErr
}

// CheckProviderHealth checks the health status of a single provider
func CheckProviderHealth(provider *Provider) (*HealthCheckResult, error) {
	// Provider identifier for logging, try DbProvider fields first.
	var providerLogName string
	var providerLogID uint
	if provider != nil && provider.DbProvider != nil {
		providerLogName = provider.DbProvider.WrapperName
		providerLogID = provider.DbProvider.ID
		log.Infof("Start to check ai provider healthy status: %s (ID: %d)", providerLogName, providerLogID)
	} else if provider != nil {
		providerLogName = provider.WrapperName // Fallback to Provider's WrapperName if DbProvider is nil
		if providerLogName == "" {
			providerLogName = provider.ModelName // Further fallback
		}
		log.Infof("start to check ai provider healthy status: %s", providerLogName)
	} else {
		log.Errorf("CheckProviderHealth called with nil provider")
		return &HealthCheckResult{IsHealthy: false, Error: fmt.Errorf("nil provider")}, fmt.Errorf("nil provider")
	}

	result := &HealthCheckResult{
		IsHealthy: false, // Default to not healthy
	}
	if provider.DbProvider != nil {
		result.Provider = provider.DbProvider
	}

	// Use the new core logic function
	// If DbProvider is nil (e.g. temporary validation), providerLogName would have been set to WrapperName or ModelName
	isHealthy, latencyMs, checkErr := ExecuteHealthCheckLogic(provider, providerLogName)

	result.IsHealthy = isHealthy
	result.ResponseTime = latencyMs
	result.Error = checkErr

	// Logging specific to CheckProviderHealth context (especially if DbProvider involved)
	if provider.DbProvider != nil { // Only log with ID if DbProvider is present
		if result.IsHealthy {
			log.Infof("Health Check Finished (CheckProviderHealth): %s (ID: %d), 延迟: %dms",
				providerLogName, providerLogID, result.ResponseTime)
		} else {
			errMsg := "Unknown ERR"
			if result.Error != nil {
				errMsg = result.Error.Error()
			}
			log.Errorf("Health Check Failed (CheckProviderHealth): %s (ID: %d), ERR: %s, Delay: %dms",
				providerLogName, providerLogID, errMsg, result.ResponseTime)
		}
	}
	// For temporary providers, ExecuteHealthCheckLogic already logged details.

	return result, nil // error from ExecuteHealthCheckLogic is in result.Error
}

// CheckAllProviders checks health status of all registered providers
func CheckAllProviders(checkManager *HealthCheckManager) ([]*HealthCheckResult, error) {
	// Get all providers
	dbProviders, err := GetAllAiProviders()
	if err != nil {
		return nil, fmt.Errorf("Failed to get provider list: %v", err)
	}

	// 存储检查结果
	var results []*HealthCheckResult
	var resultsMutex sync.Mutex
	var wg sync.WaitGroup

	// 并发执行健康检查
	for _, dbProvider := range dbProviders {
		// 跳过不需要检查的提供者
		if !checkManager.ShouldCheck(dbProvider.ID) {
			continue
		}

		wg.Add(1)
		go func(dbp *schema.AiProvider) {
			defer wg.Done()

			// 创建 Provider 实例
			provider := &Provider{
				ModelName:    dbp.ModelName,
				TypeName:     dbp.TypeName,
				ProviderMode: dbp.ProviderMode,
				DomainOrURL:  dbp.DomainOrURL,
				APIKey:       dbp.APIKey,
				NoHTTPS:      dbp.NoHTTPS,
				DbProvider:   dbp,
			}

			// 执行健康检查
			result, err := CheckProviderHealth(provider)
			if err != nil {
				log.Errorf("Error checking health status for provider %s (ID: %d): %v", dbp.WrapperName, dbp.ID, err)
				return
			}

			// 记录本次检查
			checkManager.RecordCheck(dbp.ID)

			// 1. 检查延迟是否大于0
			isLatencyValid := result.ResponseTime > 0
			if !isLatencyValid {
				log.Warnf("Provider %s (ID: %d) health check latency is not positive (%dms), marking as unhealthy.", dbp.WrapperName, dbp.ID, result.ResponseTime)
			}

			// 2. 计算基础健康状态（原始检查结果 AND 延迟有效）
			baseHealthy := result.IsHealthy && isLatencyValid

			// 3. 获取首次检查状态
			isFirstCheck := !dbp.IsFirstCheckCompleted

			// 4. 计算最终健康状态
			finalIsHealthy := baseHealthy

			if isFirstCheck {
				log.Infof("Provider %s (ID: %d) completing first health check with result: healthy=%v, latency=%dms", dbp.WrapperName, dbp.ID, baseHealthy, result.ResponseTime)
			}

			if !baseHealthy {
				// 记录不健康的原因
				if !result.IsHealthy {
					errMsg := "check failed"
					if result.Error != nil {
						errMsg = result.Error.Error()
					}
					log.Warnf("Provider %s (ID: %d) marked as unhealthy due to check failure: %s", dbp.WrapperName, dbp.ID, errMsg)
				} else if !isLatencyValid {
					log.Warnf("Provider %s (ID: %d) marked as unhealthy due to non-positive latency: %dms", dbp.WrapperName, dbp.ID, result.ResponseTime)
				}
			}

			// 更新数据库中的健康状态
			dbp.IsHealthy = finalIsHealthy // 使用最终计算出的健康状态
			dbp.LastLatency = result.ResponseTime
			dbp.HealthCheckTime = time.Now()
			dbp.IsFirstCheckCompleted = true // 标记首次检查已完成

			// 更新最后请求状态（这个状态似乎有点冗余，但保持原逻辑）
			if finalIsHealthy { // 使用最终状态判断
				dbp.LastRequestStatus = true
			} else {
				dbp.LastRequestStatus = false
				if result.Error != nil && !isFirstCheck { // 只在非首次检查失败时记录错误详情
					log.Warnf("Provider %s (ID: %d) health check failed details: %v", dbp.WrapperName, dbp.ID, result.Error)
				}
			}

			// 保存到数据库
			if err := UpdateAiProvider(dbp); err != nil {
				log.Errorf("Failed to update provider %s (ID: %d) status: %v", dbp.WrapperName, dbp.ID, err)
			}

			// 使用修改后的状态更新 result，以便返回给调用者
			result.IsHealthy = finalIsHealthy

			// 添加到结果列表
			resultsMutex.Lock()
			results = append(results, result)
			resultsMutex.Unlock()
		}(dbProvider)
	}

	// 等待所有检查完成
	wg.Wait()

	log.Infof("Completed health check for %d AI providers", len(results))
	return results, nil
}

// RunManualHealthCheck 手动执行所有提供者的健康检查
func RunManualHealthCheck() ([]*HealthCheckResult, error) {
	log.Infof("开始执行全部提供者健康检查")

	// 创建一个临时的健康检查管理器
	balancer, err := NewBalancer("")
	if err != nil {
		log.Errorf("创建临时负载均衡器失败: %v", err)
		return nil, fmt.Errorf("Failed to create temporary balancer: %v", err)
	}
	defer balancer.Close()

	healthManager := NewHealthCheckManager(balancer)

	// 获取所有提供者
	dbProviders, err := GetAllAiProviders()
	if err != nil {
		log.Errorf("获取提供者列表失败: %v", err)
		return nil, fmt.Errorf("Failed to get provider list: %v", err)
	}

	if len(dbProviders) == 0 {
		log.Warnf("没有找到可用的提供者")
		// 如果没有提供者，返回空结果而不是错误
		return []*HealthCheckResult{}, nil
	}

	log.Infof("找到 %d 个提供者，准备健康检查", len(dbProviders))

	// 将数据库提供者转换为内存对象并添加到config.Models中
	for _, dbProvider := range dbProviders {
		if dbProvider == nil {
			continue
		}

		provider := &Provider{
			ModelName:    dbProvider.ModelName,
			TypeName:     dbProvider.TypeName,
			ProviderMode: dbProvider.ProviderMode,
			DomainOrURL:  dbProvider.DomainOrURL,
			APIKey:       dbProvider.APIKey,
			NoHTTPS:      dbProvider.NoHTTPS,
			DbProvider:   dbProvider,
		}

		// 使用WrapperName作为模型名，添加到配置的模型列表中
		modelName := dbProvider.WrapperName
		if modelName == "" {
			modelName = dbProvider.ModelName // 如果WrapperName为空，使用ModelName
		}

		// 确保models映射已初始化
		if balancer.config.Models.models == nil {
			balancer.config.Models.models = make(map[string][]*Provider)
		}

		// 添加到模型列表
		balancer.config.Models.models[modelName] = append(balancer.config.Models.models[modelName], provider)
	}

	// 执行健康检查
	log.Infof("开始并行执行健康检查")
	results := CheckAllProvidersHealth(healthManager)
	log.Infof("健康检查完成, 共 %d 个结果", len(results))

	// 统计健康状态
	healthyCount := 0

	// 同步更新数据库中的健康状态
	for _, result := range results {
		if result == nil || result.Provider == nil {
			continue
		}

		dbProvider := result.Provider
		dbProvider.IsHealthy = result.IsHealthy
		dbProvider.LastLatency = result.ResponseTime
		dbProvider.HealthCheckTime = time.Now()

		if result.IsHealthy {
			dbProvider.LastRequestStatus = true
			healthyCount++
		} else {
			dbProvider.LastRequestStatus = false
			if result.Error != nil {
				log.Warnf("提供者 %s (ID: %d) 健康检查失败: %v",
					dbProvider.WrapperName, dbProvider.ID, result.Error)
			}
		}

		// 保存到数据库
		if err := UpdateAiProvider(dbProvider); err != nil {
			log.Errorf("更新提供者 %s (ID: %d) 状态失败: %v",
				dbProvider.WrapperName, dbProvider.ID, err)
		}
	}

	log.Infof("健康检查结果统计: %d/%d 个提供者健康", healthyCount, len(results))
	return results, nil
}

// StartHealthCheckScheduler 启动健康检查调度器
func StartHealthCheckScheduler(balancer *Balancer, interval time.Duration) {
	// 创建健康检查管理器
	manager := NewHealthCheckManager(balancer)

	// 设置健康检查间隔
	if interval > 0 {
		manager.checkInterval = interval
	}

	// 启动定时器，定期执行健康检查
	ticker := time.NewTicker(manager.checkInterval)

	// 首次立即执行一次健康检查
	go func() {
		log.Infof("Running initial health check...")
		results := CheckAllProvidersHealth(manager)
		if results != nil {
			log.Infof("Initial health check completed successfully, checked %d providers", len(results))
		} else {
			log.Warnf("No providers found for initial health check")
		}
	}()

	// 后台持续健康检查
	go func() {
		for {
			select {
			case <-ticker.C:
				log.Infof("Running periodic health check...")
				results := CheckAllProvidersHealth(manager)
				if results != nil {
					log.Infof("Health check completed successfully, checked %d providers", len(results))
				} else {
					log.Warnf("No providers found for health check")
				}
			case <-manager.stopChan:
				ticker.Stop()
				log.Infof("Health check scheduler stopped")
				return
			}
		}
	}()

	log.Infof("Health check scheduler started, check interval: %v", manager.checkInterval)
}

// CheckAllProvidersHealth 检查所有提供者的健康状态
func CheckAllProvidersHealth(manager *HealthCheckManager) []*HealthCheckResult {
	var results []*HealthCheckResult
	var wg sync.WaitGroup
	var mutex sync.Mutex

	// 获取所有提供者
	providers := manager.Balancer.GetProviders()
	if len(providers) == 0 {
		return nil
	}

	// 设置最大并发数，避免同时发起太多请求
	maxConcurrent := 5
	semaphore := make(chan struct{}, maxConcurrent)

	// 并行检查所有提供者
	for _, provider := range providers {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(p *Provider) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			result, err := CheckProviderHealth(p)
			if err != nil {
				log.Errorf("Error checking health status for provider [%s]: %v", p.DbProvider.ModelName, err)
				return
			}

			mutex.Lock()
			results = append(results, result)
			mutex.Unlock()
		}(provider)
	}

	wg.Wait()

	// 按响应时间排序
	sort.Slice(results, func(i, j int) bool {
		// 健康的排在前面
		if results[i].IsHealthy != results[j].IsHealthy {
			return results[i].IsHealthy
		}
		// 响应时间短的排在前面
		return results[i].ResponseTime < results[j].ResponseTime
	})

	return results
}

// GetHealthCheckResult 获取指定提供者的最新健康检查结果
func (m *HealthCheckManager) GetHealthCheckResult(providerID int) *HealthCheckResult {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.checkResults[providerID]
}

// SaveHealthCheckResult 保存健康检查结果
func (m *HealthCheckManager) SaveHealthCheckResult(result *HealthCheckResult) {
	if result == nil || result.Provider == nil {
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.checkResults[int(result.Provider.ID)] = result
}

// GetAllHealthCheckResults 获取所有健康检查结果
func (m *HealthCheckManager) GetAllHealthCheckResults() []*HealthCheckResult {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	results := make([]*HealthCheckResult, 0, len(m.checkResults))
	for _, result := range m.checkResults {
		results = append(results, result)
	}

	return results
}

// StopScheduler 停止健康检查调度器
func (m *HealthCheckManager) StopScheduler() {
	close(m.stopChan)
}

// RunSingleProviderHealthCheck 执行单个提供者的健康检查
func RunSingleProviderHealthCheck(providerID uint) (*HealthCheckResult, error) {
	log.Infof("开始单个提供者健康检查, ID=%d", providerID)

	// 获取指定 ID 的提供者
	dbProvider, err := GetAiProviderByID(providerID)
	if err != nil {
		log.Errorf("获取提供者信息失败 (ID: %d): %v", providerID, err)
		return nil, fmt.Errorf("Failed to get provider info (ID: %d): %v", providerID, err)
	}

	if dbProvider == nil {
		log.Errorf("未找到ID为 %d 的提供者", providerID)
		return nil, fmt.Errorf("Provider not found with ID: %d", providerID)
	}

	// 创建 Provider 实例
	provider := &Provider{
		ModelName:    dbProvider.ModelName,
		TypeName:     dbProvider.TypeName,
		ProviderMode: dbProvider.ProviderMode,
		DomainOrURL:  dbProvider.DomainOrURL,
		APIKey:       dbProvider.APIKey,
		NoHTTPS:      dbProvider.NoHTTPS,
		DbProvider:   dbProvider,
	}

	// 执行健康检查
	log.Infof("开始健康检查: [%s](ID: %d)...", dbProvider.WrapperName, dbProvider.ID)
	result, err := CheckProviderHealth(provider)
	if err != nil {
		log.Errorf("健康检查执行失败: %v", err)
		return nil, fmt.Errorf("Health check failed: %v", err)
	}

	// 检查结果是否为空
	if result == nil {
		log.Errorf("健康检查返回空结果")
		return &HealthCheckResult{
			Provider:     dbProvider,
			IsHealthy:    false,
			ResponseTime: 0,
			Error:        fmt.Errorf("Health check returned empty result"),
		}, nil
	}

	// 1. 检查延迟是否大于0
	isLatencyValid := result.ResponseTime > 0
	if !isLatencyValid {
		log.Warnf("Provider %s (ID: %d) single health check latency is not positive (%dms), marking as unhealthy.", dbProvider.WrapperName, dbProvider.ID, result.ResponseTime)
	}

	// 2. 计算基础健康状态（原始检查结果 AND 延迟有效）
	baseHealthy := result.IsHealthy && isLatencyValid

	// 3. 获取首次检查状态
	isFirstCheck := !dbProvider.IsFirstCheckCompleted

	// 4. 计算最终健康状态
	finalIsHealthy := baseHealthy

	if isFirstCheck {
		log.Infof("Provider %s (ID: %d) completing first health check with result: healthy=%v, latency=%dms", dbProvider.WrapperName, dbProvider.ID, baseHealthy, result.ResponseTime)
	}

	if !baseHealthy {
		// 记录不健康的原因
		if !result.IsHealthy {
			errMsg := "check failed"
			if result.Error != nil {
				errMsg = result.Error.Error()
			}
			log.Warnf("Provider %s (ID: %d) marked as unhealthy due to check failure: %s", dbProvider.WrapperName, dbProvider.ID, errMsg)
		} else if !isLatencyValid {
			log.Warnf("Provider %s (ID: %d) marked as unhealthy due to non-positive latency: %dms", dbProvider.WrapperName, dbProvider.ID, result.ResponseTime)
		}
	}

	// 更新数据库状态
	dbProvider.IsHealthy = finalIsHealthy // 使用最终计算出的健康状态
	dbProvider.LastLatency = result.ResponseTime
	dbProvider.HealthCheckTime = time.Now()
	dbProvider.IsFirstCheckCompleted = true // 标记首次检查已完成
	if finalIsHealthy {
		dbProvider.LastRequestStatus = true
	} else {
		dbProvider.LastRequestStatus = false
	}

	// --- 使用 Updates 方法显式更新字段 ---
	updateData := map[string]interface{}{
		"is_healthy":               finalIsHealthy,
		"last_latency":             result.ResponseTime,
		"health_check_time":        time.Now(),
		"is_first_check_completed": true,           // 标记首次检查已完成
		"last_request_status":      finalIsHealthy, // 更新最后请求状态
	}

	if err := GetDB().Model(&dbProvider).Updates(updateData).Error; err != nil {
		// 记录错误，但仍然返回检查结果
		log.Errorf("Failed to update provider %s (ID: %d) status after single health check: %v", dbProvider.WrapperName, dbProvider.ID, err)
		// 将错误信息附加到结果中，但不覆盖原始的检查错误（如果存在）
		if result.Error == nil {
			result.Error = fmt.Errorf("failed to update provider status: %w", err)
		} else {
			result.Error = fmt.Errorf("health check error: %v; also failed to update provider status: %w", result.Error, err)
		}
	}

	// 使用修改后的状态更新 result，以便返回给调用者
	result.IsHealthy = finalIsHealthy

	// 返回结果
	return result, nil
}
