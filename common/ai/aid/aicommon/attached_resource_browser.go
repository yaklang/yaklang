package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	AttachedResourceTypeBrowser      = "browser"
	AttachedResourceKeyBrowserDevice = "device_id"

	attachedBrowserCatalogToolName = "browser.capability.catalog"
	attachedBrowserCallToolName    = "browser.capability.call"
	attachedBrowserCryptoToolName  = "browser.crypto.inspect"
	attachedBrowserPrepareToolName = "browser.transform.prepare"
	attachedBrowserHandoffToolName = "browser.handoff.request"
	attachedBrowserHTTPToolName    = "browser.http.test"
	attachedBrowserRodToolName     = "use_browser"
)

type AttachedBrowserResourceData struct {
	DeviceID       string `json:"deviceId"`
	Name           string `json:"name"`
	Reference      string `json:"reference"`
	routingChecked bool
	routingMissing []string
}

func init() {
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeBrowser,
		func() AttachedResourceData { return &AttachedBrowserResourceData{} },
	)
}

func (d *AttachedBrowserResourceData) Type() string {
	return AttachedResourceTypeBrowser
}

func (d *AttachedBrowserResourceData) Unmarshal(raw string) error {
	if err := json.Unmarshal([]byte(raw), d); err != nil {
		return utils.Errorf("parse attached browser instance: %v", err)
	}
	d.DeviceID = strings.TrimSpace(d.DeviceID)
	d.Name = strings.TrimSpace(d.Name)
	d.Reference = strings.TrimSpace(d.Reference)
	if d.DeviceID == "" {
		return utils.Error("attached browser instance has no deviceId")
	}
	if d.Reference == "" {
		d.Reference = d.DeviceID
	}
	if len(d.DeviceID) > 512 || len(d.Name) > 512 || len(d.Reference) > 512 {
		return utils.Error("attached browser instance metadata is too long")
	}
	return nil
}

func (d *AttachedBrowserResourceData) BindLoopData(loop ReActLoopIF) error {
	d.routingChecked = true
	d.routingMissing = nil
	if loop == nil || loop.GetConfig() == nil || loop.GetConfig().GetAiToolManager() == nil {
		d.routingMissing = []string{attachedBrowserCatalogToolName, attachedBrowserCallToolName, attachedBrowserCryptoToolName, attachedBrowserPrepareToolName, attachedBrowserHandoffToolName, attachedBrowserHTTPToolName}
		return nil
	}

	config := loop.GetConfig()
	manager := config.GetAiToolManager()
	for _, name := range []string{attachedBrowserCatalogToolName, attachedBrowserCallToolName, attachedBrowserCryptoToolName, attachedBrowserPrepareToolName, attachedBrowserHandoffToolName, attachedBrowserHTTPToolName} {
		// An explicit @browser attachment is an explicit per-turn tool choice.
		manager.EnableTool(name)
		tool, err := manager.GetToolByName(name)
		if err != nil || tool == nil {
			d.routingMissing = append(d.routingMissing, name)
			continue
		}

		// Attached browser tools are not merely general inventory candidates: the
		// user selected this concrete device for the current turn. Promote all
		// bridge tools into the direct-call prompt so the model does not fall back
		// to the similarly named Rod/use_browser session tool.
		if concreteConfig, ok := config.(*Config); ok {
			concreteConfig.RecordRecentlyUsedTool(tool)
		} else {
			manager.AddRecentlyUsedTool(tool)
		}
	}
	return nil
}

