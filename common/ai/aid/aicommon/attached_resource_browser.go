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

var attachedBrowserBridgeToolNames = []string{
	attachedBrowserCatalogToolName,
	attachedBrowserCallToolName,
	attachedBrowserCryptoToolName,
	attachedBrowserPrepareToolName,
	attachedBrowserHandoffToolName,
	attachedBrowserHTTPToolName,
}

// AttachedBrowserBridgeToolNames returns the browser-extension tools that are
// promoted when a browser instance is bound to the ReAct session.
func AttachedBrowserBridgeToolNames() []string {
	return append([]string(nil), attachedBrowserBridgeToolNames...)
}

type AttachedBrowserResourceData struct {
	DeviceID  string `json:"deviceId"`
	Name      string `json:"name"`
	Reference string `json:"reference"`
}

// AttachedBrowserBinding is the runtime-owned browser selection. DeviceID is
// intentionally never rendered into prompts or exposed to the model.
type AttachedBrowserBinding struct {
	DeviceID  string
	Name      string
	Reference string
}

// BrowserAttachmentProvider exposes the current ReAct session selection.
type BrowserAttachmentProvider interface {
	GetBrowserAttachments() []AttachedBrowserBinding
}

type BrowserAttachmentSession interface {
	BrowserAttachmentProvider
	SetBrowserAttachments([]AttachedBrowserBinding)
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

func (d *AttachedBrowserResourceData) Unmarshal(resource *AttachedResource) error {
	if err := json.Unmarshal([]byte(resource.Value), d); err != nil {
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
	bindAttachedBrowserResources(loop, []AttachedResourceData{d})
	return nil
}

func (d *AttachedBrowserResourceData) ToAttachData(loop ReActLoopIF) string {
	return renderAttachedBrowserIntroduction(
		browserBindingsFromAttachedResources([]AttachedResourceData{d}),
		missingAttachedBrowserTools(loopConfig(loop)),
	)
}

func (d *AttachedBrowserResourceData) BindLoopDataBatch(loop ReActLoopIF, resources []AttachedResourceData) error {
	bindAttachedBrowserResources(loop, resources)
	return nil
}

func (d *AttachedBrowserResourceData) ToAttachDataBatch(loop ReActLoopIF, resources []AttachedResourceData) string {
	return renderAttachedBrowserIntroduction(
		browserBindingsFromAttachedResources(resources),
		missingAttachedBrowserTools(loopConfig(loop)),
	)
}

func (d *AttachedBrowserResourceData) Binding() AttachedBrowserBinding {
	return AttachedBrowserBinding{
		DeviceID:  d.DeviceID,
		Name:      d.Name,
		Reference: d.Reference,
	}
}

func loopConfig(loop ReActLoopIF) AICallerConfigIf {
	if loop == nil {
		return nil
	}
	return loop.GetConfig()
}

func browserBindingsFromAttachedResources(resources []AttachedResourceData) []AttachedBrowserBinding {
	bindings := make([]AttachedBrowserBinding, 0, len(resources))
	seen := make(map[string]struct{})
	for _, resource := range resources {
		browser, ok := resource.(*AttachedBrowserResourceData)
		if !ok || browser == nil {
			continue
		}
		if _, exists := seen[browser.DeviceID]; exists {
			continue
		}
		seen[browser.DeviceID] = struct{}{}
		bindings = append(bindings, browser.Binding())
	}
	return bindings
}

func bindAttachedBrowserResources(loop ReActLoopIF, resources []AttachedResourceData) {
	if loop == nil || loop.GetInvoker() == nil {
		return
	}
	session, ok := loop.GetInvoker().(BrowserAttachmentSession)
	if !ok {
		return
	}
	bindings := browserBindingsFromAttachedResources(resources)
	session.SetBrowserAttachments(bindings)
	promoteAttachedBrowserTools(loop.GetConfig())
}

func promoteAttachedBrowserTools(config AICallerConfigIf) []string {
	missing := missingAttachedBrowserTools(config)
	if config == nil || config.GetAiToolManager() == nil {
		return missing
	}
	manager := config.GetAiToolManager()
	missing = missing[:0]
	for _, name := range attachedBrowserBridgeToolNames {
		manager.EnableTool(name)
		tool, err := manager.GetToolByName(name)
		if err != nil || tool == nil {
			missing = append(missing, name)
			continue
		}
		if !manager.IsRecentlyUsedTool(name) {
			config.RecordRecentlyUsedTool(tool)
		}
	}
	return missing
}

func missingAttachedBrowserTools(config AICallerConfigIf) []string {
	if config == nil || config.GetAiToolManager() == nil {
		return append([]string(nil), attachedBrowserBridgeToolNames...)
	}
	manager := config.GetAiToolManager()
	missing := make([]string, 0)
	for _, name := range attachedBrowserBridgeToolNames {
		tool, err := manager.GetToolByName(name)
		if err != nil || tool == nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func renderAttachedBrowserIntroduction(bindings []AttachedBrowserBinding, missing []string) string {
	if len(bindings) == 0 {
		return ""
	}
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

func attachedBrowserResources(task AITask, runtime AIInvokeRuntime) ([]AttachedBrowserBinding, bool, error) {
	provider, ok := runtime.(BrowserAttachmentProvider)
	if !ok && task != nil {
		if loopCarrier, loopOK := task.(interface{ GetReActLoop() ReActLoopIF }); loopOK {
			if loop := loopCarrier.GetReActLoop(); loop != nil {
				provider, ok = loop.GetInvoker().(BrowserAttachmentProvider)
			}
		}
	}
	if !ok {
		return nil, false, nil
	}
	attached := provider.GetBrowserAttachments()
	resources := make([]AttachedBrowserBinding, 0, len(attached))
	seen := make(map[string]struct{})
	for _, resource := range attached {
		resource.DeviceID = strings.TrimSpace(resource.DeviceID)
		resource.Name = strings.TrimSpace(resource.Name)
		resource.Reference = strings.TrimSpace(resource.Reference)
		if resource.DeviceID == "" {
			return nil, true, utils.Error("attached browser resource has no device ID")
		}
		if resource.Reference == "" {
			resource.Reference = resource.DeviceID
		}
		if _, exists := seen[resource.DeviceID]; exists {
			continue
		}
		seen[resource.DeviceID] = struct{}{}
		resources = append(resources, resource)
	}
	return resources, len(resources) > 0, nil
}

func attachedBrowserDeviceID(task AITask, runtime AIInvokeRuntime, browserRef string) (string, bool, error) {
	resources, attached, err := attachedBrowserResources(task, runtime)
	if err != nil || !attached {
		return "", attached, err
	}
	browserRef = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(browserRef), "@"))
	if len(resources) == 1 && browserRef == "" {
		return resources[0].DeviceID, true, nil
	}
	if browserRef == "" {
		return "", true, utils.Error("multiple browser instances are attached; browser_ref is required")
	}
	var matched *AttachedBrowserBinding
	for _, resource := range resources {
		if browserRef != resource.DeviceID && !strings.EqualFold(browserRef, resource.Reference) {
			continue
		}
		matched = &resource
		break
	}
	if matched == nil {
		return "", true, utils.Errorf("browser_ref %q does not match an attached browser", browserRef)
	}
	return matched.DeviceID, true, nil
}

func isAttachedBrowserBridgeTool(toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	for _, name := range attachedBrowserBridgeToolNames {
		if toolName == name {
			return true
		}
	}
	return false
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
	return checkAttachedBrowserToolRoute(task, nil, toolName)
}

func checkAttachedBrowserToolRoute(task AITask, runtime AIInvokeRuntime, toolName string) (bool, string) {
	toolName = strings.TrimSpace(toolName)
	if toolName != attachedBrowserRodToolName && !isAttachedBrowserBridgeTool(toolName) {
		return true, ""
	}
	resources, attached, err := attachedBrowserResources(task, runtime)
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
	return bindAttachedBrowserToolParams(task, nil, toolName, params)
}

func bindAttachedBrowserToolParams(task AITask, runtime AIInvokeRuntime, toolName string, params aitool.InvokeParams) (aitool.InvokeParams, error) {
	isHTTPRequest := toolName == attachedBrowserHTTPRequestToolName
	if !isAttachedBrowserBridgeTool(toolName) && !isHTTPRequest {
		return params, nil
	}
	if params == nil {
		params = make(aitool.InvokeParams)
	}
	if isHTTPRequest {
		// A generic HTTP tool must not accept a model-supplied device ID.
		delete(params, "device_id")
		if strings.EqualFold(strings.TrimSpace(params.GetString("browser_ref")), "none") {
			delete(params, "browser_ref")
			return params, nil
		}
	}
	deviceID, attached, err := attachedBrowserDeviceID(task, runtime, params.GetString("browser_ref"))
	if err != nil {
		return params, err
	}
	if !attached {
		return params, nil
	}
	if !isHTTPRequest {
		delete(params, "browser_ref")
	}
	params["device_id"] = deviceID
	return params, nil
}
