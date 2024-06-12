package lowhttp

import (
	"net/http"
	"strings"

	"github.com/samber/lo"
)

type WebsocketExtensions struct {
	Extensions            []string
	ClientContextTakeover bool
	ServerContextTakeover bool
	IsDeflate             bool
}

func (ext *WebsocketExtensions) flateContextTakeover() bool {
	return ext.ClientContextTakeover || ext.ServerContextTakeover
}

func GetWebsocketExtensions(headers http.Header) *WebsocketExtensions {
	websocketExtensions, ok := headers["Sec-WebSocket-Extensions"]
	clientContextTakeover, serverContextTakeover := true, true
	isDeflate := false
	if !ok {
		lowerHeaders := make(map[string][]string, len(headers))
		for k, v := range headers {
			lowerHeaders[strings.ToLower(k)] = v
		}
		websocketExtensions, _ = lowerHeaders["sec-websocket-extensions"]
	}
	merged := strings.Join(websocketExtensions, ";")

	websocketExtensions = lo.FilterMap(strings.Split(merged, ";"), func(s string, _ int) (string, bool) {
		trimed := strings.TrimSpace(s)
		if trimed == "" {
			return "", false
		}
		if trimed == "permessage-deflate" || trimed == "x-webkit-deflate-frame" {
			isDeflate = true
		}
		if trimed == "server_no_context_takeover" {
			serverContextTakeover = false
		}
		if trimed == "client_no_context_takeover" {
			clientContextTakeover = false
		}
		return trimed, true
	})

	return &WebsocketExtensions{
		Extensions:            websocketExtensions,
		ClientContextTakeover: clientContextTakeover,
		ServerContextTakeover: serverContextTakeover,
		IsDeflate:             isDeflate,
	}
}
