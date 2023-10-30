package minimartian

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net/http"
	"strings"
)

func (p *Proxy) doHTTPRequest(ctx *Context, req *http.Request) (*http.Response, error) {
	if ctx.SkippingRoundTrip() {
		log.Debugf("mitm: skipping round trip")
		return proxyutil.NewResponse(200, nil, req), nil
	}
	if httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped) {
		log.Debugf("mitm: skipping round trip due to user manually drop")
		return proxyutil.NewResponse(200, strings.NewReader(proxyutil.GetErrorRspBody("请求被用户丢弃")), req), nil
	}

	httpctx.SetRequestHTTPS(req, ctx.GetSessionBoolValue(httpctx.REQUEST_CONTEXT_KEY_IsHttps))
	inherit := func(i string) {
		httpctx.SetContextValueInfoFromRequest(req, i, ctx.GetSessionStringValue(i))
	}
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedTo)
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
	return p.execLowhttp(req)
}

func (p *Proxy) execLowhttp(req *http.Request) (*http.Response, error) {
	bareBytes := httpctx.GetRequestBytes(req)
	reqBytes := lowhttp.FixHTTPRequest(bareBytes)

	var isHttps = httpctx.GetRequestHTTPS(req)

	var isGmTLS = p.gmTLS && isHttps

	opts := append(
		p.lowhttpConfig,
		lowhttp.WithRequest(reqBytes),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithGmTLS(isGmTLS),
		lowhttp.WithConnPool(true),
		lowhttp.WithSaveHTTPFlow(false),
		lowhttp.WithNativeHTTPRequestInstance(req),
	)

	if connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort); connectedPort > 0 {
		portValid := (connectedPort == 443 && isHttps) || (connectedPort == 80 && !isHttps)
		if !portValid {
			//修复host和port
			if host := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost); host != "" {
				opts = append(opts, lowhttp.WithHost(host))
			}
			opts = append(opts, lowhttp.WithPort(connectedPort))
		}
	}

	httpctx.SetResponseHeaderParsed(req, func(key string, value string) {
		bwr := httpctx.GetMITMFrontendReadWriter(req)
		if bwr == nil {
			return
		}

		if key == "content-type" {
			if ret := httpctx.GetResponseContentTypeFiltered(req); ret != nil {
				if ret(value) {
					// filtered by content-type
					log.Infof("content-type: %v is filtered", value)
					httpctx.SetMITMSkipFrontendFeedback(req, true)
					httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
						bwr.Write(headerBytes)
						utils.FlushWriter(bwr)
						bodyReader = io.TeeReader(bodyReader, bwr)
						return bodyReader, nil
					})
					return
				}
			}
		}

		if key == "transfer-encoding" && value == "chunked" {
			httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
				return bodyReader, nil
			})
			return
		}

		if key == "content-length" {
			if contentLength := codec.Atoi(value); contentLength > 0 && contentLength > p.GetMaxContentLength() && httpctx.GetMITMFrontendReadWriter(req) != nil {
				// too large
				httpctx.SetResponseTooLarge(req, true)
				httpctx.SetMITMSkipFrontendFeedback(req, true)
				httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
					bwr.Write(headerBytes)
					utils.FlushWriter(bwr)
					bodyReader = io.TeeReader(bodyReader, bwr)
					return bodyReader, nil
				})
				return
			}
		}
	})

	lowHttpResp, err := lowhttp.HTTPWithoutRedirect(opts...)
	if err != nil {
		return nil, err
	}

	if lowHttpResp.RemoteAddr != "" {
		httpctx.SetRemoteAddr(req, lowHttpResp.RemoteAddr)
		req.RemoteAddr = lowHttpResp.RemoteAddr
	}

	rsp, err := lowhttp.ParseBytesToHTTPResponse(lowHttpResp.RawPacket)
	if rsp != nil {
		rsp.Request = req
	}

	utils.FixHTTPResponseForGolangNativeHTTPClient(rsp)
	return rsp, err
}
