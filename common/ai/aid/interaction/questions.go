// Package interaction defines the data-only contract shared by conversational
// questions and generated interfaces. It does not execute model-provided code.
package interaction

import (
	"fmt"
	"strings"
)

const Protocol = "memfit.ui/1"

type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

type Question struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Prompt        string   `json:"prompt"`
	Multiple      bool     `json:"multiple"`
	AllowFreeText bool     `json:"allowFreeText"`
	Options       []Option `json:"options"`
}

type QuestionRequest struct {
	ID        string     `json:"id"`
	TaskID    string     `json:"taskId"`
	Mode      string     `json:"mode"`
	Questions []Question `json:"questions"`
	Deadline  int64      `json:"deadline,omitempty"` // Unix milliseconds; absent means no timeout.
	State     string     `json:"state"`
	Touched   bool       `json:"touched"`
}

type Answer struct {
	QuestionID string   `json:"questionId"`
	Selected   []string `json:"selected"`
	Text       string   `json:"text"`
	Skipped    bool     `json:"skipped,omitempty"`
}

// Submission is metadata on an ordinary user message. Text is rebuilt from the
// authoritative question/form on the server, never from client-supplied labels.
type Submission struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	RequestID string         `json:"requestId"`
	Answers   []Answer       `json:"answers,omitempty"`
	NodeID    string         `json:"nodeId,omitempty"`
	Values    map[string]any `json:"values,omitempty"`
}

func (r *QuestionRequest) Validate() error {
	if len(r.Questions) < 1 || len(r.Questions) > 3 {
		return fmt.Errorf("questions must contain 1 to 3 questions")
	}
	ids := map[string]bool{}
	for i := range r.Questions {
		q := &r.Questions[i]
		if q.ID == "" || ids[q.ID] || strings.TrimSpace(q.Title) == "" || strings.TrimSpace(q.Prompt) == "" {
			return fmt.Errorf("question %d requires a unique id, title and prompt", i+1)
		}
		ids[q.ID] = true
		if len(q.Options) < 2 || len(q.Options) > 10 {
			return fmt.Errorf("question %s must have 2 to 10 options", q.ID)
		}
		optionIDs := map[string]bool{}
		recommended := -1
		for j, o := range q.Options {
			if o.ID == "" || optionIDs[o.ID] || strings.TrimSpace(o.Label) == "" {
				return fmt.Errorf("question %s has an invalid or duplicate option", q.ID)
			}
			optionIDs[o.ID] = true
			if o.Recommended {
				if recommended >= 0 {
					return fmt.Errorf("question %s has multiple recommended options", q.ID)
				}
				recommended = j
			}
		}
		if recommended > 0 {
			o := q.Options[recommended]
			copy(q.Options[1:recommended+1], q.Options[:recommended])
			q.Options[0] = o
		}
	}
	return nil
}

func (r QuestionRequest) AnswerText(answers []Answer) (string, error) {
	byID := map[string]Answer{}
	questions := map[string]bool{}
	for _, q := range r.Questions {
		questions[q.ID] = true
	}
	for _, a := range answers {
		if _, exists := byID[a.QuestionID]; exists || !questions[a.QuestionID] {
			return "", fmt.Errorf("unknown or duplicate question %q", a.QuestionID)
		}
		byID[a.QuestionID] = a
	}
	var lines []string
	answered := false
	for i, q := range r.Questions {
		a := byID[q.ID]
		if !q.AllowFreeText && strings.TrimSpace(a.Text) != "" {
			return "", fmt.Errorf("question %s does not allow free text", q.ID)
		}
		if !q.Multiple && len(a.Selected) > 1 {
			return "", fmt.Errorf("question %s is single choice", q.ID)
		}
		if a.Skipped && (len(a.Selected) > 0 || strings.TrimSpace(a.Text) != "") {
			return "", fmt.Errorf("skipped question %s cannot contain an answer", q.ID)
		}
		options := map[string]string{}
		for _, o := range q.Options {
			options[o.ID] = o.Label
		}
		var parts []string
		seen := map[string]bool{}
		for _, id := range a.Selected {
			label, ok := options[id]
			if !ok || seen[id] {
				return "", fmt.Errorf("invalid option %q for question %s", id, q.ID)
			}
			seen[id] = true
			parts = append(parts, label)
		}
		if text := strings.TrimSpace(a.Text); text != "" {
			parts = append(parts, text)
		}
		value := "未选择"
		if len(parts) > 0 {
			answered = true
			value = strings.Join(parts, "；")
		}
		lines = append(lines, fmt.Sprintf("%d. %s（%s）：%s", i+1, q.Title, q.Prompt, value))
	}
	if !answered {
		return "", nil
	}
	return "对追问的回答：\n" + strings.Join(lines, "\n"), nil
}

// QuestionSchema is also used in the model's action definition.
func QuestionSchema() map[string]any {
	return map[string]any{
		"type": "array", "minItems": 1, "maxItems": 3,
		"items": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"id", "title", "prompt", "multiple", "allowFreeText", "options"},
			"properties": map[string]any{
				"id": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"},
				"prompt": map[string]any{"type": "string"}, "multiple": map[string]any{"type": "boolean"},
				"allowFreeText": map[string]any{"type": "boolean", "description": "Whether the user may answer in their own words."},
				"options": map[string]any{
					"type": "array", "minItems": 2, "maxItems": 10,
					"items": map[string]any{
						"type": "object", "additionalProperties": false, "required": []string{"id", "label"},
						"properties": map[string]any{
							"id": map[string]any{"type": "string"}, "label": map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"}, "recommended": map[string]any{"type": "boolean"},
						},
					},
				},
			},
		},
	}
}
