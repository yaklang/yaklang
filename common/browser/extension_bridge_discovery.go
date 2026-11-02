package browser

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const extensionBridgeDiscoveryPath = "/discovery"
const extensionBridgePortWindow = 16

// Keep these windows in sync with the extension's discovery.ts. Separate
// windows avoid Windows/Hyper-V reserving the entire preferred port range.
var extensionBridgePortBases = [...]int{64333, 14333, 24333, 34333}

func extensionBridgeCandidatePorts(preferred int) []int {
	if preferred <= 0 || preferred > 65535 {
		return []int{preferred}
	}
	ports := make([]int, 0, extensionBridgePortWindow*(1+len(extensionBridgePortBases)))
	seen := make(map[int]bool)
	appendWindow := func(first int) {
		for port := first; port < first+extensionBridgePortWindow && port <= 65535; port++ {
			if !seen[port] {
				seen[port] = true
				ports = append(ports, port)
			}
		}
	}
	appendWindow(preferred)
	for _, first := range extensionBridgePortBases {
		appendWindow(first)
	}
	return ports
}

func retryableExtensionBridgeListenError(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) ||
		// Winsock errors do not compare equal to the POSIX syscall constants.
		(runtime.GOOS == "windows" && (errors.Is(err, syscall.Errno(10048)) || errors.Is(err, syscall.Errno(10013))))
}

type extensionBridgeDiscoveryEnvelope struct {
	Type             string `json:"type"`
	ProtocolVersion  int    `json:"protocolVersion"`
	EngineIdentityID string `json:"engineIdentityId"`
	EngineInstanceID string `json:"engineInstanceId"`
	Endpoint         string `json:"endpoint"`
	FrontendID       string `json:"frontendId"`
	FrontendName     string `json:"frontendName"`
}

func extensionBridgeFrontend() (string, string) {
	edition := strings.ToLower(os.Getenv("YAKIT_EDITION"))
	database := strings.ToLower(filepath.Base(os.Getenv("YAK_DEFAULT_PROJECT_DATABASE_NAME")))
	switch {
	case strings.Contains(edition, "memfit") || strings.Contains(database, "memfit"):
		return "memfit", "Memfit AI"
	case strings.Contains(edition, "irify") || strings.Contains(database, "irify"):
		return "irify", "IRify"
	case strings.Contains(edition, "ee") || strings.Contains(edition, "enterprise") || strings.Contains(database, "company"):
		return "yakit-enterprise", "Yakit 企业版"
	case strings.Contains(edition, "yakit") || strings.Contains(database, "yakit") || os.Getenv("YAKIT_HOME") != "":
		return "yakit", "Yakit"
	default:
		return "yak", "独立 Yak 引擎"
	}
}

func (s *ExtensionBridgeServer) handleDiscoveryWebSocket(writer http.ResponseWriter, request *http.Request) {
	engineIdentityID, _ := s.manager.EngineIdentity()
	frontendID, frontendName := extensionBridgeFrontend()
	envelope := extensionBridgeDiscoveryEnvelope{
		Type: "engine", ProtocolVersion: managedExtensionBridgeProtocolVersion,
		EngineIdentityID: engineIdentityID, EngineInstanceID: s.engineInstanceID, Endpoint: s.URL(),
		FrontendID: frontendID, FrontendName: frontendName,
	}
	// YTray reads the same public discovery hint before launching a browser.
	// Browser clients still use the extension-origin WebSocket route below.
	if !websocket.IsWebSocketUpgrade(request) {
		host, _, err := net.SplitHostPort(request.Host)
		if request.Method != http.MethodGet || request.Header.Get("Origin") != "" || err != nil ||
			(host != "127.0.0.1" && host != "localhost" && host != "::1") {
			http.Error(writer, "local discovery only", http.StatusForbidden)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(writer).Encode(envelope)
		return
	}
	upgrader := websocket.Upgrader{
		HandshakeTimeout: 2 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			_, valid := NormalizeBrowserExtensionOrigin(r.Header.Get("Origin"))
			return valid
		},
	}
	connection, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	_ = connection.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = connection.WriteJSON(envelope)
}
