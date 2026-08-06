//go:build !yakit_exclude

package yakurl

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type browserExtensionAction struct{}

type browserExtensionMutation struct {
	Name            string `json:"name"`
	Message         string `json:"message"`
	ReplaceDeviceID string `json:"replaceDeviceId"`
	TTLSeconds      int64  `json:"ttlSeconds"`
}

func newBrowserExtensionAction() Action {
	return &browserExtensionAction{}
}

func (a *browserExtensionAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	if normalizeBrowserExtensionPath(params) != "/snapshot" {
		return nil, utils.Error("browser extension resource not found")
	}
	manager, err := activeBrowserExtensionManager()
	if err != nil {
		return nil, err
	}
	return browserExtensionSnapshotResponse(manager)
}

func (a *browserExtensionAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	manager, err := activeBrowserExtensionManager()
	if err != nil {
		return nil, err
	}
	path := normalizeBrowserExtensionPath(params)
	var mutation browserExtensionMutation
	if len(params.GetBody()) > 0 {
		decoder := json.NewDecoder(strings.NewReader(string(params.GetBody())))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&mutation); err != nil {
			return nil, utils.Errorf("invalid browser extension mutation: %s", err)
		}
	}
	switch {
	case path == "/pairing-window":
		manager.OpenPairingWindow(time.Duration(mutation.TTLSeconds) * time.Second)
	case strings.HasPrefix(path, "/pairings/") && strings.HasSuffix(path, "/approve"):
		requestID := strings.TrimSuffix(strings.TrimPrefix(path, "/pairings/"), "/approve")
		if strings.TrimSpace(requestID) == "" || strings.Contains(requestID, "/") {
			return nil, utils.Error("invalid browser extension pairing request id")
		}
		if _, approveErr := manager.ApprovePairing(requestID, mutation.Name, mutation.ReplaceDeviceID); approveErr != nil {
			return nil, approveErr
		}
	case strings.HasPrefix(path, "/devices/"):
		deviceID := strings.TrimPrefix(path, "/devices/")
		if strings.TrimSpace(deviceID) == "" || strings.Contains(deviceID, "/") {
			return nil, utils.Error("invalid browser extension device id")
		}
		if err := manager.RenameDevice(deviceID, mutation.Name); err != nil {
			return nil, err
		}
	default:
		return nil, utils.Error("browser extension mutation not found")
	}
	return browserExtensionSnapshotResponse(manager)
}

func (a *browserExtensionAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.Post(params)
}

func (a *browserExtensionAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	manager, err := activeBrowserExtensionManager()
	if err != nil {
		return nil, err
	}
	path := normalizeBrowserExtensionPath(params)
	switch {
	case strings.HasPrefix(path, "/pairings/"):
		requestID := strings.TrimPrefix(path, "/pairings/")
		if strings.TrimSpace(requestID) == "" || strings.Contains(requestID, "/") {
			return nil, utils.Error("invalid browser extension pairing request id")
		}
		var mutation browserExtensionMutation
		if len(params.GetBody()) > 0 {
			_ = json.Unmarshal(params.GetBody(), &mutation)
		}
		if err := manager.RejectPairing(requestID, mutation.Message); err != nil {
			return nil, err
		}
	case strings.HasPrefix(path, "/devices/"):
		deviceID := strings.TrimPrefix(path, "/devices/")
		if strings.TrimSpace(deviceID) == "" || strings.Contains(deviceID, "/") {
			return nil, utils.Error("invalid browser extension device id")
		}
		if err := manager.RevokeDevice(deviceID); err != nil {
			return nil, err
		}
	default:
		return nil, utils.Error("browser extension resource not found")
	}
	return browserExtensionSnapshotResponse(manager)
}

func (a *browserExtensionAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.Get(&ypb.RequestYakURLParams{Method: http.MethodGet, Url: &ypb.YakURL{Path: "/snapshot"}})
}

func (a *browserExtensionAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	switch strings.ToUpper(params.GetMethod()) {
	case http.MethodGet:
		return a.Get(params)
	case http.MethodPost:
		return a.Post(params)
	case http.MethodPut:
		return a.Put(params)
	case http.MethodDelete:
		return a.Delete(params)
	case http.MethodHead:
		return a.Head(params)
	default:
		return nil, utils.Errorf("unsupported browser extension method: %s", params.GetMethod())
	}
}

func activeBrowserExtensionManager() (*browser.ExtensionBridgeManager, error) {
	manager := browser.ActiveExtensionBridgeManager()
	if manager == nil {
		return nil, utils.Error("browser extension bridge is not available in this Yak engine")
	}
	return manager, nil
}

func normalizeBrowserExtensionPath(params *ypb.RequestYakURLParams) string {
	if params == nil || params.GetUrl() == nil {
		return "/"
	}
	path := strings.TrimSpace(params.GetUrl().GetPath())
	if path == "" || path == "/" {
		return "/snapshot"
	}
	return "/" + strings.Trim(path, "/")
}

func browserExtensionSnapshotResponse(manager *browser.ExtensionBridgeManager) (*ypb.RequestYakURLResponse, error) {
	snapshot := manager.Snapshot()
	resources := make([]*ypb.YakURLResource, 0, 1+len(snapshot.Pending)+len(snapshot.Devices))
	status := struct {
		Revision         uint64                              `json:"revision"`
		Running          bool                                `json:"running"`
		Connected        bool                                `json:"connected"`
		URL              string                              `json:"url,omitempty"`
		LastError        string                              `json:"lastError,omitempty"`
		ProtocolVersion  int                                 `json:"protocolVersion"`
		EngineIdentityID string                              `json:"engineIdentityId"`
		EngineInstanceID string                              `json:"engineInstanceId"`
		PairingOpenUntil int64                               `json:"pairingOpenUntil,omitempty"`
		Connections      []browser.ExtensionBridgeConnection `json:"connections"`
	}{
		Revision: snapshot.Revision, Running: snapshot.Running, Connected: snapshot.Connected,
		URL: snapshot.URL, LastError: snapshot.LastError, ProtocolVersion: snapshot.ProtocolVersion,
		EngineIdentityID: snapshot.EngineIdentityID, EngineInstanceID: snapshot.EngineInstanceID,
		PairingOpenUntil: snapshot.PairingOpenUntil,
		Connections:      snapshot.Connections,
	}
	resources = append(resources, browserExtensionResource("status", "bridge", "Yak Browser Bridge", status, time.Now().Unix()))
	for _, pending := range snapshot.Pending {
		resources = append(resources, browserExtensionResource("pairing-request", pending.ID, pending.Client, pending, pending.CreatedAt/1000))
	}
	for _, device := range snapshot.Devices {
		resources = append(resources, browserExtensionResource("paired-device", device.ID, device.Name, device, device.LastSeenAt/1000))
	}
	return &ypb.RequestYakURLResponse{Page: 1, PageSize: int64(len(resources)), Total: int64(len(resources)), Resources: resources}, nil
}

func browserExtensionResource(resourceType, name, verboseName string, value interface{}, modified int64) *ypb.YakURLResource {
	encoded, _ := json.Marshal(value)
	return &ypb.YakURLResource{
		ResourceType: resourceType, VerboseType: resourceType,
		ResourceName: name, VerboseName: verboseName, ModifiedTimestamp: modified,
		Path:  "/" + resourceType + "/" + name,
		Url:   &ypb.YakURL{Schema: "browser-extension", Location: "local", Path: "/" + resourceType + "/" + name},
		Extra: []*ypb.KVPair{{Key: "data", Value: string(encoded)}},
	}
}
