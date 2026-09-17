package browsertools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/browser"
)

const (
	ReadTimeout    = 20 * time.Second
	ReplayTimeout  = 60 * time.Second
	maxResultBytes = 2 << 20
)

type Caller interface {
	CallDevice(context.Context, string, string, interface{}) (json.RawMessage, error)
}

type Bridge interface {
	Caller
	Available() bool
	CapabilityCatalog(deviceID string) (*browser.ExtensionBridgeCapabilityCatalog, bool)
	Connections() []browser.ExtensionBridgeConnection
}

type connectedBrowser struct {
	Reference  string
	DeviceID   string
	Manager    string
	InstanceID string
	Client     string
	Domains    []string
}

func connectedBrowsers(bridge Bridge) []connectedBrowser {
	if bridge == nil || !bridge.Available() {
		return nil
	}
	connections := bridge.Connections()
	result := make([]connectedBrowser, 0, len(connections))
	for _, connection := range connections {
		deviceID := strings.TrimSpace(connection.DeviceID)
		if deviceID == "" {
			continue
		}
		item := connectedBrowser{
			Reference: fmt.Sprintf("Browser %d", len(result)+1),
			DeviceID:  deviceID,
			Client:    strings.TrimSpace(connection.Client),
		}
		if connection.ManagedInstance != nil {
			item.Manager = strings.TrimSpace(connection.ManagedInstance.Manager)
			item.InstanceID = strings.TrimSpace(connection.ManagedInstance.InstanceID)
			if badge := strings.TrimSpace(connection.ManagedInstance.Badge); badge != "" {
				item.Reference = badge
			}
		}
		domains := make(map[string]struct{})
		if connection.CapabilityCatalog != nil {
			for _, capability := range connection.CapabilityCatalog.Capabilities {
				if capability.VisibleToAgent() && strings.TrimSpace(capability.Domain) != "" {
					domains[capability.Domain] = struct{}{}
				}
			}
		}
		for domain := range domains {
			item.Domains = append(item.Domains, domain)
		}
		sort.Strings(item.Domains)
		result = append(result, item)
	}
	return result
}

func resolveBrowserDevice(bridge Bridge, deviceID, browserRef string) (connectedBrowser, error) {
	instances := connectedBrowsers(bridge)
	if len(instances) == 0 {
		return connectedBrowser{}, errors.New("no browser-extension instance is connected")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID != "" {
		for _, instance := range instances {
			if instance.DeviceID == deviceID {
				return instance, nil
			}
		}
		return connectedBrowser{}, errors.New("the explicitly attached browser instance is offline")
	}

	browserRef = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(browserRef), "@"))
	if browserRef == "" {
		if len(instances) == 1 {
			return instances[0], nil
		}
		references := make([]string, 0, len(instances))
		for _, instance := range instances {
			references = append(references, instance.Reference)
		}
		return connectedBrowser{}, fmt.Errorf(
			"multiple browser-extension instances are online (%s); choose one with browser_ref",
			strings.Join(references, ", "),
		)
	}

	var matched *connectedBrowser
	for index := range instances {
		instance := &instances[index]
		if !strings.EqualFold(browserRef, instance.Reference) &&
			!strings.EqualFold(browserRef, instance.InstanceID) && browserRef != instance.DeviceID {
			continue
		}
		if matched != nil && matched.DeviceID != instance.DeviceID {
			return connectedBrowser{}, fmt.Errorf("browser_ref %q matches more than one online browser", browserRef)
		}
		matched = instance
	}
	if matched == nil {
		return connectedBrowser{}, fmt.Errorf("browser_ref %q does not match an online browser", browserRef)
	}
	return *matched, nil
}

// ResolveBrowserDevice resolves a user-facing A/B/C reference inside the
// engine. Device IDs never need to enter the model-visible result.
func ResolveBrowserDevice(bridge Bridge, deviceID, browserRef string) (string, string, error) {
	instance, err := resolveBrowserDevice(bridge, deviceID, browserRef)
	if err != nil {
		return "", "", err
	}
	return instance.DeviceID, instance.Reference, nil
}

// RuntimeContext is refreshed before every Agent decision. Device IDs stay in
// the engine; the model only sees stable, user-facing browser references.
func RuntimeContext(bridge Bridge) string {
	instances := connectedBrowsers(bridge)
	if len(instances) == 0 {
		return ""
	}
	var output strings.Builder
	output.WriteString("# Connected Browser Extension Instances\n\n")
	output.WriteString("These are already-open external browsers, not Rod automation sessions. Use browser.instances.list, browser.capability.catalog, browser.capability.call, browser.crypto.inspect, browser.transform.prepare, browser.handoff.request, and browser.http.test. The use_browser tool is unrelated Rod automation and never routes to these instances. @-mention is optional; an explicit @ or browser_ref always wins. When exactly one instance is online, the browser-extension tools select it automatically. With multiple instances, use their references for singular operations and use all relevant references for comparison or authorization testing. For page-backed encrypted HTTP, call browser.crypto.inspect once, browser.transform.prepare once, then browser.http.test; prefer automatic preparation; use catalog-discovered recording/callable/debugger/Profile tools when advanced diagnosis or recovery is needed. Never reopen a page merely because a dialog appeared or resend its plaintext with another HTTP tool. Never invent or expose a device ID.\n")
	for _, instance := range instances {
		fmt.Fprintf(&output, "\n- %s: online", instance.Reference)
		if instance.Manager != "" {
			fmt.Fprintf(&output, ", managed by %s", instance.Manager)
		}
		if instance.Client != "" {
			fmt.Fprintf(&output, ", client %s", instance.Client)
		}
		if len(instance.Domains) > 0 {
			fmt.Fprintf(&output, ", capability domains: %s", strings.Join(instance.Domains, ", "))
		}
	}
	return output.String()
}

type Target struct {
	TabID      int
	FrameID    int
	DocumentID string
}

func (t Target) Params() map[string]interface{} {
	result := map[string]interface{}{
		"tabId":   t.TabID,
		"frameId": t.FrameID,
	}
	if t.DocumentID != "" {
		result["documentId"] = t.DocumentID
	}
	return result
}

func cloneParams(params aitool.InvokeParams) map[string]interface{} {
	result := make(map[string]interface{}, len(params)+3)
	for key, value := range params {
		result[key] = value
	}
	return result
}

func decodeResult(raw json.RawMessage) (interface{}, error) {
	if len(raw) > maxResultBytes {
		return nil, fmt.Errorf(
			"browser capability result exceeds %d bytes; narrow the requested target or evidence",
			maxResultBytes,
		)
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var result interface{}
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode browser capability result: %w", err)
	}
	return result, nil
}

func CallCapability(
	ctx context.Context,
	caller Caller,
	deviceID string,
	target Target,
	method string,
	params aitool.InvokeParams,
	timeout time.Duration,
	withTarget bool,
) (interface{}, error) {
	if caller == nil {
		return nil, errors.New("browser extension bridge is not running")
	}

	payload := cloneParams(params)
	if withTarget {
		for key, value := range target.Params() {
			payload[key] = value
		}
	}

	callContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raw, err := caller.CallDevice(callContext, deviceID, method, payload)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	return decodeResult(raw)
}
