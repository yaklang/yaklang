package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yaklang/yaklang/common/browser"
)

const defaultEndpoint = "ws://127.0.0.1:64333/extension"

func configuredEndpoint() string {
	if endpoint := strings.TrimSpace(os.Getenv("YAKIT_BROWSER_AGENT_ENDPOINT")); endpoint != "" {
		return endpoint
	}
	configPath := strings.TrimSpace(os.Getenv("YAKIT_BROWSER_AGENT_CONFIG"))
	if configPath == "" {
		if configRoot, err := os.UserConfigDir(); err == nil {
			configPath = configRoot + string(os.PathSeparator) + "yakit" + string(os.PathSeparator) + "browser-agent-native-host.json"
		}
	}
	var config struct {
		Endpoint string `json:"endpoint"`
	}
	if payload, err := os.ReadFile(configPath); err == nil && json.Unmarshal(payload, &config) == nil && strings.TrimSpace(config.Endpoint) != "" {
		return strings.TrimSpace(config.Endpoint)
	}
	return defaultEndpoint
}

func extensionOrigin(args []string) string {
	for _, argument := range args {
		if origin, ok := browser.NormalizeBrowserExtensionOrigin(argument); ok {
			return origin
		}
	}
	if len(args) >= 2 {
		manifestPath := strings.TrimSpace(args[0])
		extensionID := strings.TrimSpace(args[1])
		var manifest struct {
			AllowedExtensions []string `json:"allowed_extensions"`
		}
		payload, err := os.ReadFile(manifestPath)
		if err != nil || json.Unmarshal(payload, &manifest) != nil {
			return ""
		}
		for _, allowed := range manifest.AllowedExtensions {
			if extensionID != "" && extensionID == allowed {
				digest := sha256.Sum256([]byte(extensionID))
				return "moz-extension://" + hex.EncodeToString(digest[:])
			}
		}
	}
	return ""
}

func main() {
	endpoint := configuredEndpoint()
	origin := extensionOrigin(os.Args[1:])
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := browser.RunNativeMessagingProxy(ctx, os.Stdin, os.Stdout, browser.NativeMessagingProxyOptions{
		Endpoint: endpoint,
		Origin:   origin,
	}); err != nil && err != context.Canceled {
		_, _ = fmt.Fprintf(os.Stderr, "yakit browser native host: %v\n", err)
		os.Exit(1)
	}
}
