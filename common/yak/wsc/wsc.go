package wsc

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"net/http"
	"strings"
	"sync"
)

type WebsocketController struct {
	Token   string
	Port    int
	Ctx     context.Context
	clients map[string]*websocket.Conn
	mu      sync.Mutex
}

func NewWebsocketController(token string, port int) *WebsocketController {
	return &WebsocketController{Port: port, Token: token, clients: make(map[string]*websocket.Conn)}
}

// Store the client connection on establishment
func (w *WebsocketController) storeClient(clientAddr string, conn *websocket.Conn) {
	w.clients[clientAddr] = conn
}

// Remove the client connection on disconnection
func (w *WebsocketController) removeClient() {
	token := w.Token
	if _, ok := w.clients[token]; ok {
		msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "wsc server closing connection")
		w.clients[token].WriteMessage(websocket.CloseMessage, msg)
		w.clients[token] = nil
		delete(w.clients, token)
	}
}

func (w *WebsocketController) Run() error {
	log.Infof("Starting to listen on websocket controller on : 0.0.0.0:%v", w.Port)
	http.HandleFunc("/", w.handleWebSocket)
	err := http.ListenAndServe(fmt.Sprintf(":%d", w.Port), nil)
	if err != nil {
		log.Errorf("Listen ws controller failed: %v", err)
		return err
	}
	return nil
}

func (w *WebsocketController) handleWebSocket(wr http.ResponseWriter, req *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	w.Token = req.URL.Query().Get("token")

	conn, err := upgrader.Upgrade(wr, req, nil)
	if err != nil {
		log.Errorf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	//w.storeClient(clientAddr, conn)
	//defer w.removeClient(clientAddr)
	w.clients[w.Token] = conn
	defer w.removeClient()
	log.Infof("Accepted ws connection from %v", clientAddr)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Errorf("Read message error: %v", err)
			break
		}

		w.onMessage(message, conn)
	}
}

func (w *WebsocketController) onMessage(jsonText []byte, conn *websocket.Conn) {
	result := gjson.ParseBytes(jsonText)
	msgType := result.Get("type").String()
	switch strings.ToLower(strings.TrimSpace(msgType)) {
	case "heartbeat":
		log.Infof("Heartbeat message from ws client")
	case "chrome-extension":
		w.mu.Lock()
		fw, exists := w.clients["fuzzer"]
		if exists {
			if err := fw.WriteMessage(websocket.TextMessage, jsonText); err != nil {
				log.Errorf("Forward message error: %v", err)
			}
		}
		w.mu.Unlock()
	default:
		w.mu.Lock()
		if w.Token == "fuzzer" {
			fw, exists := w.clients["chrome"]
			if exists {
				if err := fw.WriteMessage(websocket.TextMessage, jsonText); err != nil {
					log.Errorf("Forward message error: %v", err)
				}
			}
			log.Infof("Received from fuzzer client: %v", string(result.String()))
		} else if w.Token == "chrome" {
			fw, exists := w.clients["fuzzer"]
			if exists {
				if err := fw.WriteMessage(websocket.TextMessage, jsonText); err != nil {
					log.Errorf("Forward message error: %v", err)
				}
			}
			log.Infof("Received from chrome client: %v", string(result.String()))
		}
		w.mu.Unlock()
	}
}
