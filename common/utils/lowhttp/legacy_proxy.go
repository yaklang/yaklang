package lowhttp

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func BuildLegacyProxyRequest(req []byte, connectHTTPS ...bool) ([]byte, error) {
	var packetRequest bytes.Buffer
	var writePath bool
	var headerBytes bytes.Buffer
	var originPath string
	var originProto string

	schema := "http://"
	if len(connectHTTPS) > 0 && connectHTTPS[0] {
		schema = "https://"
	}

	_, body := SplitHTTPPacket(req, func(method, path, proto string) error {
		packetRequest.WriteString(method)
		originProto = proto
		if utils.IsHttpOrHttpsUrl(path) {
			writePath = true
			packetRequest.WriteString(" ")
			packetRequest.WriteString(path)
			packetRequest.WriteString(" ")
			packetRequest.WriteString(proto)
		} else {
			originPath = path
		}
		return nil
	}, nil, func(line string) string {
		header, value := SplitHTTPHeader(line)
		switch header {
		case "Host", "host":
			if writePath {
				break
			}
			writePath = true
			packetRequest.WriteByte(' ')
			if utils.IsHttpOrHttpsUrl(value) {
				packetRequest.WriteString(value)
			} else {
				packetRequest.WriteString(schema)
				packetRequest.WriteString(value)
			}
			if strings.HasSuffix(value, "/") || strings.HasPrefix(originPath, "/") {
				packetRequest.WriteString(originPath)
			} else {
				packetRequest.WriteByte('/')
				packetRequest.WriteString(originPath)
			}

			packetRequest.WriteByte(' ')
			packetRequest.WriteString(originProto)
		}
		headerBytes.WriteString(line)
		headerBytes.WriteString(CRLF)
		return line
	})

	if !writePath {
		return nil, utils.Errorf("invalid http request for legacy proxy request: %s", req)
	}

	// curl like, proxy should remove it
	// ignore it
	headerBytes.WriteString(`Proxy-Connection: keep-alive`)
	headerBytes.WriteString(CRLF)
	headerBytes.WriteString(`Connection: close`)
	headerBytes.WriteString(CRLF)

	packetRequest.WriteString(CRLF)
	packetRequest.Write(headerBytes.Bytes())
	packetRequest.WriteString(CRLF)
	packetRequest.Write(body)
	return packetRequest.Bytes(), nil
}
