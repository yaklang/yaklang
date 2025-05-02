package aibalance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"

	_ "github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func (c *Config) serveChatCompletions(conn net.Conn, rawPacket []byte) {
	// handle ai request
	auth := ""
	_, body := lowhttp.SplitHTTPPacket(rawPacket, func(method string, requestUri string, proto string) error {
		return nil
	}, func(proto string, code int, codeMsg string) error {
		return nil
	}, func(line string) string {
		k, v := lowhttp.SplitHTTPHeader(line)
		if k == "Authorization" || k == "authorization" {
			auth = v
		}
		return line
	})
	if string(body) == "" {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}
	value := strings.TrimPrefix(auth, "Bearer ")
	log.Info("fetch key from header: ", value)
	if value == "" {
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
		return
	}

	key, ok := c.Keys.Get(value)
	if !ok {
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
		return
	}

	_ = key
	_ = body

	var bodyIns aispec.ChatMessage
	err := json.Unmarshal([]byte(body), &bodyIns)
	if err != nil {
		log.Errorf("unmarshal body error: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}
	modelName := bodyIns.Model

	var prompt bytes.Buffer
	for _, message := range bodyIns.Messages {
		prompt.WriteString(message.Content + "\n")
	}

	if prompt.Len() == 0 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nX-Reason: empty prompt\r\n\r\n"))
		return
	}

	allowedModels, ok := c.KeyAllowedModels.Get(key.Key)
	if !ok {
		log.Errorf("key[%v] request model %s not found", key.Key, modelName)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}

	isAllowed, ok := allowedModels.Get(modelName)
	if !ok {
		log.Errorf("key[%v] request model %s not found (or not allowed) in allowed models: %v", key.Key, modelName, allowedModels.Keys())
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}

	if !isAllowed {
		log.Errorf("key[%v] request model %s not allowed", key.Key, modelName)
		conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\n"))
		return
	}

	model, ok := c.Models.Get(modelName)
	if !ok {
		log.Errorf("key[%v] request model %s not found", key.Key, modelName)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}

	log.Infof("key[%v] request model %s, start to forward", key.Key, modelName)
	_ = model
	provider := c.Entrypoints.PeekProvider(modelName)
	if provider == nil {
		log.Errorf("key[%v] request model %s not found", key.Key, modelName)
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 404 Not Found\r\nX-Reason: no provider found, contact admin to add provider for %v\r\n\r\n", modelName)))
		return
	}

	client, err := provider.GetAIClient()
	if err != nil {
		log.Errorf("key[%v] request model %s get ai client error: %v", key.Key, modelName, err)
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nX-Reason: %v\r\n\r\n", err)))
		return
	}

	conn.Write([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n"))
	utils.FlushWriter(conn)

	writer := NewChatJSONChunkWriter(conn, key.Key, modelName)
	defer writer.Close()
	client.LoadOption(aispec.WithStreamHandler(
		func(r io.Reader) {
			io.Copy(writer.GetOutputWriter(), r)
		},
	), aispec.WithReasonStreamHandler(func(r io.Reader) {
		io.Copy(writer.GetReasonWriter(), r)
	}))
	finalMsg, err := client.Chat(prompt.String())
	if err != nil {
		log.Errorf("key[%v] request model %s chat error: %v", key.Key, modelName, err)
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nX-Reason: %v\r\n\r\n", err)))
		return
	}
	_ = finalMsg
}

func (c *Config) Serve(conn net.Conn) {
	log.Infof("new connection from %s", conn.RemoteAddr())
	defer conn.Close()
	reader := bufio.NewReader(conn)
	request, err := utils.ReadHTTPRequestFromBufioReader(reader)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}
	uriIns, err := url.ParseRequestURI(request.RequestURI)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	requestRaw, err := utils.DumpHTTPRequest(request, true)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	fmt.Println(string(requestRaw))

	switch {
	case strings.HasPrefix(uriIns.Path, "/v1/chat/completions"):
		c.serveChatCompletions(conn, requestRaw)
		return
	case uriIns.Path == "/register/forward":
		fallthrough
	default:
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}
}
