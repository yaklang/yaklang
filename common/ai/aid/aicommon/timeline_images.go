package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// PromptImage binds an image to the actual Timeline item rendered alongside it.
// It is captured under the same Timeline lock as the text, not reconstructed
// from a session-wide resource catalogue after prompt assembly.
type PromptImage struct {
	TimelineItemID int64
	Image          *aimedia.Image
}

func (m *Timeline) PushTextWithImages(id int64, text string, images ...*aimedia.Image) {
	m.pushTextItem(&TextTimelineItem{ID: id, Text: text, Images: images})
}

func (m *Timeline) PushImageObservation(id int64, key, text string) {
	m.pushTextItem(&TextTimelineItem{ID: id, Text: text, ImageObservationKey: key})
}

func (m *Timeline) HasImageObservation(key string) bool {
	if key == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, item := range m.idToTimelineItem.Values() {
		if text, ok := timelineTextItem(item); ok && text.ImageObservationKey == key {
			return true
		}
	}
	return false
}

func timelineItemImages(item *TimelineItem) []PromptImage {
	if item == nil || item.deleted || item.value == nil ||
		item.value.GetShrinkResult() != "" || item.value.GetShrinkSimilarResult() != "" {
		return nil
	}
	var images []*aimedia.Image
	switch value := item.value.(type) {
	case *TextTimelineItem:
		images = value.Images
	case *aitool.ToolResult:
		images = value.Images
	}
	var out []PromptImage
	for _, img := range images {
		if img != nil {
			copy := *img
			out = append(out, PromptImage{TimelineItemID: item.GetID(), Image: &copy})
		}
	}
	return out
}

func timelineBlockImages(blocks TimelineRenderableBlocks) []PromptImage {
	var out []PromptImage
	for _, block := range blocks {
		if interval, ok := block.(*TimelineIntervalBlock); ok {
			for _, item := range interval.Items {
				out = append(out, timelineItemImages(item)...)
			}
		}
	}
	return out
}
