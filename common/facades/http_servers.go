package facades

import (
	"bufio"
	"errors"
	"github.com/h2non/filetype"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net"
	"time"
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
	disableNotify bool
}

func NewHttpRawResource(data []byte) *HttpResource {
	return &HttpResource{
		responseType: HttpResourceType_Raw,
		resource:     data,
	}
}
func NewHttpFileResource(data []byte) *HttpResource {
	return &HttpResource{
		responseType: HttpResourceType_File,
		resource:     data,
	}
}

func (f *FacadeServer) SetRawResourceEx(pattern string, resource []byte, disableNotify bool) {
	f.SaveHttpRoute(pattern, &HttpResource{resource: resource, responseType: HttpResourceType_Raw, disableNotify: disableNotify})
}

func (f *FacadeServer) RemoveHTTPResource(pattern string) {
	f.httpMux.Lock()
	f.httpResource.DeleteResource(pattern)
	f.httpMux.Unlock()
}

func (f *FacadeServer) SetResource(protocol string, name string, token string, resource any) error {
	switch protocol {
	case "rmi":
		res, ok := resource.([]byte)
		if ok {
			f.rmiResourceAddrs.SetResource(token, res, name)
		} else {
			return errors.New("expect bytes type")
		}
	case "ldap":
		res, ok := resource.(map[string]any)
		if ok {
			f.ldapResourceAddrs.SetResource(token, res, name)
		} else {
			return errors.New("expect map type")
		}
	case "http":
		res, ok := resource.([]byte)
		if ok {
			f.httpResource.SetResource(token, NewHttpRawResource(res), name)
		} else {
			return errors.New("expect bytes type")
		}
	}
	return nil
}
func (f *FacadeServer) SetHttpRawResource(pattern string, resource []byte) {
	f.SaveHttpRoute(pattern, &HttpResource{resource: resource, responseType: HttpResourceType_File})
}

// 如果路由已经存在，则覆盖原有的路由
func (f *FacadeServer) SaveHttpRoute(pattern string, resource *HttpResource) {
	f.httpMux.Lock()
	f.httpResource.SetResource(pattern, resource, "")
	f.httpMux.Unlock()
}

func (f *FacadeServer) getHTTPHandler(isHttps bool) FacadeConnectionHandler {
	return func(peekConn *utils.BufferedPeekableConn) error {
		var c net.Conn = peekConn
		c.SetDeadline(time.Now().Add(3 * time.Second))
		log.Infof("start to read http request from %s", c.RemoteAddr())
		req, err := utils.ReadHTTPRequestFromBufioReader(bufio.NewReader(c))
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
		// TODO: has multi read map risk
		for k, v := range f.httpResource.Resources {
			pattern := k
			response := v.Resource
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
