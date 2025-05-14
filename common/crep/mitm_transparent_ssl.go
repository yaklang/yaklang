package crep

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/netx"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	setHijackRequestLock = new(sync.Mutex)
	fallbackHttpFrame    = []byte(`HTTP/1.1 200 OK
Content-Length: 11

origin_fail

`)
)

func (m *MITMServer) ServeTransparentTLS(ctx context.Context, addr string) error {
	if m.mitmConfig == nil {
		return utils.Errorf("mitm config empty")
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return utils.Errorf("listen tcp://%v failed; %s", addr, err)
	}

	addrVerbose := fmt.Sprintf("tcp://%v", addr)
	go func() {
		log.Infof("start to server transparent mitm server: tcp://%v", addr)
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Errorf("%v accept new conn failed: %s", addrVerbose, err)
				return
			}

			log.Infof("recv tcp conn from %v", conn.RemoteAddr().String())
			_ = conn
			go func() {
				defer conn.Close()

				log.Infof("start check tls/http connection... for %s", conn.RemoteAddr().String())
				err := m.handleHTTPS(ctx, conn, addr)
				if err != nil {
					log.Errorf("handle conn from [%s] failed: %s", conn.RemoteAddr().String(), err)
					return
				}
			}()
		}
	}()

	select {
	case <-ctx.Done():
		return l.Close()
	}
}

