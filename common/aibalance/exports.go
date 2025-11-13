package aibalance

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var Exports = map[string]interface{}{
	"Start": Start,

	// Options
	"host":          withHost,
	"port":          withPort,
	"adminPassword": withAdminPassword,
	"config":        withConfig,
}

type startOptions struct {
	host          string
	port          int
	adminPassword string
	configFile    string
}

type StartOption func(*startOptions)

func withHost(host string) StartOption {
	return func(o *startOptions) {
		o.host = host
	}
}

func withPort(port int) StartOption {
	return func(o *startOptions) {
		o.port = port
	}
}

func withAdminPassword(password string) StartOption {
	return func(o *startOptions) {
		o.adminPassword = password
	}
}

func withConfig(configFile string) StartOption {
	return func(o *startOptions) {
		o.configFile = configFile
	}
}

// Start starts the AI balance server with options
func Start(opts ...StartOption) error {
	options := &startOptions{
		host:          "127.0.0.1",
		port:          8223,
		adminPassword: "",
		configFile:    "",
	}

	for _, opt := range opts {
		opt(options)
	}

	listenAddr := fmt.Sprintf("%s:%d", options.host, options.port)
	log.Infof("Starting aibalance server on %s", listenAddr)

	// Create balancer from config file (or empty config if file doesn't exist)
	b, err := NewBalancer(options.configFile)
	if err != nil {
		log.Errorf("Failed to create aibalance balancer: %v", err)
		return utils.Errorf("failed to create aibalance balancer: %v", err)
	}

	// Set admin password if provided
	if options.adminPassword != "" {
		b.config.AdminPassword = options.adminPassword
		log.Infof("Admin password set")
	}

	// Start the server
	err = b.RunWithAddr(listenAddr)
	if err != nil {
		log.Errorf("Failed to start aibalance server: %v", err)
		return utils.Errorf("failed to start aibalance server: %v", err)
	}

	return nil
}
