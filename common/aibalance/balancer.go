package aibalance

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

// Used to ensure the health check scheduler is started only once
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

	// Load and restore providers from database
	if err := LoadProvidersFromDatabase(serverConfig); err != nil {
		log.Warnf("Failed to load providers from database: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &Balancer{
		config: serverConfig,
		ctx:    ctx,
		cancel: cancel,
	}

	// Start health check scheduler (ensure it only starts once)
	healthCheckSchedulerStarted.Do(func() {
		StartHealthCheckScheduler(b, 10*time.Minute) // Check every 10 minutes
	})

	return b, nil
}

// NewBalancer creates a new balancer instance. If the config file cannot be read,
// it will create a default configuration and load from the database
func NewBalancer(configFile string) (*Balancer, error) {
	// Try to read the config file
	raw, err := os.ReadFile(configFile)
	if err != nil {
		// If config file doesn't exist, create a basic server config
		log.Warnf("Failed to read config file %s: %v, using default configuration and loading from database", configFile, err)

		// Create default configuration
		serverConfig := NewServerConfig()

		// Load and restore providers from database
		if err := LoadProvidersFromDatabase(serverConfig); err != nil {
			log.Warnf("Failed to load providers from database: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		b := &Balancer{
			config: serverConfig,
			ctx:    ctx,
			cancel: cancel,
		}

		// Start health check scheduler (ensure it only starts once)
		healthCheckSchedulerStarted.Do(func() {
			StartHealthCheckScheduler(b, 10*time.Minute) // Check every 10 minutes
		})

		return b, nil
	}

	// If config file exists, create normally
	b, err := NewBalancerFromRawConfig(raw, configFile)
	if err != nil {
		return nil, err
	}

	// --- BEGIN DATA FIX ---
	// Perform one-time update for existing healthy providers
	// that were added before the IsFirstCheckCompleted field was introduced.
	if err := fixHistoricalProviderHealthState(); err != nil {
		// Log the error but don't block startup
		log.Errorf("Failed to fix historical provider health states: %v", err)
	}
	// --- END DATA FIX ---

	return b, nil
}

// fixHistoricalProviderHealthState updates providers that were healthy before the IsFirstCheckCompleted field was added.
func fixHistoricalProviderHealthState() error {
	log.Infof("Checking for historical providers needing health state fix...")
	db := GetDB() // Assuming GetDB() returns the correct *gorm.DB instance
	if db == nil {
		return fmt.Errorf("database connection is nil, cannot perform fix")
	}

	// Find providers where: IsFirstCheckCompleted is false AND IsHealthy is true
	// We don't need to check HealthCheckTime specifically, as IsHealthy=true implies a successful check happened.
	result := db.Model(&schema.AiProvider{}).
		Where("is_first_check_completed = ? AND is_healthy = ?", false, true).
		Update("is_first_check_completed", true)

	if result.Error != nil {
		return fmt.Errorf("database update failed: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Infof("Successfully fixed health state for %d historical providers.", result.RowsAffected)
	} else {
		log.Infof("No historical providers needed health state fixing.")
	}

	return nil
}

// LoadProvidersFromDatabase loads all providers and API keys from the database
func LoadProvidersFromDatabase(config *ServerConfig) error {
	log.Infof("Starting to load AI providers and API keys from database")

	// 1. Load AI providers
	// Get all providers from the database
	dbProviders, err := GetAllAiProviders()
	if err != nil {
		return utils.Errorf("Failed to get AI providers from database: %v", err)
	}

	log.Infof("Retrieved %d AI providers from database", len(dbProviders))

	// Group providers by WrapperName
	modelProviders := make(map[string][]*Provider)

	for _, dbProvider := range dbProviders {
		// Skip invalid providers
		if dbProvider.TypeName == "" || dbProvider.ModelName == "" {
			log.Warnf("Skipping invalid provider: TypeName=%s, ModelName=%s", dbProvider.TypeName, dbProvider.ModelName)
			continue
		}

		// Create Provider instance
		provider := &Provider{
			ModelName:    dbProvider.ModelName,
			TypeName:     dbProvider.TypeName,
			ProviderMode: dbProvider.ProviderMode,
			DomainOrURL:  dbProvider.DomainOrURL,
			APIKey:       dbProvider.APIKey,
			NoHTTPS:      dbProvider.NoHTTPS,
			DbProvider:   dbProvider, // Set database object directly
		}

		// Use WrapperName as model name for grouping
		modelName := dbProvider.WrapperName
		if modelName == "" {
			modelName = dbProvider.ModelName // If WrapperName is empty, use ModelName
		}

		modelProviders[modelName] = append(modelProviders[modelName], provider)
	}

	// Add providers to config
	for modelName, providers := range modelProviders {
		if len(providers) > 0 {
			log.Infof("Adding %d providers for model %s", len(providers), modelName)

			// Add to Models
			config.Models.models[modelName] = providers

			// Add to Entrypoints
			config.Entrypoints.providers[modelName] = providers

			// Print provider information
			for i, p := range providers {
				log.Infof("  Provider %d: TypeName=%s, ModelName=%s, Domain=%s, HealthStatus=%v",
					i, p.TypeName, p.ModelName, p.DomainOrURL, p.DbProvider.IsHealthy)
			}
		}
	}

	log.Infof("Database AI providers loaded, added %d models in total", len(modelProviders))

	// 2. Load API keys
	err = config.LoadAPIKeysFromDB()
	if err != nil {
		return utils.Errorf("Failed to load API keys from database: %v", err)
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
				return nil // Normal closure
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

// Close closes the balancer and releases resources
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

// GetProviders gets all providers
func (b *Balancer) GetProviders() []*Provider {
	var providers []*Provider

	// Get providers from all models
	for _, modelProviders := range b.config.Models.models {
		providers = append(providers, modelProviders...)
	}

	return providers
}