func (m *MITMServer) handleHTTPS(ctx context.Context, conn net.Conn, origin string) error {
	pc := utils.NewPeekableNetConn(conn)
	raw, err := pc.Peek(1)
	if err != nil {
		return utils.Errorf("peek [%s] failed: %s", conn.RemoteAddr(), err)
	}

	log.Infof("peek one char[%#v] for %v", raw, conn.RemoteAddr().String())

	isHttps := utils.NewAtomicBool()
	var httpConn net.Conn
	var sni string
	switch raw[0] {
	case 0x16: // https
		log.Infof("serving https for: %s", conn.RemoteAddr().String())
		tconn := gmtls.Server(pc, m.mitmConfig.TLS())
		err := tconn.Handshake()
		if err != nil {
			return utils.Errorf("tls handshake failed: %s", err)
		}
		log.Infof("conn: %s handshake finished", conn.RemoteAddr().String())
		httpConn = tconn
		sni = tconn.ConnectionState().ServerName
		isHttps.Set()
	default: // http
		log.Infof("start to serve http for %s", conn.RemoteAddr().String())
		httpConn = pc
		isHttps.UnSet()
	}

	// log.Infof("parse req http finished: %v", spew.Sdump(req))
	if httpConn == nil {
		return nil
	}

	if sni == "" {
		return utils.Errorf("SNI empty...")
	}

	log.Infof("start to handle http request for %s", conn.RemoteAddr().String())
	var readerBuffer bytes.Buffer
	reqReader := io.TeeReader(httpConn, &readerBuffer)
	firstRequest, err := utils.ReadHTTPRequestFromBufioReader(bufio.NewReader(reqReader))
	if err != nil {
		return utils.Errorf("read request failed: %s for %s", err, conn.RemoteAddr().String())
	}
	log.Infof("read request finished for %v", httpConn.RemoteAddr())

	var fakeUrl string
	if isHttps.IsSet() {
		fakeUrl = fmt.Sprintf("https://%v", firstRequest.Host)
	} else {
		fakeUrl = fmt.Sprintf("http://%v", firstRequest.Host)
	}

	// 设置超时和 context 控制
	var timeout time.Duration = 30 * time.Second
	var ctxDDL time.Time
	if ddl, ok := ctx.Deadline(); ok {
		ctxDDL = ddl
		timeout = ddl.Sub(time.Now())
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
	}

	host, port, err := utils.ParseStringToHostPort(fakeUrl)
	if err != nil {
		return utils.Errorf("cannot identify target[%s]: %s", fakeUrl, err)
	}

	originHost := host
	if (!utils.IsIPv4(host)) && (!utils.IsIPv6(utils.FixForParseIP(host))) {
		log.Infof("start to handle dns items for %v", host)
		cachedTarget, ok := m.dnsCache.Load(host)
		if !ok {
			target := netx.LookupFirst(host, netx.WithTimeout(timeout), netx.WithDNSServers(m.DNSServers...))
			if target == "" {
				// httpConn.Write(fallbackHttpFrame)
				return utils.Errorf("cannot query dns host[%s]", host)
			}
			log.Infof("dns query finished for %v: results: [%#v]", host, target)
			host = target
			m.dnsCache.Store(host, target)
		} else {
			_h := cachedTarget.(string)
			log.Infof("dns cache matched: %v -> %v", host, _h)
			host = _h
		}
	}
	target := utils.HostPort(host, port)

	// 如果是环回，就返回一个自定义内容
	if utils.HostPort(host, port) == origin {
		log.Infof("lookback: %s", origin)
		httpConn.Write(fallbackHttpFrame)
		return nil
	}

	log.Infof("start to connect remote addr: %v", target)
	dialer := &net.Dialer{
		Timeout:  timeout,
		Deadline: ctxDDL,
	}

	var remoteConn net.Conn
	if !isHttps.IsSet() {
		log.Infof("tcp connect to %s", target)
		remoteConn, err = dialer.Dial("tcp", target)
		if err != nil {
			return utils.Errorf("remote tcp://%v failed to dial: %s", utils.HostPort(host, port), err)
		}
	} else {
		log.Infof("tcp+tls connect to %s", target)
		minVer, maxVer := consts.GetGlobalTLSVersion()
		remoteConn, err = tls.DialWithDialer(dialer, "tcp", target, &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         minVer,
			MaxVersion:         maxVer,
			ServerName:         originHost,
		})
		if err != nil {
			return utils.Errorf("remote tcp+tls://%v failed: %s", target, err)
		}
	}
	defer remoteConn.Close()

	// 以下是转发模式的, 不做劫持
	if m.transparentHijackMode == nil || !m.transparentHijackMode.IsSet() {
		// 在透明模式里面，所有的回调都不生效
		_, err = remoteConn.Write(readerBuffer.Bytes())
		if err != nil {
			return utils.Errorf("write first http.Request raw []byte failed: %s", err)
		}
		if firstRequest.Body != nil {
			n, _ := io.Copy(remoteConn, firstRequest.Body)
			log.Errorf("request have body len: %v", n)
		}

		wg := new(sync.WaitGroup)
		wg.Add(2)
		log.Infof("start to do transparent traffic")
		go func() {
			defer wg.Done()
			defer remoteConn.Close()
			io.Copy(remoteConn, httpConn)
		}()

		go func() {
			defer wg.Done()
			defer remoteConn.Close()
			io.Copy(httpConn, remoteConn)
		}()
		defer log.Infof("finished conn from %s to %s", conn.LocalAddr().String(), conn.RemoteAddr().String())
		wg.Wait()
	} else {
		// 接下来是如何进行网络交互？
		// 透明模式，劫持开启之后回调才会生效

		// 劫持第一个 request
		reqBytes := readerBuffer.Bytes()
		if m.transparentHijackRequest == nil && m.transparentHijackRequestManager == nil {
			// 不劫持请求的时候，直接写，不要等待全部读完
			log.Infof("write first request for %v", remoteConn.RemoteAddr().String())
			_, err = remoteConn.Write(reqBytes)
			if err != nil {
				return utils.Errorf("write first http.Request raw []byte failed: %s", err)
			}
			if firstRequest.Body != nil {
				n, _ := io.Copy(remoteConn, firstRequest.Body)
				log.Errorf("request have body len: %v", n)
			}
			log.Infof("write first request finished for %v", remoteConn.RemoteAddr().String())
		} else {
			// 劫持场景下的处理第一个数据包
			if firstRequest.Body != nil {
				_, _ = ioutil.ReadAll(firstRequest.Body)
			}

			reqBytes = readerBuffer.Bytes()
			if m.transparentHijackRequest != nil {
				reqBytes = m.transparentHijackRequest(isHttps.IsSet(), reqBytes)
			}

			if m.transparentHijackRequestManager != nil {
				reqBytes = m.transparentHijackRequestManager.Hijacked(isHttps.IsSet(), reqBytes)
			}

			remoteConn.Write(reqBytes)

		}

		var rspRaw bytes.Buffer
		// 解析 response
		responseReader := io.TeeReader(remoteConn, &rspRaw)

		// 不劫持响应的话，读多少写多少保证速度
		if m.transparentHijackResponse == nil {
			responseReader = io.TeeReader(responseReader, httpConn)
		}

		// 构建响应，这个响应很关键
		rsp, err := utils.ReadHTTPResponseFromBufioReader(bufio.NewReader(responseReader), firstRequest)
		if err != nil {
			return utils.Errorf("read response for req[%v]->%v failed: %s", firstRequest.URL.String(), remoteConn.RemoteAddr(), err)
		}

		// 解析 Body，这个 body 是从 remote -> local 的
		// 不劫持详情的情况下，正常读完就行了，不用在乎太多
		// 劫持的时候，读取并不会直接写入，需要手动 httpConn.Write
		if rsp.Body != nil {
			rspBody, _ := ioutil.ReadAll(rsp.Body)
			if len(rspBody) > 0 {
				log.Infof("rsp body found length: %v", len(rspBody))
			}
		}
		log.Info("first req and rsp recv finished!")

		rspBytes := rspRaw.Bytes()
		// 劫持响应的话，要手动写 httpConn, 但是必须读完才能劫持，所以这里可能会影响速度
		if m.transparentHijackResponse != nil {
			rspBytes = m.transparentHijackResponse(isHttps.IsSet(), rspBytes)
			_, err = httpConn.Write(rspBytes)
			if err != nil {
				return utils.Errorf("feedback response bytes from [%s] to [%s] failed: %s",
					remoteConn.RemoteAddr().String(), httpConn.RemoteAddr().String(), err,
				)
			}
		}

		if m.transparentOriginMirror != nil {
			go m.transparentOriginMirror(isHttps.IsSet(), readerBuffer.Bytes(), rspRaw.Bytes())
		}

		if m.transparentHijackedMirror != nil {
			go m.transparentHijackedMirror(isHttps.IsSet(), reqBytes, rspBytes)
		}

		if rsp.Close {
			return nil
		}

		for {
			// 读取 request
			var reqRaw bytes.Buffer

			// 这里是移除一些没有用的不符合 HTTP 协议前缀请求的字符
			buf := make([]byte, 1)
			for {
				_, err := httpConn.Read(buf)
				if err != nil {
					return utils.Errorf("httpConn read failed: %s", err)
				}
				if len(buf) > 0 {
					firstByte := buf[0]
					if ('A' <= firstByte && 'Z' >= firstByte) || (firstByte >= 91 && firstByte <= 122) {
						break
					} else {
						continue
					}
				}
			}

			reqReader := io.TeeReader( // 从本地读 http.Request 出来
				io.MultiReader(bytes.NewReader(buf), httpConn),
				&reqRaw,
			)

			// 如果不劫持，读多少转发多少
			if m.transparentHijackRequest == nil && m.transparentHijackRequestManager == nil {
				reqReader = io.TeeReader(reqReader, remoteConn)
			}
			req, err := utils.ReadHTTPRequestFromBufioReader(bufio.NewReader(reqReader))
			if err != nil {
				return utils.Errorf("read http request from: %s failed: %s", httpConn.RemoteAddr().String(), err)
			}

			// 这个目的是为了把 body 的缓冲区读完，如果劫持了请求，会同步写入到 remoteConn 中
			// 如果这里没有劫持请求，则不会发生什么奇怪的事情，仅仅读出来，在 reqRaw 中收结果吧
			if req.Body != nil {
				_, _ = ioutil.ReadAll(req.Body)
			}

			// 劫持请求，这个 reqRaw 一定是包含 body 的了（如果可能）
			reqBytes := reqRaw.Bytes()
			switch true {
			case m.transparentHijackRequest != nil:
				reqBytes = m.transparentHijackRequest(isHttps.IsSet(), reqBytes)
				_, err = remoteConn.Write(reqBytes)
				if err != nil {
					return utils.Errorf("write http request from [%v] to [%s] failed: %s", httpConn.RemoteAddr().String(), remoteConn.RemoteAddr().String(), err)
				}
			case m.transparentHijackRequestManager != nil:
				reqBytes = m.transparentHijackRequestManager.Hijacked(isHttps.IsSet(), reqBytes)
				_, err := remoteConn.Write(reqBytes)
				if err != nil {
					return utils.Errorf("write http request from [%v] to [%s] failed: %s", httpConn.RemoteAddr().String(), remoteConn.RemoteAddr().String(), err)
				}
			}

			// 读取 response
			var rspRaw bytes.Buffer
			remoteResponseReader := io.TeeReader(remoteConn, &rspRaw)
			if m.transparentHijackResponse == nil {
				remoteResponseReader = io.TeeReader(remoteResponseReader, httpConn)
			}
			rsp, err := utils.ReadHTTPResponseFromBufioReader(bufio.NewReader(remoteResponseReader), req)
			if err != nil {
				return utils.Errorf("read http response from: %s failed: %s", remoteConn.RemoteAddr().String(), err)
			}

			// 类似上面的代码，这个是为了读缓冲区出来
			if rsp.Body != nil {
				_, _ = ioutil.ReadAll(rsp.Body)
			}

			// 镜像流量，这个流量是没有劫持过得！
			if m.transparentOriginMirror != nil {
				go m.transparentOriginMirror(isHttps.IsSet(), reqRaw.Bytes(), rspRaw.Bytes())
			}

			// 劫持返回结果
			// 这里的劫持，并没有自动写入 httpConn，所以需要手动写入，这里是同步操作，性能瓶颈在这里
			rspBytes := rspRaw.Bytes()
			if m.transparentHijackResponse != nil {
				rspBytes = m.transparentHijackResponse(isHttps.IsSet(), rspBytes)
				_, err = httpConn.Write(rspBytes)
				if err != nil {
					return utils.Errorf("write http response from [%s] to [%s] failed: %s", remoteConn.RemoteAddr().String(), httpConn.RemoteAddr().String(), err)
				}
			}

			// 劫持后的镜像流量
			if m.transparentHijackedMirror != nil {
				go m.transparentHijackedMirror(isHttps.IsSet(), reqBytes, rspBytes)
			}

			// 当前 req/rsp 处理完毕，并且 response 要求关闭，关闭前一定要信息传输回去
			if rsp.Close {
				return nil
			}
		}
	}

	return nil
}
