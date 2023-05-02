package facades

import (
	"bufio"
	"github.com/h2non/filetype"
	"net"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const defaultHTTPFallback = "HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 0\r\n\r\n\r\n\r\n"

// var HttpRoutes sync.Map
type HttpResourceType string

const (
	FileResponseHeader = `HTTP/1.1 200 OK
Connection: close
Accept-Ranges: bytes
Content-Encoding: identity
Content-Type: application/octet-stream
Content-Length: 0

`
	BodyResponseHeader = `HTTP/1.1 200 OK
Connection: close
Accept-Ranges: bytes
Content-Encoding: identity
Content-Length: 0

`
)

const (
	HttpResourceType_File HttpResourceType = "file"
	HttpResourceType_Body HttpResourceType = "body"
	HttpResourceType_Raw  HttpResourceType = "raw"
)

type HttpResource struct {
	//response func(req *http.Request, c net.Conn) error
	resource      interface{}
	responseType  HttpResourceType
	times         int
	disableNotify bool
}

// 如果路由已存在，则跳过
func (f *FacadeServer) AddHttpRoute(pattern string, rsc *HttpResource) {
	f.httpMux.Lock()
	_, ok := f.httpResource[pattern]
	if !ok {
		f.httpResource[pattern] = rsc
	}
	f.httpMux.Unlock()
}

func (f *FacadeServer) AddFileResource(pattern string, resource []byte) {
	f.AddHttpRoute(pattern, &HttpResource{resource: resource, responseType: HttpResourceType_File, times: -1})
}

func (f *FacadeServer) SetRawResource(pattern string, resource []byte) {
	f.SetRawResourceEx(pattern, resource, false)
}

func (f *FacadeServer) SetRawResourceEx(pattern string, resource []byte, disableNotify bool) {
	f.SaveHttpRoute(pattern, &HttpResource{resource: resource, responseType: HttpResourceType_Raw, times: -1, disableNotify: disableNotify})
}

func (f *FacadeServer) RemoveHTTPResource(pattern string) {
	f.httpMux.Lock()
	delete(f.httpResource, pattern)
	f.httpMux.Unlock()
}

func (f *FacadeServer) OverwriteFileResource(pattern string, resource []byte) {
	f.SaveHttpRoute(pattern, &HttpResource{resource: resource, responseType: HttpResourceType_File, times: -1})
}

// 如果路由已经存在，则覆盖原有的路由
func (f *FacadeServer) SaveHttpRoute(pattern string, resource *HttpResource) {
	f.httpMux.Lock()
	f.httpResource[pattern] = resource
	f.httpMux.Unlock()
}

func (f *FacadeServer) GetHTTPHandler(isHttps bool) FacadeConnectionHandler {
	return func(peekConn *utils.BufferedPeekableConn) error {
		var c net.Conn = peekConn
		c.SetDeadline(time.Now().Add(3 * time.Second))
		log.Infof("start to read http request from %s", c.RemoteAddr())
		//reader := io.TeeReader(c, os.Stdout)
		req, err := lowhttp.ReadHTTPRequest(bufio.NewReader(c))
		if err != nil {
			log.Errorf("read http request from conn[%s] failed", c.RemoteAddr())
			return err
		}

		log.Infof("request is received from %s", c.RemoteAddr())
		reqRaw, err := utils.HttpDumpWithBody(req, true)
		if err != nil {
			log.Errorf("dump http request failed: %s", err)
			return err
		}
		//originToken := req.RequestURI
		token := req.RequestURI
		//for strings.HasPrefix(token, "/") {
		//	token = token[1:]
		//}

		var msgType string
		if isHttps {
			msgType = "https"
		} else {
			msgType = "http"
		}
		for pattern, response := range f.httpResource {
			if utils.MatchAllOfGlob(pattern, token) && response != nil {
				switch response.responseType {
				case HttpResourceType_Raw:
					responseRaw := utils.InterfaceToBytes(response.resource)
					rsp, _, _ := lowhttp.FixHTTPResponse(responseRaw)
					if rsp != nil {
						responseRaw = rsp
					}
					c.Write(rsp)
					c.Close()

					if !response.disableNotify {
						f.triggerNotificationEx(msgType, peekConn.Conn, token, reqRaw, token)
					}
					response.times -= 1
					if response.times == 0 {
						delete(f.httpResource, pattern)
					}
					return nil
				case HttpResourceType_Body:
					responseBody := utils.InterfaceToBytes(response.resource)
					var mimeType = "text/html"
					t, _ := filetype.Match(responseBody)
					if t.Extension != "" {
						mimeType = t.MIME.Value
					}
					raw := lowhttp.ReplaceHTTPPacketBody(lowhttp.ReplaceMIMEType([]byte(BodyResponseHeader), mimeType), responseBody, false)
					fixed, _, _ := lowhttp.FixHTTPResponse(raw)
					if fixed != nil {
						raw = fixed
					}
					c.Write(fixed)
					c.Close()

					if !response.disableNotify {
						f.triggerNotificationEx(msgType, peekConn.Conn, token, reqRaw, token)
					}
					response.times -= 1
					if response.times == 0 {
						delete(f.httpResource, pattern)
					}
					return nil
				case HttpResourceType_File:
					resource := utils.InterfaceToBytes(response.resource)
					respb := append([]byte(FileResponseHeader), resource...)
					resp, body, err := lowhttp.FixHTTPResponse(respb)
					if err != nil {
						log.Error("parse get request error")
						return nil
					}
					res := append(resp, body...)
					c.SetDeadline(time.Now().Add(3 * time.Second))
					c.Write(res)
					c.Close()

					if !response.disableNotify {
						f.triggerNotificationEx(msgType, peekConn.GetOriginConn(), token, reqRaw, token)
					}
					response.times -= 1
					if response.times == 0 {
						delete(f.httpResource, pattern)
					}
					return nil
				}
			}
		}

		f.triggerNotificationEx(msgType, peekConn.GetOriginConn(), token, reqRaw, "<empty>")
		c.SetDeadline(time.Now().Add(3 * time.Second))
		c.Write([]byte(defaultHTTPFallback))
		c.Close()
		return nil
	}
}
