package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type NativeMessagingProxyOptions struct {
	Endpoint string
	Origin   string
}

func ValidateNativeMessagingProxyOptions(options NativeMessagingProxyOptions) error {
	endpoint, err := url.Parse(strings.TrimSpace(options.Endpoint))
	if err != nil || (endpoint.Scheme != "ws" && endpoint.Scheme != "wss") {
		return errors.New("native host endpoint must be a WebSocket URL")
	}
	hostname := strings.TrimSpace(endpoint.Hostname())
	if hostname == "" {
		return errors.New("native host endpoint is missing a hostname")
	}
	if hostname != "localhost" {
		ip := net.ParseIP(hostname)
		if ip == nil || !ip.IsLoopback() {
			return errors.New("native host endpoint must resolve to an explicit loopback address")
		}
	}
	if _, ok := NormalizeBrowserExtensionOrigin(options.Origin); !ok {
		return errors.New("native host requires a browser extension origin")
	}
	return nil
}

func NormalizeBrowserExtensionOrigin(value string) (string, bool) {
	origin, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (origin.Scheme != "chrome-extension" && origin.Scheme != "moz-extension") {
		return "", false
	}
	if origin.Hostname() == "" || origin.Port() != "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" {
		return "", false
	}
	if origin.Path != "" && origin.Path != "/" {
		return "", false
	}
	return origin.Scheme + "://" + strings.ToLower(origin.Hostname()), true
}

func RunNativeMessagingProxy(ctx context.Context, input io.Reader, output io.Writer, options NativeMessagingProxyOptions) error {
	if err := ValidateNativeMessagingProxyOptions(options); err != nil {
		return err
	}
	origin, _ := NormalizeBrowserExtensionOrigin(options.Origin)
	header := http.Header{"Origin": []string{origin}}
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, options.Endpoint, header)
	if err != nil {
		return fmt.Errorf("connect native host to Yak Bridge: %w", err)
	}
	defer connection.Close()
	connection.SetReadLimit(MaxNativeMessagingMessageSize)

	errorChannel := make(chan error, 2)
	go func() {
		for {
			message, readErr := ReadNativeMessagingMessage(input)
			if readErr != nil {
				errorChannel <- readErr
				return
			}
			if writeErr := connection.WriteMessage(websocket.TextMessage, message); writeErr != nil {
				errorChannel <- fmt.Errorf("forward native message to Yak Bridge: %w", writeErr)
				return
			}
		}
	}()
	go func() {
		for {
			_, message, readErr := connection.ReadMessage()
			if readErr != nil {
				errorChannel <- readErr
				return
			}
			if writeErr := WriteNativeMessagingMessage(output, json.RawMessage(message)); writeErr != nil {
				errorChannel <- fmt.Errorf("forward Yak Bridge message to browser: %w", writeErr)
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "native host stopped"), time.Now().Add(time.Second))
		return ctx.Err()
	case proxyErr := <-errorChannel:
		if errors.Is(proxyErr, io.EOF) || websocket.IsCloseError(proxyErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			return nil
		}
		return proxyErr
	}
}
