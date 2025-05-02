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

// HealthCheckResult 存储健康检查结果
type HealthCheckResult struct {
	Provider     *schema.AiProvider
	IsHealthy    bool
	ResponseTime int64 // 毫秒
	Error        error
}

// 使用简单的 ping 消息进行健康检查
const healthCheckPrompt = "Ping. Please respond with 'Pong'."

// HealthCheckManager 管理提供者的健康检查
type HealthCheckManager struct {
	Balancer      *Balancer                  // 负载均衡器
	checkInterval time.Duration              // 健康检查间隔
	checkResults  map[int]*HealthCheckResult // 存储最新的健康检查结果
	lastCheckTime map[uint]time.Time         // 存储上次检查时间，避免频繁检查同一提供者
	mutex         sync.RWMutex               // 保护 checkResults 和 lastCheckTime 的互斥锁
	stopChan      chan struct{}              // 停止健康检查的信号
}

// NewHealthCheckManager 创建健康检查管理器
func NewHealthCheckManager(balancer *Balancer) *HealthCheckManager {
	return &HealthCheckManager{
		Balancer:      balancer,
		checkInterval: 5 * time.Minute,
		checkResults:  make(map[int]*HealthCheckResult),
		lastCheckTime: make(map[uint]time.Time),
		stopChan:      make(chan struct{}),
	}
}

// SetCheckInterval 设置健康检查的间隔时间
func (m *HealthCheckManager) SetCheckInterval(interval time.Duration) {
	if interval > 0 {
		m.checkInterval = interval
	}
}

// ShouldCheck 判断是否应该检查指定的提供者
func (m *HealthCheckManager) ShouldCheck(providerID uint) bool {
	m.mutex.RLock()
	lastTime, exists := m.lastCheckTime[providerID]
	m.mutex.RUnlock()

	if !exists {
		return true
	}

	return time.Since(lastTime) > m.checkInterval
}

// RecordCheck 记录检查时间
func (m *HealthCheckManager) RecordCheck(providerID uint) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.lastCheckTime[providerID] = time.Now()
}

// CheckProviderHealth 检查单个提供者的健康状态
func CheckProviderHealth(provider *Provider) (*HealthCheckResult, error) {
	result := &HealthCheckResult{
		Provider:  provider.DbProvider,
		IsHealthy: false,
	}

	// 计时开始
	startTime := time.Now()
	var firstByteDuration time.Duration
	rspOnce := new(sync.Once)
	succeededChan := make(chan bool, 1) // 用于存储是否成功的结果
	var respErr error                   // 用于存储响应错误

	// 创建 AI 客户端
	client, err := provider.GetAIClient(
		func(reader io.Reader) {
			io.Copy(utils.FirstWriter(func(b []byte) {
				rspOnce.Do(func() {
					firstByteDuration = time.Since(startTime)
					// 收到第一个字节，说明请求成功
					succeededChan <- true
				})
			}), reader)
		},
		func(reader io.Reader) {
			io.Copy(utils.FirstWriter(func(b []byte) {
				rspOnce.Do(func() {
					firstByteDuration = time.Since(startTime)
					// 收到第一个字节，说明请求成功
					succeededChan <- true
				})
			}), reader)
		},
	)
	if err != nil {
		result.Error = fmt.Errorf("获取 AI 客户端失败: %v", err)
		return result, nil
	}

	// 创建一个异步的 goroutine 发送 ping 请求
	go func() {
		// 执行 Chat ping
		_, err := client.Chat(healthCheckPrompt)
		if err != nil {
			respErr = err
			// 如果还没有接收到第一个字节时出错，则标记为失败
			select {
			case succeededChan <- false:
			default:
				// 如果管道已经关闭或已经有值，则不做任何事
			}
		}
	}()

	// 设置超时等待结果
	var succeeded bool
	select {
	case succeeded = <-succeededChan:
		// 成功获取到结果
	case <-time.After(15 * time.Second): // 15秒超时
		succeeded = false
		result.Error = fmt.Errorf("健康检查超时")
	}

	// 如果没有收到任何字节，firstByteDuration 可能为 0
	// 使用当前时间与起始时间的差值作为响应时间
	if firstByteDuration == 0 {
		firstByteDuration = time.Since(startTime)
	}

	// 设置响应时间
	result.ResponseTime = firstByteDuration.Milliseconds()

	// 根据 server.go 中的 UpdateDbProvider 逻辑判断健康状态：
	// 如果响应成功且延迟小于 3000ms，则标记为健康
	result.IsHealthy = succeeded && result.ResponseTime < 3000

	// 如果有错误但尚未设置错误消息
	if !succeeded && result.Error == nil && respErr != nil {
		result.Error = fmt.Errorf("健康检查失败: %v", respErr)
	}

	return result, nil
}

