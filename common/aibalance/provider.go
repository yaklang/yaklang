package aibalance

import (
	"errors"
	"sync"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

type Provider struct {
	ModelName   string `yaml:"model_name" json:"model_name"`
	TypeName    string `yaml:"type_name" json:"type_name"`
	DomainOrURL string `yaml:"domain_or_url" json:"domain_or_url"`
	APIKey      string `yaml:"api_key" json:"api_key"`
	NoHTTPS     bool   `yaml:"no_https" json:"no_https"`
	// works for qwen3
	OptionalAllowReason  string `yaml:"optional_allow_reason,omitempty" json:"optional_allow_reason,omitempty"`
	OptionalReasonBudget int    `yaml:"optional_reason_budget,omitempty" json:"optional_reason_budget,omitempty"`

	_cacheLock sync.RWMutex    `yaml:"-" json:"-"`
	_cache     aispec.AIClient `yaml:"-" json:"-"`
}

func (p *Provider) GetAIClient() (aispec.AIClient, error) {
	p._cacheLock.RLock()
	defer p._cacheLock.RUnlock()

	if p._cache != nil {
		return p._cache, nil
	}

	client := ai.GetAI(
		p.ModelName,
		aispec.WithNoHTTPS(p.NoHTTPS),
		aispec.WithAPIKey(p.APIKey),
		aispec.WithBaseURL(p.DomainOrURL),
		aispec.WithType(p.TypeName),
	)
	if utils.IsNil(client) || client == nil {
		return nil, errors.New("failed to get ai client, no such type: " + p.TypeName)
	}
	p._cache = client
	return client, nil
}
