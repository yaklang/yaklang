package aispec

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ModelContextIdentity identifies a concrete configured service, not a provider
// adapter or a model display name. Credentials and tenant headers only enter the
// digest; they must never appear in context events or storage keys in plaintext.
func ModelContextIdentity(model *ypb.AIModelConfig, extraOpts ...AIConfigOption) string {
	if model == nil || model.GetProvider() == nil {
		return ""
	}
	opts := append(BuildOptionsFromConfig(model), extraOpts...)
	c := NewDefaultAIConfig(opts...)
	if strings.TrimSpace(c.Type) == "" || strings.TrimSpace(c.Model) == "" {
		return ""
	}
	headers := make(map[string]string)
	for _, h := range c.Headers {
		headers[strings.ToLower(strings.TrimSpace(h.GetKey()))] = h.GetValue()
	}
	apiType := c.APIType
	if apiType == "" {
		apiType = "chat_completions"
	}
	endpoint := ""
	if c.EnableEndpoint {
		endpoint = strings.TrimSpace(c.Endpoint)
	}
	raw, _ := json.Marshal(struct {
		Type, Model, Domain, BaseURL, Endpoint, APIType, APIKey, Script string
		NoHTTPS                                                         bool
		Headers                                                         map[string]string
	}{strings.TrimSpace(c.Type), strings.TrimSpace(c.Model), strings.TrimSpace(c.Domain),
		strings.TrimRight(strings.TrimSpace(c.BaseURL), "/"), endpoint, apiType, c.APIKey,
		c.Script, c.NoHttps, headers})
	return fmt.Sprintf("v2:%x", sha256.Sum256(raw))
}
