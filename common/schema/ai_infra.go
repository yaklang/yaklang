package schema

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type AiCoordinatorRuntime struct {
	gorm.Model

	Uuid string `json:"uuid" gorm:"unique_index"`
	Name string `json:"name"`
	Seq  int64  `json:"seq" gorm:"unique_index"`
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
