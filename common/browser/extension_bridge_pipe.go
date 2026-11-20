package browser

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
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

type extensionBridgeNativeWire struct {
	net.Conn
	readLimit int64
	writeMu   sync.Mutex
}

func (w *extensionBridgeNativeWire) SetReadLimit(limit int64) { w.readLimit = limit }
func (w *extensionBridgeNativeWire) ReadMessage() (int, []byte, error) {
	var size uint32
	if err := binary.Read(w.Conn, binary.LittleEndian, &size); err != nil {
		return 0, nil, err
	}
	if size == 0 || int64(size) > w.readLimit {
		return 0, nil, errors.New("invalid browser bridge native frame size")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(w.Conn, data)
	return websocket.TextMessage, data, err
}
func (w *extensionBridgeNativeWire) WriteMessage(_ int, data []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	_ = w.SetWriteDeadline(time.Now().Add(5 * time.Second))
	defer w.SetWriteDeadline(time.Time{})
	return WriteNativeMessagingMessage(w.Conn, json.RawMessage(data))
}
func (w *extensionBridgeNativeWire) WriteJSON(value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.WriteMessage(websocket.TextMessage, data)
}
func (w *extensionBridgeNativeWire) WriteControl(_ int, _ []byte, _ time.Time) error {
	return w.Close()
}

type extensionBridgeNativeDescriptor struct {
	extensionBridgeDiscoveryEnvelope
	PipeName   string `json:"pipeName,omitempty"`
	SocketName string `json:"socketName,omitempty"`
	PID        int    `json:"pid"`
}

type extensionBridgeNativeServer struct {
	address, descriptor string
	listener            net.Listener
	cancel              context.CancelFunc
	mu                  sync.Mutex
	connections         map[net.Conn]struct{}
}

func newNativeExtensionBridgeServer(manager *ExtensionBridgeManager) (*ExtensionBridgeServer, error) {
	bridge, err := makeExtensionBridgeServer("", manager)
	if err != nil {
		return nil, err
	}
	name := extensionBridgePipePrefix + bridge.engineInstanceID
	identity, _ := manager.EngineIdentity()
	frontend, label := extensionBridgeFrontend()
	desc := extensionBridgeNativeDescriptor{
		extensionBridgeDiscoveryEnvelope: extensionBridgeDiscoveryEnvelope{
			Type: "engine", ProtocolVersion: managedExtensionBridgeProtocolVersion,
			EngineIdentityID: identity, EngineInstanceID: bridge.engineInstanceID,
			FrontendID: frontend, FrontendName: label,
		},
		PID: os.Getpid(),
	}

	var directory, address string
	var listener net.Listener
	if runtime.GOOS == "windows" {
		cache, cacheErr := os.UserCacheDir()
		if cacheErr != nil {
			return nil, cacheErr
		}
		directory = filepath.Join(cache, "YTray", "BrowserEngines")
		if err := os.MkdirAll(directory, 0700); err != nil {
			return nil, err
		}
		desc.PipeName = name
		address = "npipe:" + name
		listener, err = engineendpoint.Listen("npipe", `\\.\pipe\`+name)
	} else {
		desc.SocketName = name + ".sock"
		directory, listener, err = listenExtensionBridgeUnix(desc.SocketName)
		address = "unix:" + filepath.Join(directory, desc.SocketName)
	}
	if err != nil {
		return nil, fmt.Errorf("start browser bridge IPC: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	native := &extensionBridgeNativeServer{address: address, descriptor: filepath.Join(directory, name+".json"),
		listener: listener, cancel: cancel, connections: make(map[net.Conn]struct{})}
	bridge.native = native
	data, _ := json.Marshal(desc)
	temp := native.descriptor + ".tmp"
	if err := os.WriteFile(temp, data, 0600); err != nil {
		_ = native.close()
		return nil, err
	}
	if err := os.Rename(temp, native.descriptor); err != nil {
		_ = os.Remove(temp)
		_ = native.close()
		return nil, err
	}
	go native.serve(ctx, bridge, desc)
	return bridge, nil
}

func (s *extensionBridgeNativeServer) serve(ctx context.Context, bridge *ExtensionBridgeServer, desc extensionBridgeNativeDescriptor) {
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
			wire := &extensionBridgeNativeWire{Conn: conn, readLimit: 4096}
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

func (s *extensionBridgeNativeServer) close() error {
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
