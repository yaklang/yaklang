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

	newUrl, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
	if err != nil {
		return nil, err
	}

	host, port, err := utils.ParseStringToHostPort(newUrl.String())
	if err != nil {
		return nil, err
	}

	cacheKey := utils.HostPort(host, port)

	var isH2 bool

	if cached, ok := p.h2Cache.Load(cacheKey); ok {
		isH2 = cached.(bool)
	}

	var isGmTLS = p.gmTLS && isHttps

	opts := append(
		p.lowhttpConfig,
		lowhttp.WithRequest(reqBytes),
		lowhttp.WithHttp2(isH2),
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

		// filter / forward to client conn via Content-Type
		if key == "content-type" {
			if ret := httpctx.GetResponseContentTypeFiltered(req); ret != nil {
				if ret(value) {
					// filtered by content-type
					httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
						httpctx.SetMITMSkipFrontendFeedback(req, true)
						bwr.Write(headerBytes)
						utils.FlushWriter(bwr)
						httpctx.SetResponseFinishedCallback(req, func() {
							utils.FlushWriter(bwr)
						})
						return io.TeeReader(bodyReader, bwr), nil
					})
					return
				}
			}
		}

		// content-length is too short
		if key == "content-length" {
			if contentLength := codec.Atoi(value); contentLength > 0 && contentLength < p.GetMaxContentLength() {
				return
			}
		}

		// trigger: content-length is too large
		httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
			writerCloser := utils.NewTriggerWriter(uint64(p.GetMaxContentLength()), func(buffer io.ReadCloser) {
				httpctx.SetResponseTooLarge(req, true)
				httpctx.SetMITMSkipFrontendFeedback(req, true)
				bwr.Write(headerBytes)
				utils.FlushWriter(bwr)
				go func() {
					_, err := io.Copy(bwr, buffer)
					utils.FlushWriter(bwr)
					if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
						log.Errorf("io.Copy error: %s", err)
					}
				}()
			})
			httpctx.SetResponseFinishedCallback(req, func() {
				httpctx.SetResponseTooLargeSize(req, writerCloser.GetCount())
				writerCloser.Close()
			})
			return io.TeeReader(bodyReader, writerCloser), nil
		})
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
