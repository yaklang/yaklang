package httpclient

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Timeout for establishing the connection and for reading/writing messages.
	writeWait = 30 * time.Second

	pongWait   = 20 * time.Second
	pingPeriod = (pongWait * 8) / 10
	// Maximum message size allowed from peer.
	maxMessageSize = 1024
	// maxMessageSize = 512.
)

type IWsClient interface {
	ConnClient(req interface{}) error
	CloseClient() error
	SendBinaryDates(data []byte)
	ResultChans(wsmsg <-chan WsMessage, err <-chan error)
}

// StartClient starts the client operation.
func (c *WsClient) ConnClient(req interface{}) error {
	if err := c.connect(); err != nil {
		return err
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return err
	}
	reqInput := WsMessage{
		Type: websocket.TextMessage,
		Data: reqJSON,
	}

	c.inputChan <- reqInput

	err, ok := <-c.errChan
	if ok && err != nil {
		log.Println("error: ", err)
	}
	return nil
}

func (c *WsClient) CloseClient() error {
	close(c.inputChan)
	close(c.outputChan)
	close(c.errChan)
	c.Conn.Close()
	return nil
}

func (c *WsClient) SendBinaryDates(data []byte) {
	streamInput := WsMessage{
		Type: websocket.BinaryMessage,
		Data: data,
	}

	c.inputChan <- streamInput
}

func (c *WsClient) ResultChans() (<-chan WsMessage, <-chan error) {
	return c.outputChan, c.errChan
}

type WsMessage struct {
	// ws data type, e.g. websocket.TextMessage, websocket.BinaryMessage...
	Type int
	// ws data body
	Data []byte
}

// Client represents a websocket client.
type WsClient struct {
	URL        string
	Headers    http.Header
	Conn       *websocket.Conn
	inputChan  chan WsMessage
	outputChan chan WsMessage
	errChan    chan error
}

func NewWsClient(url string, headers http.Header) *WsClient {
	return &WsClient{
		URL:     url,
		Headers: headers,
	}
}

// readPump pumps messages from the websocket connection to the hub.
func (c *WsClient) readPump() {
	defer func() {
		c.Conn.Close()
	}()

	pongDelay := time.Now().Add(pongWait)
	pongFn := func(string) error {
		if err := c.Conn.SetReadDeadline(pongDelay); err != nil {
			return err
		}
		return nil
	}

	c.Conn.SetReadLimit(maxMessageSize)
	if err := c.Conn.SetReadDeadline(pongDelay); err != nil {
		log.Printf("error: %v", err)
	}
	c.Conn.SetPongHandler(pongFn)
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
				c.errChan <- err
			}
			break
		}

		log.Println("message: ", string(message))
		c.outputChan <- WsMessage{
			Type: websocket.TextMessage,
			Data: message,
		}
		// Process the message (this part needs to be implemented based on your application logic).
	}
}

// writePump pumps messages from the write channel to the websocket connection.
//
//nolint:cyclop
func (c *WsClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.inputChan:
			if !ok {
				// The write channel is closed.
				c.errChan <- errors.New("write channel is closed")
				err := c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				if err != nil {
					log.Printf("error: %v", err)
				}
				return
			}
			err := c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err != nil {
				log.Printf("error: %v", err)
			}

			// TODO: 临时输出
			if message.Type == websocket.TextMessage {
				log.Printf("ws TextMessage: %v\n", string(message.Data))
			}

			if err := c.Conn.WriteMessage(message.Type, message.Data); err != nil {
				log.Println("err in write message: ", err)
				c.errChan <- err
				return
			}

			c.errChan <- nil
		case <-ticker.C:
			if err := c.Conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.errChan <- err
				return
			}
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.errChan <- err
				return
			}
		}
	}
}

// connect initializes the websocket connection and starts the read and write pumps.
func (c *WsClient) connect() error {
	conn, resp, err := websocket.DefaultDialer.Dial(c.URL, c.Headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.Conn = conn
	c.inputChan = make(chan WsMessage, 100)
	c.outputChan = make(chan WsMessage, 100)
	c.errChan = make(chan error, 1)
	go c.writePump()
	go c.readPump()
	return nil
}
