package aihttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handleListAIModels(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	var raw map[string]any
	if err := readJSON(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	req, err := buildListAiModelGRPCRequest(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.GetConfig() == "" {
		writeError(w, http.StatusBadRequest, "config is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.ListAiModel(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "list ai models failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

type aiModelConfigPayload struct {
	Type    string `json:"Type"`
	APIKey  string `json:"api_key,omitempty"`
	Domain  string `json:"domain,omitempty"`
	Proxy   string `json:"proxy,omitempty"`
	NoHttps bool   `json:"no_https,omitempty"`
}

func buildListAiModelGRPCRequest(raw map[string]any) (*ypb.ListAiModelRequest, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("config is required")
	}

	legacyConfig := strings.TrimSpace(pickString(raw, "Config"))
	if legacyConfig != "" {
		if strings.HasPrefix(legacyConfig, "{") {
			return &ypb.ListAiModelRequest{Config: legacyConfig}, nil
		}
		configRaw, err := json.Marshal(aiModelConfigPayload{Type: legacyConfig})
		if err != nil {
			return nil, err
		}
		return &ypb.ListAiModelRequest{Config: string(configRaw)}, nil
	}

	cfg := pickMap(raw, "config", "Config")
	if cfg == nil && hasAny(raw, "Type", "type", "APIKey", "api_key", "Domain", "domain", "ExtraParams", "extra_params") {
		cfg = raw
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	proxy, hasProxy := pickStringWithPresence(cfg, "Proxy", "proxy")
	noHttps, hasNoHttps := pickBool(cfg, "NoHttps", "no_https", "noHttps", "NoHTTPS")
	extraParams := parseExtraParams(pickAny(cfg, "ExtraParams", "extraParams", "extra_params"))

	if !hasProxy {
		if v, ok := extraParams["proxy"]; ok {
			proxy = strings.TrimSpace(v)
		}
	}
	if !hasNoHttps {
		if v, ok := extraParams["no_https"]; ok {
			noHttps, hasNoHttps = parseBoolValue(v)
		} else if v, ok := extraParams["nohttps"]; ok {
			noHttps, hasNoHttps = parseBoolValue(v)
		}
	}

	payload := aiModelConfigPayload{
		Type:   strings.TrimSpace(pickString(cfg, "Type", "type")),
		APIKey: strings.TrimSpace(pickString(cfg, "APIKey", "api_key", "apiKey")),
		Domain: strings.TrimSpace(pickString(cfg, "Domain", "domain")),
		Proxy:  strings.TrimSpace(proxy),
	}
	if payload.Type == "" {
		payload.Type = strings.TrimSpace(extraParams["type"])
	}
	if payload.APIKey == "" {
		payload.APIKey = strings.TrimSpace(extraParams["api_key"])
	}
	if payload.Domain == "" {
		payload.Domain = strings.TrimSpace(extraParams["domain"])
	}
	if payload.Proxy == "" {
		payload.Proxy = strings.TrimSpace(extraParams["proxy"])
	}
	if hasNoHttps {
		payload.NoHttps = noHttps
	}

	if payload.Type == "" {
		return nil, fmt.Errorf("config.type is required")
	}
	configRaw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &ypb.ListAiModelRequest{Config: string(configRaw)}, nil
}

func pickAny(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func pickString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func pickStringWithPresence(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "<nil>" {
			return "", true
		}
		return text, true
	}
	return "", false
}

func pickMap(values map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if row, ok := value.(map[string]any); ok {
				return row
			}
		}
	}
	return nil
}

func parseExtraParams(raw any) map[string]string {
	result := make(map[string]string)
	if raw == nil {
		return result
	}
	switch values := raw.(type) {
	case map[string]any:
		for key, value := range values {
			normKey := strings.ToLower(strings.TrimSpace(key))
			normValue := strings.TrimSpace(fmt.Sprint(value))
			if normKey == "" || normValue == "" || normValue == "<nil>" {
				continue
			}
			result[normKey] = normValue
		}
	case []any:
		for _, item := range values {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(pickString(row, "Key", "key")))
			value := strings.TrimSpace(pickString(row, "Value", "value"))
			if key == "" || value == "" {
				continue
			}
			result[key] = value
		}
	}
	return result
}

func pickBool(values map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		return parseBoolValue(value)
	}
	return false, false
}

func parseBoolValue(raw any) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case float64:
		if value == 1 {
			return true, true
		}
		if value == 0 {
			return false, true
		}
	case int:
		if value == 1 {
			return true, true
		}
		if value == 0 {
			return false, true
		}
	case int64:
		if value == 1 {
			return true, true
		}
		if value == 0 {
			return false, true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "y", "on", "enabled":
			return true, true
		case "0", "false", "no", "n", "off", "disabled":
			return false, true
		}
	}
	return false, false
}

func hasAny(values map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}
