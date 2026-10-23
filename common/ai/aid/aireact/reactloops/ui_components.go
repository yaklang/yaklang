package reactloops

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
)

type uiComponentView struct {
	catalogHash [32]byte
	taskID      string
	content     string
}

func (r *ReActLoop) uiCatalog() interaction.Catalog {
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
		return cfg.UICatalog()
	}
	return interaction.Catalog{}
}

func uiCatalogHash(catalog interaction.Catalog) [32]byte {
	raw, _ := json.Marshal(catalog)
	return sha256.Sum256(raw)
}

// LoadUIComponents replaces the task's active schema batch, instead of appending
// definitions to Timeline/model replay. Feedback contains only an acknowledgement;
// the real, unmodified schemas are projected once in the dynamic prompt section.
func (r *ReActLoop) LoadUIComponents(names []string) (map[string]any, error) {
	taskID := ""
	if task := r.GetCurrentTask(); task != nil {
		if err := task.GetContext().Err(); err != nil {
			return nil, err
		}
		taskID = task.GetId()
	}
	catalog := r.uiCatalog()
	if len(catalog.Components) == 0 {
		return nil, fmt.Errorf("client UI unavailable; use plain text")
	}
	content, selected, err := catalog.LoadPromptDefinitions(names)
	if err != nil {
		return nil, err
	}
	r.uiComponentsMu.Lock()
	r.uiComponents = &uiComponentView{catalogHash: uiCatalogHash(catalog), taskID: taskID, content: content}
	r.uiComponentsMu.Unlock()
	return map[string]any{"status": "loaded", "names": selected,
		"instruction": "UI_COMPONENT_DEFINITIONS now contains exactly this batch. Reuse it for render_ui; include every needed component when replacing it. names:[] clears it. No UI was rendered and no user input is awaited."}, nil
}

func (r *ReActLoop) SearchUIComponents(query string, offset int) (interaction.UIComponentSearchResult, error) {
	return r.uiCatalog().Search(query, offset)
}

func (r *ReActLoop) clearUIComponents() {
	r.uiComponentsMu.Lock()
	r.uiComponents = nil
	r.uiComponentsMu.Unlock()
}

func (r *ReActLoop) renderUIComponents() string {
	r.uiComponentsMu.Lock()
	loaded := r.uiComponents != nil
	r.uiComponentsMu.Unlock()
	if !loaded {
		return ""
	}
	catalog := r.uiCatalog()
	task := r.GetCurrentTask()
	taskID := ""
	if task != nil {
		taskID = task.GetId()
	}
	r.uiComponentsMu.Lock()
	defer r.uiComponentsMu.Unlock()
	if r.uiComponents == nil {
		return ""
	}
	// Version and schema content changes all invalidate the view.
	if r.uiComponents.taskID != taskID || r.uiComponents.catalogHash != uiCatalogHash(catalog) || (task != nil && task.GetContext().Err() != nil) {
		r.uiComponents = nil
		return ""
	}
	return r.uiComponents.content
}