func (d *AttachedBrowserResourceData) ToAttachData(ReActLoopIF) string {
	name := d.Name
	if name == "" {
		name = "Unnamed browser"
	}
	routingStatus := "The required bridge tools are available and have been promoted for direct use."
	if d.routingChecked && len(d.routingMissing) > 0 {
		routingStatus = fmt.Sprintf(
			"Bridge routing is unavailable because these tools are missing: %s. Report that the attached browser bridge is unavailable; do not open a replacement browser.",
			strings.Join(d.routingMissing, ", "),
		)
	}
	return fmt.Sprintf(`## Attached Browser Instance

- Name: %q
- Reference: %q
- Resource semantics: this is an already-open external browser connected through the Yakit browser extension. It is not a Rod-managed browser session.
- Mandatory routing: for every operation on this reference, use only browser.capability.catalog, browser.capability.call, browser.crypto.inspect, browser.transform.prepare, browser.handoff.request, and browser.http.test. When more than one browser is attached, pass this exact Reference as browser_ref; the runtime securely maps it to the attached Device ID. Never generate or pass device_id yourself.
- Stable wrapper parameters: call browser.capability.call directly with the top-level object {"browser_ref":"...","method":"browser.*","params":{...}}. The method and params keys are always required. Do not invoke parameter generation merely to construct this stable wrapper; query browser.capability.catalog only when the needed method or its inner params are genuinely unknown.
- Current-page inspection: call method browser.tabs with params {} through browser.capability.call. Prefer the returned tab with active=true, and use its tab/frame/document target to call browser.context with includeDom=true. Refreshes, navigations, and newly opened HTTP(S) tabs remain part of this paired browser instance; fetch a new page context only when the document changed or a returned node is stale.
- Open a website: call browser.capability.call with method browser.tab.open and params {"url":"https://..."}. This opens a foreground tab inside this exact attached instance; never use use_browser or Eval merely to navigate.
- Page cryptography inspection: after browser.context returns a fresh captureId and visible trigger nodeId, call browser.crypto.inspect once. It atomically triggers that node, handles modal dialogs, captures crypto/encoding/network evidence, and returns postAction with the current document and fresh node references. A captured dialog does not imply navigation, reload, or logout; trust postAction.sameDocument and authentication instead. Continue from postAction without another context call unless it is absent or the selected node is stale. Do not manually orchestrate recording, callable, debugger, or Profile methods for this task.
- Plaintext HTTP testing (domain=transform): when browser.crypto.inspect returns gatewayPreparation.state=ready, call browser.transform.prepare with its candidateId and a complete plaintext request. Pass the returned validationDraft.id to browser.http.test; it encrypts before network I/O and can decrypt the response without permanently saving a Profile. Use a saved profile_id only when the user already confirmed one in the extension. Never call recording/callable/debugger/browser.profile.* yourself, reopen the website because a dialog appeared, or send the same plaintext request through another HTTP tool.
- Login and human verification: when the user asks to scan a QR code, log in, complete MFA/CAPTCHA, or confirm a device, make the relevant page UI visible and then MUST call browser.handoff.request. The tool renders the local handoff card and waits. Never finish the task by merely telling the user to scan or operate in the browser.
- Prohibited fallback: do not use use_browser, generic browser automation, a browser session parameter, op=open, or create a replacement browser. Only do that if the user explicitly asks for a separate automation browser instead of this attached instance.
- Capability names do not change after reconnect. If a method is rejected, query its relevant catalog domain once and use the returned exact name; never invent a renamed method or attribute the mismatch to reconnecting.
- Routing status: %s
- Authority: pairing grants this exact browser instance access to its HTTP(S) tabs. The signed capability catalog, browser/enterprise restrictions, and the AI review policy remain authoritative for every operation.`, name, d.Reference, routingStatus)
}

func attachedBrowserResources(task AITask) ([]*AttachedBrowserResourceData, bool, error) {
	if task == nil {
		return nil, false, nil
	}
	attachedTask, ok := task.(interface{ GetAttachedDatas() []*AttachedResource })
	if !ok {
		return nil, false, nil
	}
	resources := make([]*AttachedBrowserResourceData, 0)
	seen := make(map[string]struct{})
	for _, data := range attachedTask.GetAttachedDatas() {
		if data == nil || !data.HasType(AttachedResourceTypeBrowser) {
			continue
		}
		resource, err := ParseAttachedResourceData(data)
		if err != nil {
			return nil, true, err
		}
		browserResource, ok := resource.(*AttachedBrowserResourceData)
		if !ok || browserResource.DeviceID == "" {
			return nil, true, utils.Error("attached browser resource has no device ID")
		}
		if _, exists := seen[browserResource.DeviceID]; exists {
			continue
		}
		seen[browserResource.DeviceID] = struct{}{}
		resources = append(resources, browserResource)
	}
	return resources, len(resources) > 0, nil
}

