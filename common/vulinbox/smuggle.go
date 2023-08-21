package vulinbox

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net"
	"net/http"
	"strings"
)

func Pipeline(ctx context.Context, port int) error {
	if port <= 0 {
		port = utils.GetRandomAvailableTCPPort()
	}
	ctx, cancel := context.WithCancel(ctx)
	_ = cancel
	log.Infof("start to listen Pipeline server proxy: %v", port)
	lis, err := net.Listen("tcp", ":"+fmt.Sprint(port))
	if err != nil {
		return err
	}
	go func() {
		select {
		case <-ctx.Done():
			cancel()
			lis.Close()
		}
	}()
	defer lis.Close()

	ordinaryRequest := lowhttp.FixHTTPRequestOut([]byte(`HTTP/1.1 200 OK
Server: ReverseProxy for restriction admin in VULINBOX!
Content-Type: text/html; charset=utf-8
`))
	ordinaryRequest = lowhttp.ReplaceHTTPPacketBody(
		ordinaryRequest, UnsafeRender(
			"Pipeline demo", []byte("HTTP Pipeline")),
		false,
	)

	handleRequest := func(reader *bufio.Reader, writer *bufio.Writer) {
		for {
			req, err := http.ReadRequest(reader)
			if err != nil {
				log.Error(err)
				return
			}
			log.Infof("method: %v, url: %v", req.Method, req.URL)
			raw, _ := io.ReadAll(req.Body)
			if len(raw) > 0 {
				spew.Dump(raw)
			}
			writer.Write(ordinaryRequest)
			writer.Flush()
			if req.Close {
				log.Info("close connection")
				return
			}
		}
	}

	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Error(err)
			break
		}
		br := bufio.NewReader(conn)
		bw := bufio.NewWriter(conn)
		go func() {
			handleRequest(br, bw)
			conn.Close()
		}()
	}
	return nil
}

func Smuggle(ctx context.Context, port int) error {
	if port <= 0 {
		port = utils.GetRandomAvailableTCPPort()
	}
	ctx, cancel := context.WithCancel(ctx)
	_ = cancel
	log.Infof("start to listen Pipeline server proxy: %v", port)
	lis, err := net.Listen("tcp", ":"+fmt.Sprint(port))
	if err != nil {
		return err
	}
	go func() {
		select {
		case <-ctx.Done():
			cancel()
			lis.Close()
		}
	}()
	defer lis.Close()

	ordinaryRequest := lowhttp.FixHTTPRequestOut([]byte(`HTTP/1.1 200 OK
Server: ReverseProxy for restriction admin in VULINBOX!
Content-Type: text/html; charset=utf-8
`))
	ordinaryRequest = lowhttp.ReplaceHTTPPacketBody(
		ordinaryRequest, UnsafeRender(
			"Pipeline demo", []byte("HTTP Pipeline")),
		false,
	)

	onlyHandleContentLength := func(reader *bufio.Reader, writer *bufio.Writer) ([]byte, error) {
		var proxied bytes.Buffer
		fistLine, err := utils.BufioReadLine(reader)
		if err != nil {
			return nil, err
		}
		proxied.WriteString(string(fistLine) + "\r\n")
		log.Infof("first line: %v", string(fistLine))
		var bodySize int
		for {
			line, err := utils.BufioReadLine(reader)
			if err != nil {
				log.Error(err)
				break
			}
			if string(line) == "" {
				break
			}
			k, v := lowhttp.SplitHTTPHeader(string(line))
			if strings.ToLower(k) == "content-length" {
				bodySize = utils.Atoi(v)
				log.Infof("content-length: %v", bodySize)
			} else {
				proxied.WriteString(string(line) + "\r\n")
			}
		}
		proxied.WriteString("\r\n")
		if bodySize > 0 {
			body := make([]byte, bodySize)
			_, _ = io.ReadFull(reader, body)
			proxied.Write(body)
		}
		return proxied.Bytes(), nil
	}
	handleRequest := func(ret []byte, writer *bufio.Writer) {
		for {
			chunked := false
			_, body := lowhttp.SplitHTTPPacket(ret, func(method string, requestUri string, proto string) error {
				return nil
			}, nil, func(line string) string {
				k, v := lowhttp.SplitHTTPHeader(line)
				if strings.ToLower(k) == "transfer-encoding" && v == "chunked" {
					chunked = true
				}
				return line
			})
			writer.Write(ordinaryRequest)
			writer.Flush()
			if chunked {
				var before []byte
				var ok bool
				before, ret, ok = bytes.Cut(body, []byte("0\r\n\r\n"))
				if ok {
					raw, err := codec.HTTPChunkedDecode(append(before, []byte("0\r\n\r\n")...))
					if err != nil {
						log.Error(err)
						return
					}
					spew.Dump(raw)
					continue
				}
			}
			return
		}
	}

	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Error(err)
			break
		}
		br := bufio.NewReader(conn)
		bw := bufio.NewWriter(conn)
		go func() {
			defer conn.Close()
			proxies, err := onlyHandleContentLength(br, bw)
			if err != nil {
				log.Error(err)
				return
			}
			fmt.Println(string(proxies))
			spew.Dump(proxies)
			handleRequest(proxies, bw)
			println("=====================================")
		}()
	}
	return nil
}
