package cybertunnel

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/gmsm/x509"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

type HTTPTrigger struct {
	defaultHTTPPort, defaultHTTPSPort int
	httpListener                      net.Listener
	tlsListener                       net.Listener

	dnslogDomain []string
	externalIP   string

	responseFetcherCache *utils.Cache[func([]byte) []byte]
	notificationCache    *utils.Cache[[]*tpb.HTTPRequestTriggerNotification]
}

var defaultHTTPTrigger *HTTPTrigger

func NewHTTPTrigger(external string, dnslogDomain ...string) (*HTTPTrigger, error) {
	trigger := &HTTPTrigger{
		dnslogDomain:         dnslogDomain,
		externalIP:           external,
		responseFetcherCache: utils.NewTTLCache[func([]byte) []byte](time.Minute * 4),
		notificationCache:    utils.NewTTLCache[[]*tpb.HTTPRequestTriggerNotification](time.Minute * 4),
	}
	return trigger, nil
}

func (t *HTTPTrigger) serveRequest(isTls bool, req []byte, conn net.Conn) error {
	reqUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(req, isTls)
	if err != nil {
		return err
	}
	uStr := reqUrl.String()
	host := utils.ExtractHost(uStr)

	method := lowhttp.GetHTTPRequestMethod(req)
	reqBodyLen := len(lowhttp.GetHTTPPacketBody(req))

	logMsg := bytes.NewBufferString(fmt.Sprintf("[%v] %v body: %v", method, uStr, utils.ByteSize(uint64(reqBodyLen))))
	var token = strings.ToLower(host)
	if len(t.dnslogDomain) > 0 {
		for _, expectedDomain := range t.dnslogDomain {
			domainLower := strings.ToLower(expectedDomain)
			before, _, ok := strings.Cut(token, domainLower)
			if !ok {
				log.Info(logMsg)
				continue
			}
			before = strings.TrimSpace(before)
			before = strings.Trim(before, ".")
			token = before
			idx := strings.LastIndex(before, ".")
			if idx > 0 {
				token = before[idx+1:]
			}
			break
		}
	}
	log.Infof("found token: %v from: %v", token, uStr)
	fetcher, haveToken := t.responseFetcherCache.Get(token)
	if !haveToken {
		log.Infof("no token found: log" + logMsg.String())
		return nil
	}
	rsp := fetcher(req)
	if rsp == nil {
		log.Info(logMsg.String())
		return nil
	}
	rspStatus := lowhttp.ExtractStatusCodeFromResponse(rsp)
	rspBody := lowhttp.GetHTTPPacketBody(rsp)
	log.Infof("status: %3d len: %6d [%v] %v", rspStatus, len(rspBody), method, uStr)

	var ns []*tpb.HTTPRequestTriggerNotification
	notifications, ok := t.notificationCache.Get(token)
	if ok {
		ns = notifications
	}
	ns = append(ns, &tpb.HTTPRequestTriggerNotification{
		Url:              uStr,
		IsHttps:          isTls,
		RemoteAddr:       conn.RemoteAddr().String(),
		TriggerTimestamp: time.Now().Unix(),
		Request:          req,
		Response:         rsp,
	})
	t.notificationCache.Set(token, ns)
	conn.Write(rsp)
	conn.Close()
	return nil
}

func (t *HTTPTrigger) QueryResults(token string) ([]*tpb.HTTPRequestTriggerNotification, bool) {
	if defaultHTTPTrigger == nil {
		return nil, false
	}
	if defaultHTTPTrigger.notificationCache == nil {
		return nil, false
	}
	return defaultHTTPTrigger.notificationCache.Get(strings.ToLower(token))
}

func (t *HTTPTrigger) Register(token string, response func([]byte) []byte) ([]string, error) {
	token = strings.ToLower(token)
	if t == nil {
		return nil, utils.Error("nil HTTPTrigger")
	}

	t.responseFetcherCache.Set(token, response)
	var results []string
	if len(t.dnslogDomain) > 0 {
		domain := fmt.Sprintf("%v.%v", token, t.dnslogDomain[rand.Intn(len(t.dnslogDomain))])
		results = append(results, domain)
		results = append(results, "https://"+domain)
		results = append(results, "http://"+domain)
	} else if t.externalIP == "" {
		results = append(results, t.externalIP)
		results = append(results, "https://"+t.externalIP)
		results = append(results, "http://"+t.externalIP)
	}

	if len(results) > 0 {
		return results, nil
	}
	return nil, utils.Errorf("register %v failed, plz checking your domain or external ip", token)
}

