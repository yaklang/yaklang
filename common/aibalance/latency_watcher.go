package aibalance

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// LatencyWatcher monitors provider latency and triggers health checks when issues are detected
// It runs as a background goroutine and observes the health status of all providers
type LatencyWatcher struct {
	normalInterval   time.Duration      // Normal check interval (e.g., 5 minutes)
	fastInterval     time.Duration      // Fast check interval when issues detected (e.g., 10 seconds)
	latencyThreshold int64              // Latency threshold in ms (e.g., 10000ms = 10s)
	stopChan         chan struct{}      // Channel to stop the watcher
	mutex            sync.RWMutex       // Protect concurrent access
	problematicIDs   map[uint]time.Time // Track problematic provider IDs and when they were detected
	running          bool               // Whether the watcher is running

	// inFlightChecks 记录当前正在执行健康检查的 provider, 受 mutex 保护.
	// 事故里坏 provider 每 10s tick 都会 go triggerHealthCheck, 无去重导致同一
	// provider 叠加大量在途健康检查 goroutine (每个又因上游卡死 20s 才返回),
	// 进一步加剧调度器饥饿. 这里做在途去重: 同一 provider 同时只允许一个在途检查.
	// 关键词: LatencyWatcher inFlightChecks, 健康检查在途去重, 防 goroutine 叠加
	inFlightChecks map[uint]struct{}
}

var (
	globalLatencyWatcher *LatencyWatcher
	latencyWatcherOnce   sync.Once
	latencyWatcherMutex  sync.Mutex
)

// NewLatencyWatcher creates a new latency watcher
func NewLatencyWatcher() *LatencyWatcher {
	return &LatencyWatcher{
		normalInterval:   5 * time.Minute,
		fastInterval:     10 * time.Second,
		latencyThreshold: 10000, // 10 seconds
		stopChan:         make(chan struct{}),
		problematicIDs:   make(map[uint]time.Time),
		inFlightChecks:   make(map[uint]struct{}),
		running:          false,
	}
}

// GetGlobalLatencyWatcher returns the global latency watcher instance (singleton)
func GetGlobalLatencyWatcher() *LatencyWatcher {
	latencyWatcherOnce.Do(func() {
		globalLatencyWatcher = NewLatencyWatcher()
	})
	return globalLatencyWatcher
}

// StartLatencyWatcher starts the global latency watcher
func StartLatencyWatcher() {
	watcher := GetGlobalLatencyWatcher()
	watcher.Start()
}

// StopLatencyWatcher stops the global latency watcher
func StopLatencyWatcher() {
	if globalLatencyWatcher != nil {
		globalLatencyWatcher.Stop()
	}
}

// Start begins the latency watcher background goroutine.
//
// 注意：支持 Stop -> Start 的循环（测试场景需要在 chat baseline 采样
// 之前暂停 watcher、采样后重新开启）。如果之前的 Stop 已经 close 了
// stopChan，这里会重建一个新的 channel，让 watchLoop 不会刚启动就退出。
//
// 关键词: LatencyWatcher Start 支持 stopChan 重建, Stop->Start 循环
func (w *LatencyWatcher) Start() {
	w.mutex.Lock()
	if w.running {
		w.mutex.Unlock()
		log.Infof("LatencyWatcher is already running")
		return
	}
	// 如果旧 stopChan 已 close，重建一个，避免 watchLoop 起来就退出。
	select {
	case <-w.stopChan:
		w.stopChan = make(chan struct{})
	default:
	}
	w.running = true
	w.mutex.Unlock()

	log.Infof("Starting LatencyWatcher with normal interval %v and fast interval %v", w.normalInterval, w.fastInterval)

	go w.watchLoop()
}

// Stop stops the latency watcher
func (w *LatencyWatcher) Stop() {
	w.mutex.Lock()
	if !w.running {
		w.mutex.Unlock()
		return
	}
	w.running = false
	w.mutex.Unlock()

	close(w.stopChan)
	log.Infof("LatencyWatcher stopped")
}

