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

func (s *VulinServer) registerPipelineNSmuggle() {
	smugglePort := utils.GetRandomAvailableTCPPort()
	pipelinePort := utils.GetRandomAvailableTCPPort()

	pipelineNSmuggleSubroute := s.router.PathPrefix("/http/protocol").Name("HTTP CDN 与 Pipeline 安全").Subrouter()
	go func() {
		err := Smuggle(context.Background(), smugglePort)
		if err != nil {
			log.Error(err)
		}
	}()
	err := utils.WaitConnect(utils.HostPort("127.0.0.1", smugglePort), 3)
	if err != nil {
		log.Error(err)
		return
	}
	addRouteWithVulInfo(pipelineNSmuggleSubroute, &VulInfo{
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Location", "http://"+utils.HostPort("127.0.0.1", smugglePort))
			writer.WriteHeader(302)
		},
		Path:  `/smuggle`,
		Title: "HTTP 请求走私案例：HTTP Smuggle",
	})

	go func() {
		err := Pipeline(context.Background(), pipelinePort)
		if err != nil {
			log.Error(err)
		}
	}()
	err = utils.WaitConnect(utils.HostPort("127.0.0.1", pipelinePort), 3)
	if err != nil {
		log.Error(err)
		return
	}
	addRouteWithVulInfo(pipelineNSmuggleSubroute, &VulInfo{
		Path: `/pipeline`,
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Location", "http://"+utils.HostPort("127.0.0.1", pipelinePort))
			writer.WriteHeader(302)
		},
		Title: "HTTP Pipeline 正常案例（对照组，并不是漏洞）",
	})
}

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
			"HTTP/1.1 Pipeline", []byte(`
在 HTTP/1.1 中，默认 Connection: keep-alive 被设置, <br>
在这种情况下，如果客户端发送了多个请求，服务器会将这些请求串行化，<br>
也就是说，只有前一个请求完成，才会进行下一个请求。<br>
<br>
一般来说，这并不会导致安全问题
`)),
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
			"HTTP/1.1 Smuggle", []byte(`
在代理服务器反向代理真实服务器的业务场景下，<br>
如果代理服务器没有正确处理 Transfer-Encoding: chunked 和 Content-Length: ... 的优先级 <br>
那么，黑客可能会构造一个特殊的请求，这个请求在代理服务器和真实服务器中的解析结果不一致，<br>
从而导致安全问题。<br>
<br>
例如在本端口的服务器中：代理服务器会认为这是一个按照 Content-Length 读 Body 的 HTTP/1.1 的请求，<br>
代理服务器将会读取到 Body 中的数据，并且拼接传递给真实服务器，<br>
而真实服务器会认为这是一个按照 Transfer-Encoding: chunked 读 Body 的 HTTP/1.1 的请求，把请求进行错误的分割，同时读到了两个请求造成安全问题<br>
<br>
`)),
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
				bodySize = codec.Atoi(v)
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