func (t *HTTPTrigger) SetHTTPPort(i int) {
	t.defaultHTTPPort = i
}

func (t *HTTPTrigger) SetHTTPSPort(i int) {
	t.defaultHTTPSPort = i
}

func (t *HTTPTrigger) Serve() error {

	var httpErr, tlsErr error

	log.Info("start to listen in :80/:443")

	var defaultHTTPPort = t.defaultHTTPPort
	if defaultHTTPPort <= 0 {
		defaultHTTPPort = 80
	}
	var defaultHTTPSPort = t.defaultHTTPSPort
	if defaultHTTPSPort <= 0 {
		defaultHTTPSPort = 443
	}

	t.httpListener, httpErr = net.Listen("tcp", "0.0.0.0:"+fmt.Sprint(defaultHTTPPort))
	t.tlsListener, tlsErr = net.Listen("tcp", "0.0.0.0:"+fmt.Sprint(defaultHTTPSPort))
	defer func() {
		if t.httpListener != nil {
			t.httpListener.Close()
		}
		if t.tlsListener != nil {
			t.tlsListener.Close()
		}
	}()
	errMsg := ""
	if httpErr != nil {
		errMsg = utils.Wrap(httpErr, "create http listener failed\n").Error()
	}
	if tlsErr != nil {
		errMsg = utils.Wrap(tlsErr, "create tls listener failed\n").Error()
	}

	if errMsg != "" {
		return utils.Error(errMsg)
	}

	caPem, caPrivateKey, err := tlsutils.GenerateSelfSignedCertKeyWithCommonName("Yak Bridge Service", "yaklang.io", nil, nil)
	if err != nil {
		return err
	}

	cert, err := x509.ParseCertificate(caPem)
	if err != nil {
		return utils.Errorf("extract x509 cert failed: %s", err)
	}
	mitmConfig, err := mitm.NewConfig(cert, caPrivateKey)
	if err != nil {
		return utils.Errorf("create mitm config failed: %s", err)
	}

	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()

		for {
			conn, err := t.httpListener.Accept()
			if err != nil {
				log.Errorf("accept http connection failed: %s", err)
				return
			}
			log.Infof("accept http connection: %s", conn.RemoteAddr().String())

			go func() {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("panic: %s", err)
					}
				}()

				defer conn.Close()
				cr := bufio.NewReader(conn)
				req, err := utils.ReadHTTPRequestFromBufioReader(cr)
				if err != nil {
					log.Errorf("read http request failed: %s", err)
					return
				}
				reqRaw := httpctx.GetRequestBytes(req)
				err = t.serveRequest(false, reqRaw, conn)
				if err != nil {
					log.Warnf("serve http request failed: %s", err)
				}
			}()
		}
	}()

	go func() {
		defer wg.Done()
		mitmConfig.SkipTLSVerify(true)

		for {
			conn, err := t.tlsListener.Accept()
			if err != nil {
				log.Errorf("accept tls connection failed: %s", err)
				return
			}
			log.Infof("accept http(tls) connection: %s", conn.RemoteAddr().String())

			go func() {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("panic: %s", err)
					}
				}()

				conn = gmtls.Server(conn, mitmConfig.TLS())

				defer conn.Close()
				cr := bufio.NewReader(conn)
				req, err := utils.ReadHTTPRequestFromBufioReader(cr)
				if err != nil {
					log.Warnf("read http request failed: %s", err)
					return
				}
				reqRaw := httpctx.GetRequestBytes(req)
				err = t.serveRequest(true, reqRaw, conn)
				if err != nil {
					log.Warnf("serve http request failed: %s", err)
				}
			}()
		}
	}()

	wg.Wait()
	return nil
}
