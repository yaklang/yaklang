package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"
)

type EvidenceItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type EvidenceStore struct {
	Items []EvidenceItem `json:"items"`
}

func NewEvidenceStore() *EvidenceStore {
	return &EvidenceStore{Items: make([]EvidenceItem, 0)}
}

func (s *EvidenceStore) IsEmpty() bool {
	return len(s.Items) == 0
}

func (s *EvidenceStore) ApplyOperations(ops []EvidenceOperation) {
	for _, op := range ops {
		id := strings.TrimSpace(op.ID)
		if id == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(op.Op)) {
		case "add":
			content := strings.TrimSpace(op.Content)
			if content == "" {
				continue
			}
			found := false
			for i, item := range s.Items {
				if item.ID == id {
					s.Items[i].Content = content
					found = true
					break
				}
			}
			if !found {
				s.Items = append(s.Items, EvidenceItem{ID: id, Content: content})
			}
		case "update":
			content := strings.TrimSpace(op.Content)
			if content == "" {
				continue
			}
			found := false
			for i, item := range s.Items {
				if item.ID == id {
					s.Items[i].Content = content
					found = true
					break
				}
			}
			if !found {
				s.Items = append(s.Items, EvidenceItem{ID: id, Content: content})
			}
		case "delete":
			for i, item := range s.Items {
				if item.ID == id {
					s.Items = append(s.Items[:i], s.Items[i+1:]...)
					break
				}
			}
		}
	}
}

// Render produces markdown for prompt injection, each item prefixed with [id: xxx].
func (s *EvidenceStore) Render() string {
	if len(s.Items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(s.Items))
	for _, item := range s.Items {
		parts = append(parts, fmt.Sprintf("[id: %s]\n%s", item.ID, strings.TrimSpace(item.Content)))
	}
	return strings.Join(parts, "\n\n")
}

func (s *EvidenceStore) ShrinkToTokenBudget(budget int) {
	for len(s.Items) > 1 {
		rendered := s.Render()
		if MeasureTokens(rendered) <= budget {
			return
		}
		s.Items = s.Items[1:]
	}
}

func (s *EvidenceStore) Marshal() string {
	data, err := json.Marshal(s)
	if err != nil {
		return `{"items":[]}`
	}
	return string(data)
}

func UnmarshalEvidenceStore(data string) *EvidenceStore {
	data = strings.TrimSpace(data)
	if data == "" {
		return NewEvidenceStore()
	}
	store := &EvidenceStore{}
	if err := json.Unmarshal([]byte(data), store); err == nil && len(store.Items) > 0 {
		return store
	}
	return &EvidenceStore{
		Items: []EvidenceItem{{ID: "legacy", Content: data}},
	}
}
