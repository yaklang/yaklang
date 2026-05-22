package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type EvidenceItem struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	CreatedUnix int64  `json:"created_unix,omitempty"`
	UpdatedUnix int64  `json:"updated_unix,omitempty"`
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
	s.ApplyOperationsAt(ops, time.Now().Unix())
}

func (s *EvidenceStore) ApplyOperationsAt(ops []EvidenceOperation, nowUnix int64) {
	if nowUnix <= 0 {
		nowUnix = time.Now().Unix()
	}
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
					s.Items[i].touch(nowUnix)
					found = true
					break
				}
			}
			if !found {
				s.Items = append(s.Items, newEvidenceItem(id, content, nowUnix))
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
					s.Items[i].touch(nowUnix)
					found = true
					break
				}
			}
			if !found {
				s.Items = append(s.Items, newEvidenceItem(id, content, nowUnix))
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
	return renderEvidenceItems(s.Items)
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
	if err := json.Unmarshal([]byte(data), store); err == nil {
		if store.Items == nil {
			store.Items = make([]EvidenceItem, 0)
		}
		for i := range store.Items {
			normalizeEvidenceItemTimestamps(&store.Items[i])
		}
		return store
	}
	return &EvidenceStore{
		Items: []EvidenceItem{newLegacyEvidenceItem(data)},
	}
}

func (s *EvidenceStore) SnapshotItems() []EvidenceItem {
	if s == nil || len(s.Items) == 0 {
		return nil
	}
	items := make([]EvidenceItem, len(s.Items))
	copy(items, s.Items)
	return items
}

func renderEvidenceItems(items []EvidenceItem) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		content := strings.TrimSpace(item.Content)
		if id == "" || content == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("[id: %s]\n%s", id, content))
	}
	return strings.Join(parts, "\n\n")
}

func newEvidenceItem(id, content string, ts int64) EvidenceItem {
	item := EvidenceItem{
		ID:      strings.TrimSpace(id),
		Content: strings.TrimSpace(content),
	}
	item.touch(ts)
	if item.CreatedUnix <= 0 {
		item.CreatedUnix = item.UpdatedUnix
	}
	return item
}

func newLegacyEvidenceItem(content string) EvidenceItem {
	item := EvidenceItem{
		ID:      "legacy",
		Content: strings.TrimSpace(content),
	}
	normalizeEvidenceItemTimestamps(&item)
	return item
}

func normalizeEvidenceItemTimestamps(item *EvidenceItem) {
	if item == nil {
		return
	}
	if item.CreatedUnix <= 0 && item.UpdatedUnix > 0 {
		item.CreatedUnix = item.UpdatedUnix
	}
	if item.UpdatedUnix <= 0 && item.CreatedUnix > 0 {
		item.UpdatedUnix = item.CreatedUnix
	}
	if item.CreatedUnix <= 0 && item.UpdatedUnix <= 0 {
		item.CreatedUnix = 1
		item.UpdatedUnix = 1
	}
}

func (i *EvidenceItem) touch(ts int64) {
	if i == nil {
		return
	}
	if ts <= 0 {
		ts = time.Now().Unix()
	}
	if i.CreatedUnix <= 0 {
		i.CreatedUnix = ts
	}
	i.UpdatedUnix = ts
}

func (i EvidenceItem) EffectiveUpdatedUnix() int64 {
	if i.UpdatedUnix > 0 {
		return i.UpdatedUnix
	}
	if i.CreatedUnix > 0 {
		return i.CreatedUnix
	}
	return 1
}
