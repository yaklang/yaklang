package aicommon

import (
	"bytes"
	"encoding/json"
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

// timelineItemSerializable 用于序列化的 TimelineItem 结构体
type timelineItemSerializable struct {
	Deleted   bool            `json:"deleted"`
	CreatedAt time.Time       `json:"created_at"`
	Type      string          `json:"type"`
	Value     json.RawMessage `json:"value"`
}

func (item *TimelineItem) GetValue() TimelineItemValue {
	return item.value
}

// MarshalJSON 实现自定义 JSON 序列化
func (item *TimelineItem) MarshalJSON() ([]byte, error) {
	var typeName string
	var valueJSON json.RawMessage

	switch v := item.value.(type) {
	case *aitool.ToolResult:
		typeName = "tool_result"
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		valueJSON = data
	case *UserInteraction:
		typeName = "user_interaction"
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		valueJSON = data
	case *TextTimelineItem:
		typeName = "text"
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		valueJSON = data
	default:
		typeName = "raw"
		valueJSON = []byte(fmt.Sprintf(`"%v"`, v))
	}

	serializable := timelineItemSerializable{
		Deleted:   item.deleted,
		CreatedAt: item.createdAt,
		Type:      typeName,
		Value:     valueJSON,
	}

	return json.Marshal(serializable)
}

// UnmarshalJSON 实现自定义 JSON 反序列化
func (item *TimelineItem) UnmarshalJSON(data []byte) error {
	var serializable timelineItemSerializable
	err := json.Unmarshal(data, &serializable)
	if err != nil {
		return err
	}

	item.deleted = serializable.Deleted
	item.createdAt = serializable.CreatedAt

	switch serializable.Type {
	case "tool_result":
		var toolResult aitool.ToolResult
		err := json.Unmarshal(serializable.Value, &toolResult)
		if err != nil {
			return err
		}
		item.value = &toolResult
	case "user_interaction":
		var userInteraction UserInteraction
		err := json.Unmarshal(serializable.Value, &userInteraction)
		if err != nil {
			return err
		}
		item.value = &userInteraction
	case "text":
		var textItem TextTimelineItem
		err := json.Unmarshal(serializable.Value, &textItem)
		if err != nil {
			return err
		}
		item.value = &textItem
	default:
		// 对于未知类型，尝试作为字符串处理
		item.value = &TextTimelineItem{
			Text: string(serializable.Value),
		}
	}

	return nil
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
