package aibalance

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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

// Start begins the latency watcher background goroutine
func (w *LatencyWatcher) Start() {
	w.mutex.Lock()
	if w.running {
		w.mutex.Unlock()
		log.Infof("LatencyWatcher is already running")
		return
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
			// Trigger health check for problematic provider (runs in fast mode)
			go w.triggerHealthCheck(p.ID, p.WrapperName)
		} else if wasTracked {
			// Provider has recovered
			log.Infof("LatencyWatcher: provider %s (ID: %d) has recovered, latency: %dms, healthy: %v",
				p.WrapperName, p.ID, p.LastLatency, p.IsHealthy)
			delete(w.problematicIDs, p.ID)
		}
	}

	// Log summary
	if len(w.problematicIDs) > 0 {
		log.Infof("LatencyWatcher: currently monitoring %d problematic providers with fast health check interval (%v)",
			len(w.problematicIDs), w.fastInterval)
	}
}

// isProviderProblematic checks if a provider is problematic based on latency and health status
func (w *LatencyWatcher) isProviderProblematic(p *schema.AiProvider) bool {
	// Provider is problematic if:
	// 1. Not healthy
	// 2. Latency is 0 (no latency data)
	// 3. Latency exceeds threshold
	// 4. First check not completed
	return !p.IsHealthy ||
		p.LastLatency <= 0 ||
		p.LastLatency >= w.latencyThreshold ||
		!p.IsFirstCheckCompleted
}

// triggerHealthCheck triggers a health check for a specific provider
func (w *LatencyWatcher) triggerHealthCheck(providerID uint, providerName string) {
	log.Debugf("LatencyWatcher: triggering health check for provider %s (ID: %d)", providerName, providerID)
	result, err := RunSingleProviderHealthCheck(providerID)
	if err != nil {
		log.Errorf("LatencyWatcher: health check failed for provider %s (ID: %d): %v", providerName, providerID, err)
		return
	}

	if result != nil {
		log.Infof("LatencyWatcher: health check result for provider %s (ID: %d): healthy=%v, latency=%dms",
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

	// Trigger immediate health check
	go w.triggerHealthCheck(providerID, providerName)
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
