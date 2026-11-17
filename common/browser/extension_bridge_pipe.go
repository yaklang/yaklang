package browser

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/utils/engineendpoint"
)

const extensionBridgePipePrefix = "yak-browser-bridge-v1-"
const extensionBridgePipeOrigin = "chrome-extension://mcnaombmlombekhbonfndagbcfhmoail"

type extensionBridgeWire interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	WriteJSON(interface{}) error
	WriteControl(int, []byte, time.Time) error
	SetReadLimit(int64)
	SetReadDeadline(time.Time) error
	Close() error
}

type extensionBridgePipeWire struct {
	net.Conn
	readLimit int64
	writeMu   sync.Mutex
}

func (w *extensionBridgePipeWire) SetReadLimit(limit int64) { w.readLimit = limit }
func (w *extensionBridgePipeWire) ReadMessage() (int, []byte, error) {
	var size uint32
	if err := binary.Read(w.Conn, binary.LittleEndian, &size); err != nil {
		return 0, nil, err
	}
	if size == 0 || int64(size) > w.readLimit {
		return 0, nil, errors.New("invalid browser bridge pipe frame size")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(w.Conn, data)
	return websocket.TextMessage, data, err
}
func (w *extensionBridgePipeWire) WriteMessage(_ int, data []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	_ = w.SetWriteDeadline(time.Now().Add(5 * time.Second))
	defer w.SetWriteDeadline(time.Time{})
	return WriteNativeMessagingMessage(w.Conn, json.RawMessage(data))
}
func (w *extensionBridgePipeWire) WriteJSON(value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.WriteMessage(websocket.TextMessage, data)
}
func (w *extensionBridgePipeWire) WriteControl(_ int, _ []byte, _ time.Time) error { return w.Close() }

type extensionBridgePipeDescriptor struct {
	extensionBridgeDiscoveryEnvelope
	PipeName string `json:"pipeName"`
	PID      int    `json:"pid"`
}

type extensionBridgePipeServer struct {
	name, descriptor string
	listener         net.Listener
	cancel           context.CancelFunc
	mu               sync.Mutex
	connections      map[net.Conn]struct{}
}

func newNativeExtensionBridgeServer(manager *ExtensionBridgeManager) (*ExtensionBridgeServer, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("browser bridge named pipes require Windows")
	}
	bridge, err := makeExtensionBridgeServer("", manager)
	if err != nil {
		return nil, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(cache, "YTray", "BrowserEngines")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	name := extensionBridgePipePrefix + bridge.engineInstanceID
	listener, err := engineendpoint.Listen("npipe", `\\.\pipe\`+name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	pipe := &extensionBridgePipeServer{name: name, descriptor: filepath.Join(directory, name+".json"),
		listener: listener, cancel: cancel, connections: make(map[net.Conn]struct{})}
	bridge.native = pipe
	identity, _ := manager.EngineIdentity()
	frontend, label := extensionBridgeFrontend()
	desc := extensionBridgePipeDescriptor{extensionBridgeDiscoveryEnvelope{
		Type: "engine", ProtocolVersion: managedExtensionBridgeProtocolVersion,
		EngineIdentityID: identity, EngineInstanceID: bridge.engineInstanceID,
		FrontendID: frontend, FrontendName: label,
	}, name, os.Getpid()}
	data, _ := json.Marshal(desc)
	temp := pipe.descriptor + ".tmp"
	if err := os.WriteFile(temp, data, 0600); err != nil {
		_ = pipe.close()
		return nil, err
	}
	if err := os.Rename(temp, pipe.descriptor); err != nil {
		_ = os.Remove(temp)
		_ = pipe.close()
		return nil, err
	}
	go pipe.serve(ctx, bridge, desc)
	return bridge, nil
}

func (s *extensionBridgePipeServer) serve(ctx context.Context, bridge *ExtensionBridgeServer, desc extensionBridgePipeDescriptor) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if ctx.Err() != nil {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		s.connections[conn] = struct{}{}
		s.mu.Unlock()
		go func() {
			defer func() {
				_ = conn.Close()
				s.mu.Lock()
				delete(s.connections, conn)
				s.mu.Unlock()
			}()
			wire := &extensionBridgePipeWire{Conn: conn, readLimit: 4096}
			_ = wire.SetReadDeadline(time.Now().Add(4 * time.Second))
			var hello struct {
				Type             string `json:"type"`
				Origin           string `json:"origin"`
				Purpose          string `json:"purpose"`
				EngineInstanceID string `json:"engineInstanceId"`
				EngineIdentityID string `json:"engineIdentityId"`
			}
			if err := readStrictExtensionBridgeJSON(wire, &hello); err != nil {
				return
			}
			if hello.EngineInstanceID != desc.EngineInstanceID || hello.EngineIdentityID != desc.EngineIdentityID {
				return
			}
			if hello.Type == "discover" {
				_ = wire.WriteJSON(desc)
				return
			}
			if hello.Type != "open" || strings.TrimSuffix(hello.Origin, "/") != extensionBridgePipeOrigin {
				return
			}
			switch hello.Purpose {
			case "connect":
				bridge.handleBridgeConnection(wire, extensionBridgePipeOrigin)
			case "pair":
				bridge.handleBridgePairing(ctx, wire, extensionBridgePipeOrigin)
			}
		}()
	}
}

func (s *extensionBridgePipeServer) close() error {
	s.cancel()
	err := s.listener.Close()
	s.mu.Lock()
	for conn := range s.connections {
		_ = conn.Close()
	}
	s.mu.Unlock()
	_ = os.Remove(s.descriptor)
	return err
}