func attachedBrowserDeviceID(task AITask, browserRef string) (string, bool, error) {
	resources, attached, err := attachedBrowserResources(task)
	if err != nil || !attached {
		return "", attached, err
	}
	if len(resources) == 1 {
		return resources[0].DeviceID, true, nil
	}
	browserRef = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(browserRef), "@"))
	if browserRef == "" {
		return "", true, utils.Error("multiple browser instances are attached; browser_ref is required")
	}
	var matched *AttachedBrowserResourceData
	for _, resource := range resources {
		if browserRef != resource.DeviceID && !strings.EqualFold(browserRef, resource.Reference) {
			continue
		}
		if matched != nil && matched.DeviceID != resource.DeviceID {
			return "", true, utils.Errorf("browser_ref %q matches more than one attached browser", browserRef)
		}
		matched = resource
	}
	if matched == nil {
		return "", true, utils.Errorf("browser_ref %q does not match an attached browser", browserRef)
	}
	return matched.DeviceID, true, nil
}

func isAttachedBrowserBridgeTool(toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	return toolName == attachedBrowserCatalogToolName || toolName == attachedBrowserCallToolName || toolName == attachedBrowserCryptoToolName || toolName == attachedBrowserPrepareToolName || toolName == attachedBrowserHandoffToolName || toolName == attachedBrowserHTTPToolName
}

func browserExtensionRequested(task AITask) bool {
	statefulTask, ok := task.(interface{ GetUserInput() string })
	if !ok {
		return false
	}
	input := strings.ToLower(statefulTask.GetUserInput())
	for _, marker := range []string{
		"浏览器插件", "浏览器扩展", "yakit-browser-extension", "browser extension", "ytray",
		"实例a", "实例 a", "实例b", "实例 b", "实例c", "实例 c",
		"instance a", "instance b", "instance c",
	} {
		if strings.Contains(input, marker) {
			return true
		}
	}
	return false
}

// CheckAttachedBrowserToolRoute keeps the extension bridge and Rod as separate
// tool domains. An explicit attachment always prevents a Rod fallback.
func CheckAttachedBrowserToolRoute(task AITask, toolName string) (bool, string) {
	toolName = strings.TrimSpace(toolName)
	if toolName != attachedBrowserRodToolName && !isAttachedBrowserBridgeTool(toolName) {
		return true, ""
	}
	resources, attached, err := attachedBrowserResources(task)
	if err != nil {
		return false, err.Error()
	}
	if attached && toolName == attachedBrowserRodToolName {
		references := make([]string, 0, len(resources))
		for _, resource := range resources {
			references = append(references, resource.Reference)
		}
		return false, fmt.Sprintf(
			"tool %q is blocked for this turn because the user attached browser-extension instance(s) %q; use %s, %s, %s, %s, %s, and %s instead. Do not call op=open or create a replacement Rod browser. Remove the browser attachment if a separate automation browser is actually intended.",
			attachedBrowserRodToolName,
			references,
			attachedBrowserCatalogToolName,
			attachedBrowserCallToolName,
			attachedBrowserCryptoToolName,
			attachedBrowserPrepareToolName,
			attachedBrowserHandoffToolName,
			attachedBrowserHTTPToolName,
		)
	}
	if toolName == attachedBrowserRodToolName && browserExtensionRequested(task) {
		return false, fmt.Sprintf(
			"tool %q is blocked because the user explicitly requested an existing browser-extension/YTray instance; use %s, %s, %s, %s, %s, and %s instead. If no extension instance is online, report that fact instead of creating a replacement Rod browser.",
			attachedBrowserRodToolName,
			attachedBrowserCatalogToolName,
			attachedBrowserCallToolName,
			attachedBrowserCryptoToolName,
			attachedBrowserPrepareToolName,
			attachedBrowserHandoffToolName,
			attachedBrowserHTTPToolName,
		)
	}
	return true, ""
}

// BindAttachedBrowserToolParams supplies infrastructure-owned routing data at
// the final ToolCaller boundary. Model output cannot select or override a
// different browser once the user has attached an instance.
func BindAttachedBrowserToolParams(task AITask, toolName string, params aitool.InvokeParams) (aitool.InvokeParams, error) {
	if !isAttachedBrowserBridgeTool(toolName) {
		return params, nil
	}
	if params == nil {
		params = make(aitool.InvokeParams)
	}
	deviceID, attached, err := attachedBrowserDeviceID(task, params.GetString("browser_ref"))
	if err != nil {
		return params, err
	}
	if !attached {
		return params, nil
	}
	delete(params, "browser_ref")
	params["device_id"] = deviceID
	return params, nil
}