// watchLoop is the main loop that monitors provider latency
func (w *LatencyWatcher) watchLoop() {
	// Use fast interval ticker for quick response
	ticker := time.NewTicker(w.fastInterval)
	defer ticker.Stop()

	lastNormalCheck := time.Now()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.checkProviders(time.Since(lastNormalCheck) >= w.normalInterval)
			if time.Since(lastNormalCheck) >= w.normalInterval {
				lastNormalCheck = time.Now()
			}
		}
	}
}

// checkProviders checks all providers and triggers health checks for problematic ones
func (w *LatencyWatcher) checkProviders(isNormalCheck bool) {
	providers, err := GetAllAiProviders()
	if err != nil {
		log.Errorf("LatencyWatcher: failed to get providers: %v", err)
		return
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	for _, p := range providers {
		if p == nil {
			continue
		}

		isProblematic := w.isProviderProblematic(p)

		// Check if this provider was previously tracked as problematic
		_, wasTracked := w.problematicIDs[p.ID]

		if isProblematic {
			if !wasTracked {
				// New problematic provider detected
				log.Warnf("LatencyWatcher: detected problematic provider %s (ID: %d), latency: %dms, healthy: %v, triggering immediate health check",
					p.WrapperName, p.ID, p.LastLatency, p.IsHealthy)
				w.problematicIDs[p.ID] = time.Now()
			}
			// 在途去重: 仅当该 provider 没有在途健康检查时才起新检查, 避免多 tick
			// 叠加大量卡在上游的健康检查 goroutine. 此处已持有 w.mutex, 直接读写
			// inFlightChecks. 关键词: checkProviders 在途去重, 防健康检查 goroutine 叠加
			if _, inflight := w.inFlightChecks[p.ID]; !inflight {
				w.inFlightChecks[p.ID] = struct{}{}
				go w.triggerHealthCheck(p.ID, p.WrapperName)
			} else {
				log.Debugf("LatencyWatcher: provider %s (ID: %d) health check already in flight, skip duplicate",
					p.WrapperName, p.ID)
			}
		} else if wasTracked {
			// Provider has recovered
			log.Debugf("LatencyWatcher: provider %s (ID: %d) has recovered, latency: %dms, healthy: %v",
				p.WrapperName, p.ID, p.LastLatency, p.IsHealthy)
			delete(w.problematicIDs, p.ID)
		}
	}

	// Log summary
	if len(w.problematicIDs) > 0 {
		log.Debugf("LatencyWatcher: currently monitoring %d problematic providers with fast health check interval (%v)",
			len(w.problematicIDs), w.fastInterval)
	}
}

// isProviderProblematic checks if a provider is problematic based on latency and health status
func (w *LatencyWatcher) isProviderProblematic(p *AiProvider) bool {
	// Provider is problematic if:
	// 1. First check not completed (highest priority - need to complete first check)
	if !p.IsFirstCheckCompleted {
		return true
	}

	// After first check is completed, check other conditions:
	// 2. Not healthy
	// 3. Latency is 0 (no latency data)
	// 4. Latency exceeds threshold
	return !p.IsHealthy ||
		p.LastLatency <= 0 ||
		p.LastLatency >= w.latencyThreshold
}

// tryStartCheck 原子地尝试为 provider 抢占一个"在途健康检查"名额; 已有在途检查时
// 返回 false (调用方应跳过), 实现多 tick 去重. 该方法自行加锁, 不可在已持有
// w.mutex 时调用 (checkProviders 内部直接读写 inFlightChecks).
//
// 关键词: tryStartCheck, 健康检查在途名额抢占
func (w *LatencyWatcher) tryStartCheck(providerID uint) bool {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if _, ok := w.inFlightChecks[providerID]; ok {
		return false
	}
	w.inFlightChecks[providerID] = struct{}{}
	return true
}

// finishCheck 清除 provider 的在途健康检查标记.
//
// 关键词: finishCheck, 释放在途名额
func (w *LatencyWatcher) finishCheck(providerID uint) {
	w.mutex.Lock()
	delete(w.inFlightChecks, providerID)
	w.mutex.Unlock()
}

// triggerHealthCheck triggers a health check for a specific provider.
// 调用前必须已抢占在途名额 (checkProviders 直接置位 / MarkProviderAsProblematic
// 用 tryStartCheck), 这里负责在结束时释放, 保证名额与 goroutine 生命周期一致.
func (w *LatencyWatcher) triggerHealthCheck(providerID uint, providerName string) {
	defer w.finishCheck(providerID)
	log.Debugf("LatencyWatcher: triggering health check for provider %s (ID: %d)", providerName, providerID)
	result, err := RunSingleProviderHealthCheck(providerID)
	if err != nil {
		log.Errorf("LatencyWatcher: health check failed for provider %s (ID: %d): %v", providerName, providerID, err)
		return
	}

	if result != nil {
		log.Debugf("LatencyWatcher: health check result for provider %s (ID: %d): healthy=%v, latency=%dms",
			providerName, providerID, result.IsHealthy, result.ResponseTime)
	}
}

// MarkProviderAsProblematic marks a provider as problematic and triggers immediate monitoring
func (w *LatencyWatcher) MarkProviderAsProblematic(providerID uint, providerName string) {
	w.mutex.Lock()
	if _, exists := w.problematicIDs[providerID]; !exists {
		w.problematicIDs[providerID] = time.Now()
		log.Infof("LatencyWatcher: provider %s (ID: %d) marked as problematic, will monitor with fast interval",
			providerName, providerID)
	}
	w.mutex.Unlock()

	// 在途去重: 仅当没有在途检查时才起新检查, 避免重复 mark 叠加 goroutine.
	// 关键词: MarkProviderAsProblematic 在途去重
	if w.tryStartCheck(providerID) {
		go w.triggerHealthCheck(providerID, providerName)
	}
}

// GetProblematicProviderCount returns the number of currently tracked problematic providers
func (w *LatencyWatcher) GetProblematicProviderCount() int {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	return len(w.problematicIDs)
}

// TriggerImmediateHealthCheckForModel triggers immediate health checks for all providers of a model
// Returns providers that are available after the check
func TriggerImmediateHealthCheckForModel(modelName string, allProviders []*Provider) []*Provider {
	if len(allProviders) == 0 {
		return nil
	}

	log.Infof("TriggerImmediateHealthCheckForModel: triggering immediate health check for %d providers of model %s",
		len(allProviders), modelName)

	var wg sync.WaitGroup
	var mutex sync.Mutex
	var availableProviders []*Provider

	// Limit concurrent health checks
	semaphore := make(chan struct{}, 5)

	for _, p := range allProviders {
		if p == nil || p.DbProvider == nil {
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(provider *Provider) {
			defer wg.Done()
			defer func() { <-semaphore }()

			providerID := provider.DbProvider.ID
			providerName := provider.DbProvider.WrapperName

			log.Debugf("TriggerImmediateHealthCheckForModel: checking provider %s (ID: %d)", providerName, providerID)

			result, err := RunSingleProviderHealthCheck(providerID)
			if err != nil {
				log.Warnf("TriggerImmediateHealthCheckForModel: health check failed for %s (ID: %d): %v",
					providerName, providerID, err)
				return
			}

			// If provider is now healthy (regardless of latency for fallback), add to available list
			if result != nil && result.IsHealthy {
				mutex.Lock()
				// Update the provider's DbProvider with latest status
				provider.DbProvider.IsHealthy = result.IsHealthy
				provider.DbProvider.LastLatency = result.ResponseTime
				availableProviders = append(availableProviders, provider)
				mutex.Unlock()
				log.Infof("TriggerImmediateHealthCheckForModel: provider %s (ID: %d) is available, latency: %dms",
					providerName, providerID, result.ResponseTime)
			}
		}(p)
	}

	wg.Wait()

	log.Infof("TriggerImmediateHealthCheckForModel: found %d available providers for model %s after health check",
		len(availableProviders), modelName)

	return availableProviders
}
