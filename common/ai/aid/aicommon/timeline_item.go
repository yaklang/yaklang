package aicommon

import (
	"bytes"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type TimelineItemValue interface {
	String() string
	GetID() int64
	GetShrinkResult() string
	GetShrinkSimilarResult() string
	SetShrinkResult(string)
}

type TimelineItem struct {
	deleted   bool
	createdAt time.Time

	value TimelineItemValue // *aitool.ToolResult
}

func (item *TimelineItem) GetValue() TimelineItemValue {
	return item.value
}

func (item *TimelineItem) IsDeleted() bool {
	return item.deleted
}

func (item *TimelineItem) GetShrinkResult() string {
	return item.value.GetShrinkResult()
}

func (item *TimelineItem) GetShrinkSimilarResult() string {
	return item.value.GetShrinkSimilarResult()
}

func (item *TimelineItem) String() string {
	return item.value.String()
}

func (item *TimelineItem) SetShrinkResult(pers string) {
	item.value.SetShrinkResult(pers)
}

func (item *TimelineItem) ToTimelineItemOutput() *TimelineItemOutput {
	var typeName string
	switch item.value.(type) {
	case *aitool.ToolResult:
		typeName = "tool_result"
	case *UserInteraction:
		typeName = "user_interaction"
	case *TextTimelineItem:
		typeName = "text"
	default:
		typeName = "raw"
	}
	return &TimelineItemOutput{
		Timestamp: item.createdAt,
		Type:      typeName,
		Content:   item.String(),
	}
}

func (item *TimelineItem) GetID() int64 {
	if item.value == nil {
		return 0
	}
	return item.value.GetID()
}

var _ TimelineItemValue = (*TimelineItem)(nil)
var _ TimelineItemValue = (*UserInteraction)(nil)
var _ TimelineItemValue = (*TextTimelineItem)(nil)

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

type TextTimelineItem struct {
	ID                  int64  `json:"id"`
	Text                string `json:"text"`
	ShrinkResult        string `json:"shrink_result,omitempty"`
	ShrinkSimilarResult string `json:"shrink_similar_result,omitempty"`
}

func (t *TextTimelineItem) String() string {
	return t.Text
}

func (t *TextTimelineItem) GetID() int64 {
	return t.ID
}

func (t *TextTimelineItem) GetShrinkResult() string {
	return t.ShrinkResult
}

func (t *TextTimelineItem) GetShrinkSimilarResult() string {
	return t.ShrinkSimilarResult
}

func (t *TextTimelineItem) SetShrinkResult(s string) {
	t.ShrinkResult = s
}
