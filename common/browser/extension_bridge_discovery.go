package browser

import (
	"errors"
	"net/http"
	"runtime"
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
}

func (s *ExtensionBridgeServer) handleDiscoveryWebSocket(writer http.ResponseWriter, request *http.Request) {
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
	engineIdentityID, _ := s.manager.EngineIdentity()
	_ = connection.WriteJSON(extensionBridgeDiscoveryEnvelope{
		Type: "engine", ProtocolVersion: managedExtensionBridgeProtocolVersion,
		EngineIdentityID: engineIdentityID, EngineInstanceID: s.engineInstanceID, Endpoint: s.URL(),
	})
}
