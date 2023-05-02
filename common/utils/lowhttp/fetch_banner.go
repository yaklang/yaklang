package lowhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// https://github.com/bradleyfalzon/tlsx
func FetchBannerFromHostPort(
	baseCtx context.Context, host string, port interface{}, size int64, noWaitBanner bool,
	forceHTTPS bool,
	noFollowRedirect bool,
) []byte {
	var isTls = false
	targetAddr := utils.HostPort(host, port)
	dailer := &net.Dialer{Timeout: 10 * time.Second}

	if !forceHTTPS && utils.IContains(fmt.Sprint(port), "443") {
		/*
			判断是否是 TLS
		*/
		utils.Debug(func() {
			log.Infof("start to check tls for: %v", targetAddr)
		})
		conn, _ := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", utils.HostPort(host, port), &tls.Config{InsecureSkipVerify: true})
		if conn != nil {
			_ = conn.Close()
			isTls = true
		}
		utils.Debug(func() {
			log.Infof("finished to check tls for: %v, is tls?: %#v", targetAddr, isTls)
		})
		//conn, err = net.Dial("tcp", utils.HostPort(host, port))
		//if err != nil {
		//	return nil
		//}
		//_, _ = conn.Write(badTLSHello)
		//// 15 03 01 00 02 02 0a
		//tlsError, _ := utils.ReadConnWithTimeout(conn, 3*time.Second)
		//spew.Dump(tlsError)
		//panic(1)
		//if bytes.HasPrefix(tlsError, []byte{0x15, 0x03, 0x01, 0x00, 0x02}) {
		//	isTls = true
		//}
		//conn.Close()
	}

	if forceHTTPS {
		isTls = true
	}

	// 开始获取指纹
	var conn net.Conn
	var err error
	getConn := func() (net.Conn, error) {
		if !isTls {
			return dailer.Dial("tcp", utils.HostPort(host, port))
		} else {
			return tls.DialWithDialer(dailer, "tcp", utils.HostPort(host, port), &tls.Config{InsecureSkipVerify: true})
		}
	}

STARTCONN:
	conn, err = getConn()
	if err != nil {
		utils.Debug(func() {
			log.Errorf("fetch connect failed: %s", err)
		})
		if !isTls {
			isTls = true
			goto STARTCONN
		}
		return nil
	}

	if !noWaitBanner && fmt.Sprint(port) != "80" {
		utils.Debug(func() {
			log.Infof("start to check init data for %v", targetAddr)
		})
		// 如果直接连入就有数据的话，说明不需要发送请求
		res, _ := utils.ReadConnWithTimeout(conn, 1*time.Second)
		if res != nil {
			return res
		}
	}

	// 构建数据包
	packet := fmt.Sprintf(`GET / HTTP/1.1
Host: %v
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
`, utils.HostPort(host, port))
	packetRaw := FixHTTPRequestOut([]byte(packet))
	_, _ = conn.Write(packetRaw)

	raw, err := utils.ReadConnWithTimeout(conn, 10*time.Second)
	if err != nil {
		utils.Errorf("read from remove failed: %s", err)
	}
	//utils.Debug(func() {
	//	println(string(packetRaw))
	//	spew.Dump(raw)
	//})
	if !bytes.HasPrefix(raw, []byte("HTTP/")) {
		return raw
	}
	fixedRaw, _, _ := FixHTTPResponse(raw)
	if fixedRaw != nil {
		raw = fixedRaw
	}

	if !noFollowRedirect {
		rsp := raw[:]
		target := GetRedirectFromHTTPResponse(rsp, true)
		target = MergeUrlFromHTTPRequest(packetRaw, target, isTls)
		utils.Debug(func() {
			log.Infof("fetch redirect target: %v", target)
		})
		nextHost, nextPort, err := utils.ParseStringToHostPort(target)
		if err != nil {
			log.Errorf("cannot parse [%s]'s host:port... cannot follow redirect", target)
			return raw
		}
		newPacket := UrlToGetRequestPacket(target, packetRaw, isTls, ExtractCookieJarFromHTTPResponse(rsp)...)
		packetRaw = newPacket
		newResponse, _, _ := SendHTTPRequestWithRawPacketWithRedirect(isTls, nextHost, nextPort, newPacket, 10*time.Second, 5)
		if newResponse == nil {
			utils.Debug(func() {
				log.Errorf("send http request raw failed: %s", err)
			})
			return raw
		}
		raw = append(raw, []byte(CRLF+CRLF)...)
		return append(raw, newResponse...)
	}

	return raw
}
