package schema

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type AiCoordinatorRuntime struct {
	gorm.Model

	Uuid            string `json:"uuid" gorm:"unique_index"`
	Name            string `json:"name"`
	Seq             int64  `json:"seq" gorm:"index"`
	QuotedUserInput string `json:"quoted_user_input"`
}

func (a *AiCoordinatorRuntime) GetUserInput() string {
	return string(codec.StrConvUnquoteForce(a.QuotedUserInput))
}

type AiCheckpointType string

const (
	AiCheckpointType_AIInteractive AiCheckpointType = "ai-request"
	AiCheckpointType_ToolCall      AiCheckpointType = "tool-call"
	AiCheckpointType_Review        AiCheckpointType = "review"
)

type AiCheckpoint struct {
	gorm.Model

	CoordinatorUuid    string           `json:"coordinator_uuid" gorm:"index"`
	Seq                int64            `json:"seq" gorm:"index"`
	Type               AiCheckpointType `json:"type" gorm:"index"`
	RequestQuotedJson  string           `json:"request_quoted_json"`
	ResponseQuotedJson string           `json:"response_quoted_json"`
	Finished           bool             `json:"finished"`

	Hash string `json:"hash" gorm:"unique_index"`
}

func (c *AiCheckpoint) CalcHash() string {
	return utils.CalcSha256(c.CoordinatorUuid, c.Seq, c.Type)
}

func (c *AiCheckpoint) BeforeSave() error {
	if c.Hash == "" {
		c.Hash = c.CalcHash()
	}

	switch c.Type {
	case AiCheckpointType_AIInteractive, AiCheckpointType_ToolCall, AiCheckpointType_Review:
		break
	default:
		return fmt.Errorf("invalid checkpoint type: %s", c.Type)
	}

	if c.Seq <= 0 {
		return fmt.Errorf("seq must be greater than 0")
	}

	if c.CoordinatorUuid == "" {
		return fmt.Errorf("coordinator_uuid must be set")
	}

	return nil
}

type AiProvider struct {
	gorm.Model

	WrapperName string `json:"wrapper_name" gorm:"index"`
	ModelName   string `json:"model_name" gorm:"index"`
	TypeName    string `json:"type_name" gorm:"index"`
	DomainOrURL string `json:"domain_or_url" gorm:"index"`
	APIKey      string `json:"api_key" gorm:"index"`
	NoHTTPS     bool   `json:"no_https"`

	// 可用性指标
	SuccessCount  int64 `json:"success_count"`  // 成功请求总数
	FailureCount  int64 `json:"failure_count"`  // 失败请求总数
	TotalRequests int64 `json:"total_requests"` // 总请求数

	// 最后一次请求信息
	LastRequestTime   time.Time `json:"last_request_time"`   // 最后一次请求时间
	LastRequestStatus bool      `json:"last_request_status"` // 最后一次请求状态 (true=成功, false=失败)
	LastLatency       int64     `json:"last_latency"`        // 最后一次请求延迟 (毫秒)

	// 健康状态
	IsHealthy       bool      `json:"is_healthy"`        // 提供者是否健康
	HealthCheckTime time.Time `json:"health_check_time"` // 最后一次健康检查时间
}
