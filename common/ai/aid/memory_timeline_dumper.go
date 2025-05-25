package aid

import (
	"bytes"
	"fmt"
)

type TimelineItemValue interface {
	String() string
	GetID() int64
	GetShrinkResult() string
	GetShrinkSimilarResult() string
	SetShrinkResult(string)
}

var _ TimelineItemValue = (*UserInteraction)(nil)

type UserInteractionStage string

const (
	UserInteractionStage_BeforePlan UserInteractionStage = "before_plan"
	UserInteractionStage_Review     UserInteractionStage = "review"
	UserInteractionStage_FreeInput  UserInteractionStage = "free_input"
)

type UserInteraction struct {
	ID              int64                `json:"id"`
	SystemPrompt    string               `json:"prompt"`
	UserExtraPrompt string               `json:"extra_prompt"`
	Stage           UserInteractionStage `json:"stage"` // Stage
	ShrinkResult    string               `json:"shrink_result,omitempty"`
}

func (u *UserInteraction) String() string {
	if u.Stage == "" {
		u.Stage = UserInteractionStage_FreeInput
	}
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf(" <- [id:%v] when %v\n", u.ID, u.Stage))
	buf.WriteString("   system-question: " + u.SystemPrompt + "\n")
	buf.WriteString("       user-answer: " + u.UserExtraPrompt + "\n")
	return buf.String()
}

func (u *UserInteraction) GetID() int64 {
	return u.ID
}

func (u *UserInteraction) GetShrinkResult() string {
	return u.ShrinkResult
}

func (u *UserInteraction) GetShrinkSimilarResult() string {
	if u.ShrinkResult != "" {
		return u.ShrinkResult
	}
	return ""
}

func (u *UserInteraction) SetShrinkResult(s string) {
	u.ShrinkResult = s
}