// CheckAllProviders 检查所有注册的提供者健康状态
func CheckAllProviders(checkManager *HealthCheckManager) ([]*HealthCheckResult, error) {
	// 获取所有提供者
	dbProviders, err := GetAllAiProviders()
	if err != nil {
		return nil, fmt.Errorf("获取提供者列表失败: %v", err)
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
				ModelName:   dbp.ModelName,
				TypeName:    dbp.TypeName,
				DomainOrURL: dbp.DomainOrURL,
				APIKey:      dbp.APIKey,
				NoHTTPS:     dbp.NoHTTPS,
				DbProvider:  dbp,
			}

			// 执行健康检查
			result, err := CheckProviderHealth(provider)
			if err != nil {
				log.Errorf("检查提供者 %s (ID: %d) 健康状态时出错: %v", dbp.WrapperName, dbp.ID, err)
				return
			}

			// 记录本次检查
			checkManager.RecordCheck(dbp.ID)

			// 更新数据库中的健康状态
			dbp.IsHealthy = result.IsHealthy
			dbp.LastLatency = result.ResponseTime
			dbp.HealthCheckTime = time.Now()
			if result.IsHealthy {
				dbp.LastRequestStatus = true
			} else {
				dbp.LastRequestStatus = false
				if result.Error != nil {
					log.Warnf("提供者 %s (ID: %d) 健康检查失败: %v", dbp.WrapperName, dbp.ID, result.Error)
				}
			}

			// 保存到数据库
			if err := UpdateAiProvider(dbp); err != nil {
				log.Errorf("更新提供者 %s (ID: %d) 状态失败: %v", dbp.WrapperName, dbp.ID, err)
			}

			// 添加到结果列表
			resultsMutex.Lock()
			results = append(results, result)
			resultsMutex.Unlock()
		}(dbProvider)
	}

	// 等待所有检查完成
	wg.Wait()

	log.Infof("完成 %d 个 AI 提供者的健康检查", len(results))
	return results, nil
}

// RunManualHealthCheck 手动执行所有提供者的健康检查
func RunManualHealthCheck() ([]*HealthCheckResult, error) {
	// 获取一个 Balancer 实例
	balancer, err := NewBalancer("")
	if err != nil {
		return nil, fmt.Errorf("创建 Balancer 实例失败: %v", err)
	}
	defer balancer.Close()

	// 创建临时健康检查管理器
	manager := NewHealthCheckManager(balancer)

	// 执行健康检查
	log.Infof("开始手动健康检查...")
	results := CheckAllProvidersHealth(manager)

	// 更新数据库中提供者的健康状态
	for _, result := range results {
		if result.Provider != nil {
			result.Provider.IsHealthy = result.IsHealthy
			// 保存到数据库
			err := UpdateAiProvider(result.Provider)
			if err != nil {
				log.Errorf("更新提供者健康状态失败: %v", err)
			}
		}
	}

	log.Infof("手动健康检查完成，共检查 %d 个提供者", len(results))
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
		log.Infof("执行初始健康检查...")
		results := CheckAllProvidersHealth(manager)
		if results != nil {
			log.Infof("成功完成初始健康检查，检查了 %d 个提供者", len(results))
		} else {
			log.Warnf("初始健康检查没有找到提供者")
		}
	}()

	// 后台持续健康检查
	go func() {
		for {
			select {
			case <-ticker.C:
				log.Infof("执行定期健康检查...")
				results := CheckAllProvidersHealth(manager)
				if results != nil {
					log.Infof("成功完成健康检查，检查了 %d 个提供者", len(results))
				} else {
					log.Warnf("健康检查没有找到提供者")
				}
			case <-manager.stopChan:
				ticker.Stop()
				log.Infof("健康检查调度器已停止")
				return
			}
		}
	}()

	log.Infof("已启动健康检查调度器，检查间隔: %v", manager.checkInterval)
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
				log.Errorf("检查提供者[%s]健康状态时出错: %v", p.DbProvider.ModelName, err)
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
	// 获取指定 ID 的提供者
	dbProvider, err := GetAiProviderByID(providerID)
	if err != nil {
		return nil, fmt.Errorf("获取提供者信息失败(ID: %d): %v", providerID, err)
	}

	if dbProvider == nil {
		return nil, fmt.Errorf("未找到指定 ID 的提供者: %d", providerID)
	}

	// 创建 Provider 实例
	provider := &Provider{
		ModelName:   dbProvider.ModelName,
		TypeName:    dbProvider.TypeName,
		DomainOrURL: dbProvider.DomainOrURL,
		APIKey:      dbProvider.APIKey,
		NoHTTPS:     dbProvider.NoHTTPS,
		DbProvider:  dbProvider,
	}

	// 执行健康检查
	log.Infof("开始对提供者 [%s](ID: %d) 进行健康检查...", dbProvider.WrapperName, dbProvider.ID)
	result, err := CheckProviderHealth(provider)
	if err != nil {
		return nil, fmt.Errorf("健康检查失败: %v", err)
	}

	// 更新数据库中的健康状态
	dbProvider.IsHealthy = result.IsHealthy
	dbProvider.LastLatency = result.ResponseTime
	dbProvider.HealthCheckTime = time.Now()
	dbProvider.LastRequestTime = time.Now()
	if result.IsHealthy {
		dbProvider.LastRequestStatus = true
	} else {
		dbProvider.LastRequestStatus = false
	}

	// 保存到数据库
	if err := UpdateAiProvider(dbProvider); err != nil {
		log.Errorf("更新提供者状态失败: %v", err)
	} else {
		log.Infof("提供者 [%s](ID: %d) 健康检查完成: 健康状态=%v, 响应时间=%dms",
			dbProvider.WrapperName, dbProvider.ID, result.IsHealthy, result.ResponseTime)
	}

	return result, nil
}
