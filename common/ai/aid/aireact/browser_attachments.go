package aireact

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

func cloneBrowserBindings(bindings []aicommon.AttachedBrowserBinding) []aicommon.AttachedBrowserBinding {
	if len(bindings) == 0 {
		return nil
	}
	return append([]aicommon.AttachedBrowserBinding(nil), bindings...)
}

// parseAttachedBrowserBindings validates and deduplicates browser resources.
// The bool reports whether this turn explicitly supplied any browser resource,
// even if all of them were malformed.
func parseAttachedBrowserBindings(resources []*aicommon.AttachedResource) ([]aicommon.AttachedBrowserBinding, bool) {
	explicit := false
	bindings := make([]aicommon.AttachedBrowserBinding, 0)
	seen := make(map[string]struct{})
	for _, resource := range resources {
		if resource == nil || !resource.HasType(aicommon.AttachedResourceTypeBrowser) {
			continue
		}
		explicit = true
		parsed, err := aicommon.ParseAttachedResourceData(resource)
		if err != nil {
			log.Warnf("attached browser resource: failed to parse value=%q: %v", resource.Value, err)
			continue
		}
		browser, ok := parsed.(*aicommon.AttachedBrowserResourceData)
		if !ok || browser == nil {
			continue
		}
		binding := browser.Binding()
		if _, exists := seen[binding.DeviceID]; exists {
			continue
		}
		seen[binding.DeviceID] = struct{}{}
		bindings = append(bindings, binding)
	}
	return bindings, explicit
}

// bindBrowserAttachmentsToTask updates the latest session selection and stores
// the effective selection on the task. Running tasks keep their own snapshot;
// queued tasks see the selection that existed when they were created.
func (r *ReAct) bindBrowserAttachmentsToTask(task aicommon.AIStatefulTask, resources []*aicommon.AttachedResource) {
	if r == nil || task == nil {
		return
	}
	bindings, explicit := parseAttachedBrowserBindings(resources)

	r.browserAttachmentsMu.Lock()
	if explicit {
		r.selectedBrowserBindings = cloneBrowserBindings(bindings)
	}
	snapshot := cloneBrowserBindings(r.selectedBrowserBindings)
	r.browserAttachmentsMu.Unlock()

	setTaskBrowserBindings(task, snapshot)
}

func setTaskBrowserBindings(task aicommon.AIStatefulTask, bindings []aicommon.AttachedBrowserBinding) {
	if carrier, ok := task.(interface {
		SetAttachedBrowserBindings([]aicommon.AttachedBrowserBinding)
	}); ok {
		carrier.SetAttachedBrowserBindings(bindings)
	}
}

func getTaskBrowserBindings(task aicommon.AIStatefulTask) []aicommon.AttachedBrowserBinding {
	if carrier, ok := task.(interface {
		GetAttachedBrowserBindings() []aicommon.AttachedBrowserBinding
	}); ok {
		return carrier.GetAttachedBrowserBindings()
	}
	return nil
}

// prepareBrowserAttachmentForTask is the single task-start integration point
// for browser attachments. It activates the task snapshot, promotes the bridge
// tools once per task, and appends one concise timeline introduction.
func (r *ReAct) prepareBrowserAttachmentForTask(task aicommon.AIStatefulTask) {
	if r == nil || task == nil {
		return
	}
	if carrier, ok := task.(interface {
		GetAttachedBrowserBindings() []aicommon.AttachedBrowserBinding
	}); ok {
		r.activateBrowserAttachments(carrier.GetAttachedBrowserBindings())
	}

	bindings := r.GetActiveBrowserAttachments()
	if len(bindings) == 0 {
		return
	}
	missing := r.promoteAttachedBrowserTools()
	r.AddToTimeline("attached_browser", renderAttachedBrowserIntroduction(bindings, missing))
}

// activateBrowserAttachmentForResumedTask restores the task snapshot for
// execution paths that bypass the normal ReAct-loop startup. The attachment
// introduction was already written before the handoff and must not be repeated.
func (r *ReAct) activateBrowserAttachmentForResumedTask(task aicommon.AIStatefulTask) {
	if r == nil || task == nil {
		return
	}
	r.activateBrowserAttachments(getTaskBrowserBindings(task))
	if len(r.GetActiveBrowserAttachments()) == 0 {
		return
	}
	r.promoteAttachedBrowserTools()
}

func (r *ReAct) activateBrowserAttachments(bindings []aicommon.AttachedBrowserBinding) {
	if r == nil {
		return
	}
	r.browserAttachmentsMu.Lock()
	r.activeBrowserBindings = cloneBrowserBindings(bindings)
	r.browserAttachmentsMu.Unlock()
}

// GetActiveBrowserAttachments implements aicommon.BrowserAttachmentProvider.
func (r *ReAct) GetActiveBrowserAttachments() []aicommon.AttachedBrowserBinding {
	if r == nil {
		return nil
	}
	r.browserAttachmentsMu.RLock()
	defer r.browserAttachmentsMu.RUnlock()
	return cloneBrowserBindings(r.activeBrowserBindings)
}

func (r *ReAct) promoteAttachedBrowserTools() []string {
	if r == nil || r.config == nil || r.config.GetAiToolManager() == nil {
		return aicommon.AttachedBrowserBridgeToolNames()
	}
	manager := r.config.GetAiToolManager()
	missing := make([]string, 0)
	for _, name := range aicommon.AttachedBrowserBridgeToolNames() {
		manager.EnableTool(name)
		tool, err := manager.GetToolByName(name)
		if err != nil || tool == nil {
			missing = append(missing, name)
			continue
		}
		if !manager.IsRecentlyUsedTool(name) {
			r.config.RecordRecentlyUsedTool(tool)
		}
	}
	return missing
}

func renderAttachedBrowserIntroduction(bindings []aicommon.AttachedBrowserBinding, missing []string) string {
	var b strings.Builder
	b.WriteString("## Attached Browser\n\n")
	b.WriteString("- The user selected browser instance")
	if len(bindings) > 1 {
		b.WriteString("s")
	}
	b.WriteString(": ")
	for index, binding := range bindings {
		if index > 0 {
			b.WriteString(", ")
		}
		name := strings.TrimSpace(binding.Name)
		if name == "" {
			name = "Unnamed browser"
		}
		b.WriteString(fmt.Sprintf("`%s` (%s)", binding.Reference, name))
	}
	b.WriteString("\n")
	b.WriteString("- This is an already-open browser connected through the Yakit browser extension, not a Rod-managed browser session.\n")
	b.WriteString("- Browser bridge calls are bound to the selected instance by the runtime. Never generate or pass `device_id` yourself.\n")
	if len(bindings) > 1 {
		b.WriteString("- Multiple browsers are selected. Pass the exact `browser_ref` for singular operations; split requests by browser identity.\n")
	}
	b.WriteString("- Use the browser bridge tools for page, tab, crypto, transform, handoff, and browser-backed HTTP work. Do not use `use_browser`, `op=open`, or create a replacement browser.\n")
	b.WriteString("- For ordinary request replay or authorization testing, use `do_http_request`; the runtime binds and labels the selected browser identity.\n")
	if len(missing) > 0 {
		b.WriteString(fmt.Sprintf("- Browser bridge is unavailable because these tools are missing: %s. Report that fact instead of falling back to Rod.\n", strings.Join(missing, ", ")))
	} else {
		b.WriteString("- The required browser bridge tools are available and have been promoted for direct use.\n")
	}
	return strings.TrimSpace(b.String())
}
