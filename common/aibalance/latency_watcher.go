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

	// healthCheckInFlight tracks provider IDs for which a latency-triggered health
	// check goroutine is currently running, implementing per-provider singleflight
	// so repeated problematic ticks don't pile up duplicate checks.
	// (incident 2026-06-22: same provider triggered immediate checks every 10s
	// while already unhealthy, each spawning a new goroutine that timed out at 20s+.)
	healthCheckInFlight map[uint]bool

	// lastLatencyCheckAt records when a latency-triggered health check last started
	// for a provider, used to enforce a cooldown so we don't re-check the same
	// unhealthy provider on every fast-interval tick.
	lastLatencyCheckAt map[uint]time.Time
}

var (
	globalLatencyWatcher *LatencyWatcher
	latencyWatcherOnce   sync.Once
	latencyWatcherMutex  sync.Mutex
)

// NewLatencyWatcher creates a new latency watcher
func NewLatencyWatcher() *LatencyWatcher {
	return &LatencyWatcher{
		normalInterval:      5 * time.Minute,
		fastInterval:        10 * time.Second,
		latencyThreshold:    10000, // 10 seconds
		stopChan:            make(chan struct{}),
		problematicIDs:      make(map[uint]time.Time),
		healthCheckInFlight: make(map[uint]bool),
		lastLatencyCheckAt:  make(map[uint]time.Time),
		running:             false,
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
			// Trigger health check for problematic provider (runs in fast mode),
			// but only if one is not already running and the per-provider cooldown
			// has elapsed. This singleflight + cooldown is what prevents the
			// health-check storm seen in the incident, where the same unhealthy
			// provider spawned a new 20s-timeout goroutine every fast tick.
			// 关键词: LatencyWatcher singleflight cooldown, 健康检查放大修复
			if w.shouldTriggerLatencyCheckLocked(p.ID) {
				w.healthCheckInFlight[p.ID] = true
				w.lastLatencyCheckAt[p.ID] = time.Now()
				pid := p.ID
				pname := p.WrapperName
				go func() {
					defer func() {
						w.mutex.Lock()
						w.healthCheckInFlight[pid] = false
						w.mutex.Unlock()
					}()
					w.triggerHealthCheck(pid, pname)
				}()
			}
		} else if wasTracked {
			// Provider has recovered
			log.Debugf("LatencyWatcher: provider %s (ID: %d) has recovered, latency: %dms, healthy: %v",
				p.WrapperName, p.ID, p.LastLatency, p.IsHealthy)
			delete(w.problematicIDs, p.ID)
			delete(w.healthCheckInFlight, p.ID)
			delete(w.lastLatencyCheckAt, p.ID)
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
// shouldTriggerLatencyCheckLocked returns true if a latency-triggered health
// check may be started now for the given provider. It enforces:
//   - singleflight: no check is currently in flight for this provider
//   - cooldown: at least latencyCheckCooldown has passed since the last check
//
// MUST be called with w.mutex held. 关键词: shouldTriggerLatencyCheckLocked
func (w *LatencyWatcher) shouldTriggerLatencyCheckLocked(providerID uint) bool {
	if w.healthCheckInFlight[providerID] {
		return false
	}
	last, ok := w.lastLatencyCheckAt[providerID]
	if ok && time.Since(last) < latencyCheckCooldown {
		return false
	}
	return true
}

// latencyCheckCooldown is the minimum spacing between latency-triggered health
// checks for the SAME provider. It is deliberately larger than fastInterval so
// that even in fast mode a single unhealthy provider can produce at most one
// check per cooldown window, breaking the amplification loop.
// 关键词: latencyCheckCooldown, 健康检查去重冷却
const latencyCheckCooldown = 60 * time.Second


// triggerHealthCheck triggers a health check for a specific provider
func (w *LatencyWatcher) triggerHealthCheck(providerID uint, providerName string) {
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

// MarkProviderAsProblematic marks a provider as problematic and triggers immediate monitoring.
//
// It also enforces the same per-provider singleflight + cooldown used by checkProviders,
// because this method is called from the request hot path (PeekOrderedProvidersWithAffinity)
// when no low-latency provider is available — without dedup, a burst of requests for a
// model whose providers are all high-latency would each spawn a health-check goroutine.
// 关键词: MarkProviderAsProblematic singleflight, 请求热路径健康检查去重
func (w *LatencyWatcher) MarkProviderAsProblematic(providerID uint, providerName string) {
	w.mutex.Lock()
	if _, exists := w.problematicIDs[providerID]; !exists {
		w.problematicIDs[providerID] = time.Now()
		log.Infof("LatencyWatcher: provider %s (ID: %d) marked as problematic, will monitor with fast interval",
			providerName, providerID)
	}
	// Respect singleflight + cooldown before spawning a check from the hot path.
	if !w.shouldTriggerLatencyCheckLocked(providerID) {
		w.mutex.Unlock()
		return
	}
	w.healthCheckInFlight[providerID] = true
	w.lastLatencyCheckAt[providerID] = time.Now()
	w.mutex.Unlock()

	// Trigger immediate health check (only one in flight per provider at a time).
	go func() {
		defer func() {
			w.mutex.Lock()
			w.healthCheckInFlight[providerID] = false
			w.mutex.Unlock()
		}()
		w.triggerHealthCheck(providerID, providerName)
	}()
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
