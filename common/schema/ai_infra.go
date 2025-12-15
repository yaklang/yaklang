package schema

import (
	"fmt"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type AIAgentRuntimeType string

const (
	AIAgentRuntimeType_PlanAndExec AIAgentRuntimeType = "plan-exec"
	AIAgentRuntimeType_ReAct       AIAgentRuntimeType = "re-act"
	AIAgentRuntimeType_Unknown     AIAgentRuntimeType = ""
)

type AIAgentRuntime struct {
	gorm.Model

	Uuid              string `json:"uuid" gorm:"unique_index"`
	PersistentSession string `gorm:"index"`
	Name              string `json:"name"`
	Seq               int64  `json:"seq" gorm:"index"`

	TypeName       AIAgentRuntimeType `gorm:"index"`
	QuotedTimeline string             `json:"timeline"`

	QuotedUserInput string `json:"quoted_user_input"`
	ForgeName       string `json:"forge_name"`
}

func (a *AIAgentRuntime) GetTimeline() string {
	if a == nil {
		return ""
	}
	result, err := strconv.Unquote(a.QuotedTimeline)
	if err != nil {
		return a.QuotedTimeline
	}
	return result
}

func (a *AIAgentRuntime) GetUserInput() string {
	return string(codec.StrConvUnquoteForce(a.QuotedUserInput))
}

func (a *AIAgentRuntime) ToGRPC() *ypb.AITask {
	return &ypb.AITask{
		CoordinatorId: a.Uuid,
		Name:          a.Name,
		Seq:           a.Seq,
		UserInput:     a.GetUserInput(),
		ForgeName:     a.ForgeName,
	}
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

	WrapperName  string `json:"wrapper_name" gorm:"index"`
	ModelName    string `json:"model_name" gorm:"index"`
	TypeName     string `json:"type_name" gorm:"index"`
	DomainOrURL  string `json:"domain_or_url" gorm:"index"`
	APIKey       string `json:"api_key" gorm:"index"`
	NoHTTPS      bool   `json:"no_https"`
	ProviderMode string `json:"provider_mode" gorm:"default:'chat'"` // Provider 模式: "chat" 或 "embedding"

	// 可用性指标
	SuccessCount  int64 `json:"success_count"`  // 成功请求总数
	FailureCount  int64 `json:"failure_count"`  // 失败请求总数
	TotalRequests int64 `json:"total_requests"` // 总请求数

	// 最后一次请求信息
	LastRequestTime   time.Time `json:"last_request_time"`   // 最后一次请求时间
	LastRequestStatus bool      `json:"last_request_status"` // 最后一次请求状态 (true=成功, false=失败)
	LastLatency       int64     `json:"last_latency"`        // 最后一次请求延迟 (毫秒)

	// 健康状态
	IsHealthy             bool      `json:"is_healthy"`                                    // 提供者是否健康
	HealthCheckTime       time.Time `json:"health_check_time"`                             // 最后一次健康检查时间
	IsFirstCheckCompleted bool      `json:"is_first_check_completed" gorm:"default:false"` // 首次健康检查是否完成
}

type AiApiKeys struct {
	gorm.Model
	APIKey        string    `json:"api_key" gorm:"index"`
	AllowedModels string    `json:"allowed_models"`
	InputBytes    int64     `json:"input_bytes"`                // 输入字节数统计
	OutputBytes   int64     `json:"output_bytes"`               // 输出字节数统计
	UsageCount    int64     `json:"usage_count"`                // 使用次数统计
	SuccessCount  int64     `json:"success_count"`              // 成功请求数
	FailureCount  int64     `json:"failure_count"`              // 失败请求数
	LastUsedTime  time.Time `json:"last_used_time"`             // 上次使用时间
	Active        bool      `json:"active" gorm:"default:true"` // 新增：API Key 激活状态
}

type LoginSession struct {
	gorm.Model

	SessionID string    `json:"session_id" gorm:"index"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AIMemoryEntity 存储AI记忆条目
type AIMemoryEntity struct {
	gorm.Model

	// 记忆条目唯一标识
	MemoryID string `json:"memory_id" gorm:"unique_index;not null"`

	// 会话ID，关联到特定的AI会话
	SessionID string `json:"session_id" gorm:"index;not null"`

	// 记忆内容
	Content string `json:"content" gorm:"type:text;not null"`

	// 领域标签，用于分类和过滤
	Tags StringArray `json:"tags" gorm:"type:text"`

	// 潜在问题列表，用于RAG检索
	PotentialQuestions StringArray `json:"potential_questions" gorm:"type:text"`

	// C.O.R.E. P.A.C.T. Framework 评分 (0.0-1.0)
	C_Score float64 `json:"c_score"` // Connectivity Score 关联度
	O_Score float64 `json:"o_score"` // Origin Score 来源与确定性
	R_Score float64 `json:"r_score"` // Relevance Score 相关性
	E_Score float64 `json:"e_score"` // Emotion Score 情感
	P_Score float64 `json:"p_score"` // Preference Score 个人偏好
	A_Score float64 `json:"a_score"` // Actionability Score 可操作性
	T_Score float64 `json:"t_score"` // Temporality Score 时效性

	// C.O.R.E. P.A.C.T. 向量，用于快速过滤和排序
	CorePactVector FloatArray `json:"core_pact_vector" gorm:"type:text"`
}

func (a *AIMemoryEntity) TableName() string {
	return "ai_memory_entities_v1"
}

func (a *AIMemoryEntity) BeforeSave() error {
	if a.MemoryID == "" {
		return utils.Errorf("memory_id must be set")
	}
	if a.SessionID == "" {
		return utils.Errorf("session_id must be set")
	}
	if a.Content == "" {
		return utils.Errorf("content must be set")
	}
	return nil
}
