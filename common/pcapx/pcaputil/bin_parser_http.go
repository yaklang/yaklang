package pcaputil

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var errBinContext = errors.New("protocol context required")

type binHTTPState struct {
	ws                                                             *binWebSocket
	requestID                                                      uint64
	wsKey, wsOffer                                                 string
	header, total, cursor, next, search                            int
	chunked, trailers, closeDelimited, response, tunnel, websocket bool
	method, summary                                                string
	status                                                         int
	doh, ipp                                                       bool
}

func (h *binHTTPState) config() map[string]any {
	if !h.response {
		return nil
	}
	return map[string]any{"httpResponseToMethod": h.method, "httpCloseDelimited": h.closeDelimited}
}

func (f *binFlow) frameDirection(dir int, w []byte) (int, *binSpec, error) {
	if f.binding == nil {
		switch f.protocol {
		case "vnc":
			return f.frameRFB(dir, w)
		case "diameter":
			return f.frameDiameter(w)
		case "iec104":
			return f.frameIEC104(w)
		case "s7comm":
			return f.frameS7(w)
		case "opcua":
			return f.frameOPCUA(w)
		case "rtsp":
			return f.frameRTSP(w)
		case "stun":
			return f.frameSTUN(w)
		case "http2":
			return f.frameHTTP2(dir, w)
		case "mysql":
			return f.frameMySQL(dir, w)
		case "postgresql":
			return f.framePostgres(dir, w)
		case "ldap":
			return f.frameLDAP(w)
		case "redis":
			return f.frameRedis(w)
		case "websocket":
			return f.frameWebSocket(dir, w)
		case "mqtt":
			if f.mqtt != nil {
				return f.frameMQTT(w)
			}
		case "socks5":
			return f.frameSOCKS5(dir, w)
		case "finger", "whois", "gopher", "dict":
			return f.frameTextInternet(dir, w)
		case "mongodb":
			return f.frameMongo(w)
		case "kafka":
			return f.frameKafka(dir, w)
		case "tds":
			return f.frameTDS(w)
		case "amqp":
			return f.frameAMQP(w)
		case "smb2":
			return f.frameSMB2(w)
		case "dcerpc":
			return f.frameDCERPC(w)
		case "ssh":
			return f.frameSSH(dir, w)
		case "nfs":
			return f.frameNFS(w)
		case "snmp":
			return f.frameSNMP(w)
		case "rdp":
			return f.frameRDP(w)
		case "dot":
			return f.frameDoT(w)
		case "sip":
			return f.frameSIP(dir, w)
		case "rtp":
			return f.frameRTP(w)
		case "quic":
			return f.frameQUIC(w)
		case "smtp":
			return f.frameSMTP(dir, w)
		case "imap":
			return f.frameIMAP(dir, w)
		case "pop3":
			return f.framePOP3(w)
		case "ftp":
			return f.frameFTP(w)
		case "tns":
			return f.frameTNS(w)
		case "radius":
			return f.frameRADIUS(w)
		case "dhcp":
			return f.frameDHCP(w)
		case "ntp":
			return f.frameNTP(w)
		case "coap":
			return f.frameCoAP(w)
		case "modbus":
			return f.frameModbus(w)
		case "enip":
			return f.frameENIP(w)
		case "stratum":
			return f.frameStratum(w)
		case "gearman":
			return f.frameGearman(w)
		case "beanstalkd":
			return f.frameBeanstalk(dir, w)
		case "scgi":
			return f.frameSCGI(dir, f.scgi.clientDir, w)
		case "msgpack-rpc":
			return f.frameMessagePackRPC(w)
		case "zookeeper":
			return f.frameZooKeeper(w)
		case "clickhouse":
			return f.frameClickHouse(dir, w)
		case "dnp3":
			return f.frameDNP3(w)
		case "c37118":
			return f.frameC37118(w)
		case "goose":
			return f.frameGOOSE(w)
		case "syslog":
			return f.frameSyslog(w)
		}
	}
	if f.protocol != "http" || f.binding != nil {
		return f.frame(w)
	}
	d := &f.directions[dir]
	h := d.http
	if h == nil {
		end := incrementalHTTPIndex(w, 0, &d.headerScan, "\r\n\r\n")
		if end < 0 {
			if len(w) > 32<<10 {
				return 0, nil, fmt.Errorf("HTTP header exceeds 32 KiB")
			}
			return 0, nil, nil
		}
		end += 4
		if end > 32<<10 {
			return 0, nil, fmt.Errorf("HTTP header exceeds 32 KiB")
		}
		lower := bytes.ToLower(w[:end])
		if bytes.Contains(lower, []byte("\r\ncontent-length:")) && bytes.Contains(lower, []byte("\r\ntransfer-encoding:")) {
			return 0, nil, fmt.Errorf("HTTP has conflicting length and transfer encoding")
		}
		h = &binHTTPState{header: end, cursor: end, summary: string(bytes.SplitN(w[:end], []byte("\r\n"), 2)[0])}
		r := bufio.NewReader(bytes.NewReader(w[:end]))
		var length int64
		var transfer []string
		if bytes.HasPrefix(w, []byte("HTTP/")) {
			h.response = true
			if len(f.httpMethods) == 0 {
				return 0, nil, fmt.Errorf("%w: HTTP response has no observed request method", errBinContext)
			}
			h.method = f.httpMethods[0]
			response, err := http.ReadResponse(r, &http.Request{Method: h.method})
			if err != nil {
				return 0, nil, err
			}
			if response.Proto != "HTTP/1.0" && response.Proto != "HTTP/1.1" {
				return 0, nil, fmt.Errorf("unsupported HTTP version")
			}
			h.status = response.StatusCode
			// Interim responses do not complete the queued request and carry
			// no DNS message. Associate only this request's final response.
			h.ipp = h.status >= 200 && len(f.httpIPP) > 0 && f.httpIPP[0]
			h.doh = h.status >= 200 && len(f.httpDoH) > 0 && f.httpDoH[0]
			length, transfer = response.ContentLength, response.TransferEncoding
			h.websocket = h.status == 101 && httpHeaderHasToken(response.Header, "Upgrade", "websocket") && httpHeaderHasToken(response.Header, "Connection", "upgrade")
			if h.websocket {
				if len(f.httpUpgrades) == 0 || !f.httpUpgrades[0] {
					return 0, nil, sessionContext("WebSocket response lacks a matching upgrade request")
				}
				if len(f.httpWSKeys) == 0 {
					return 0, nil, sessionContext("WebSocket key missing")
				}
				sum := sha1.Sum([]byte(f.httpWSKeys[0] + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
				if response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
					return 0, nil, fmt.Errorf("WebSocket accept mismatch")
				}
				extension := response.Header.Get("Sec-WebSocket-Extensions")
				if err := validateWSDeflateOffer(f.httpWSExtensions[0], extension); err != nil {
					return 0, nil, err
				}
				h.ws, err = wsDeflate(extension, 1-dir)
				if err != nil {
					return 0, nil, err
				}
			}
			h.tunnel = h.status == 101 || (h.method == "CONNECT" && h.status >= 200 && h.status < 300)
			if h.method == "HEAD" || h.status < 200 || h.status == 204 || h.status == 304 || h.tunnel {
				length, transfer = 0, nil
			}
		} else {
			request, err := http.ReadRequest(r)
			if err != nil {
				return 0, nil, err
			}
			if request.Proto != "HTTP/1.0" && request.Proto != "HTTP/1.1" {
				return 0, nil, fmt.Errorf("unsupported HTTP version")
			}
			h.method = request.Method
			// Reuse the parsed header so ordinary deferred HTTP does not need
			// a second net/http parse or eager structured field decoding.
			h.ipp = ippMedia(request.Header.Get("Content-Type"))
			h.doh = dohPathEvidence(request.URL.RequestURI()) || dohMedia(request.Header.Get("Content-Type")) || dohMedia(request.Header.Get("Accept"))
			h.websocket = request.Method == "GET" && request.Header.Get("Sec-WebSocket-Version") == "13" && httpHeaderHasToken(request.Header, "Upgrade", "websocket") && httpHeaderHasToken(request.Header, "Connection", "upgrade")
			if h.websocket {
				h.wsKey = request.Header.Get("Sec-WebSocket-Key")
				key, err := base64.StdEncoding.DecodeString(h.wsKey)
				if err != nil || len(key) != 16 {
					h.websocket = false
				}
				h.wsOffer = request.Header.Get("Sec-WebSocket-Extensions")
			}
			length, transfer = request.ContentLength, request.TransferEncoding
			if len(f.httpMethods) >= 128 {
				return 0, nil, fmt.Errorf("%w: HTTP request pipeline exceeds 128 entries", errBinContext)
			}
		}
		h.chunked = len(transfer) == 1 && transfer[0] == "chunked"
		if len(transfer) > 0 && !h.chunked {
			return 0, nil, fmt.Errorf("unsupported HTTP transfer encoding")
		}
		h.closeDelimited = h.response && length < 0 && !h.chunked
		if length > int64(f.a.config.MaxMessageBytes-end) {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		if length >= 0 {
			h.total = end + int(length)
		}
		d.http = h
		// Expect: 100-continue can produce a response before the request body.
		// Associate the method as soon as its complete header is validated.
		if !h.response {
			h.requestID = f.a.ids.Add(1)
			f.httpIDs = append(f.httpIDs, h.requestID)
			f.httpTimes = append(f.httpTimes, d.ts)
			cost := int64(128*(len(f.httpMethods)+1) + len(h.wsKey) + len(h.wsOffer) + len(h.method))
			for _, s := range f.httpWSKeys {
				cost += int64(len(s))
			}
			for _, s := range f.httpWSExtensions {
				cost += int64(len(s))
			}
			if err := f.reserveSession(cost); err != nil {
				return 0, nil, err
			}
			f.httpMethods = append(f.httpMethods, h.method)
			f.httpUpgrades = append(f.httpUpgrades, h.websocket)
			f.httpWSKeys = append(f.httpWSKeys, h.wsKey)
			f.httpWSExtensions = append(f.httpWSExtensions, h.wsOffer)
			f.httpDoH = append(f.httpDoH, h.doh)
			// Allocate the sparse IPP marker queue only after an IPP request.
			if h.ipp || len(f.httpIPP) > 0 {
				for len(f.httpIPP) < len(f.httpMethods)-1 {
					f.httpIPP = append(f.httpIPP, false)
				}
				f.httpIPP = append(f.httpIPP, h.ipp)
			}
		}
	}
	if h.closeDelimited {
		return 0, nil, nil
	} // only a clean TCP FIN can end this body
	if !h.chunked {
		return h.total, f.spec("http", "HTTPExact"), nil
	}
	for {
		if h.trailers {
			if len(w) < h.cursor+2 {
				return 0, nil, nil
			}
			if bytes.Equal(w[h.cursor:h.cursor+2], []byte("\r\n")) {
				return h.cursor + 2, f.spec("http", "HTTPExact"), nil
			}
			end := incrementalHTTPIndex(w, h.cursor, &h.search, "\r\n\r\n")
			if end < 0 {
				if len(w)-h.cursor > 32<<10 {
					return 0, nil, fmt.Errorf("HTTP trailers exceed limit")
				}
				return 0, nil, nil
			}
			return end + 4, f.spec("http", "HTTPExact"), nil
		}
		if h.next > 0 {
			if len(w) < h.next+2 {
				return 0, nil, nil
			}
			if !bytes.Equal(w[h.next:h.next+2], []byte("\r\n")) {
				return 0, nil, fmt.Errorf("HTTP chunk has no trailing CRLF")
			}
			h.cursor, h.next = h.next+2, 0
		}
		end := incrementalHTTPIndex(w, h.cursor, &h.search, "\r\n")
		if end < 0 {
			if len(w)-h.cursor > 4096 {
				return 0, nil, fmt.Errorf("HTTP chunk header exceeds limit")
			}
			return 0, nil, nil
		}
		line := string(w[h.cursor:end])
		sizeText, _, _ := strings.Cut(line, ";")
		size, err := strconv.ParseUint(sizeText, 16, 32)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid HTTP chunk size")
		}
		h.cursor = end + 2
		if size == 0 {
			h.trailers = true
			continue
		}
		if size > uint64(f.a.config.MaxMessageBytes-h.cursor-2) {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		h.next = h.cursor + int(size)
	}
}

func (f *binFlow) finishHTTP(dir int) {
	h := f.directions[dir].http
	if h.response {
		if h.status >= 200 || h.status == 101 {
			f.httpMethods[0] = ""
			f.httpMethods = f.httpMethods[1:]
			f.httpIDs = f.httpIDs[1:]
			f.httpTimes = f.httpTimes[1:]
			if len(f.httpUpgrades) > 0 {
				f.httpUpgrades = f.httpUpgrades[1:]
				f.httpWSKeys = f.httpWSKeys[1:]
				f.httpWSExtensions = f.httpWSExtensions[1:]
			}
			if len(f.httpIPP) > 0 {
				f.httpIPP = f.httpIPP[1:]
			}
			if len(f.httpDoH) > 0 {
				f.httpDoH = f.httpDoH[1:]
			}
		}
	}
	f.directions[dir].http = nil
	f.directions[dir].headerScan = 0
	if h.tunnel {
		f.httpIDs, f.httpTimes = nil, nil
		f.httpWSKeys, f.httpWSExtensions = nil, nil
	}
	if h.tunnel && h.status == 101 && h.websocket {
		client := 1 - dir
		f.protocol, f.ws = "websocket", h.ws
		if f.ws == nil {
			f.ws = &binWebSocket{client: client, phase: "frame"}
		}
		f.httpUpgrades = nil
		f.httpDoH, f.httpIPP = nil, nil
		f.binding, f.level, f.httpMethods = nil, 0, nil
		return
	}
	if h.tunnel {
		// A confirmed CONNECT/101 response is an explicit protocol boundary.
		// Re-probe the next ordered bytes once; keep the capture flow identity.
		f.protocol, f.binding, f.level = "", nil, 0
		f.httpMethods = nil
		f.httpUpgrades = nil
		f.httpDoH, f.httpIPP = nil, nil
	}
}

// Search only new bytes plus the separator overlap. Coordinates are relative to
// the current message, so buffer reallocation preserves them; finish/release
// resets the direction before a pipelined message or protocol re-probe.
func incrementalHTTPIndex(w []byte, start int, cursor *int, separator string) int {
	at := *cursor
	if at < start || at > len(w) {
		at = start
	}
	if end := bytes.Index(w[at:], []byte(separator)); end >= 0 {
		return at + end
	}
	*cursor = len(w) - len(separator) + 1
	if *cursor < start {
		*cursor = start
	}
	return -1
}

func httpHeaderHasToken(header http.Header, name, token string) bool {
	for _, value := range header.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func (f *binFlow) httpEvidence(dir int, e *ProtocolEvent) {
	h := f.directions[dir].http
	if h == nil {
		return
	}
	e.Completeness = "message"
	if !h.response {
		e.ID = h.requestID
		e.TransactionID = e.ID
	} else if len(f.httpIDs) > 0 {
		e.TransactionID = f.httpIDs[0]
		e.ResponseTo = f.httpIDs[0]
		e.Session = map[string]any{"Response Latency": e.Timestamp.Sub(f.httpTimes[0]) / time.Microsecond}
		if h.status >= 200 {
			e.Completeness = "transaction"
		}
	}
}
