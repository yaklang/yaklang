package facades

import (
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestNewDNSServer(t *testing.T) {
	t.SkipNow()

	lis, err := net.Listen("tcp", "127.0.0.1:4443")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				t.Error(err)
				t.FailNow()
				return
			}

			isTls := utils.NewBool(false)
		WRAPPER:
			peekableConn := utils.NewPeekableNetConn(conn)
			raw, err := peekableConn.Peek(1)
			if err != nil {
				utils.Errorf("")
				return
			}
			switch raw[0] {
			case 0x16: // https
				tlsConn := tlsutils.NewDefaultTLSServer(peekableConn)
				log.Error("https conn is recv... start to handshake")
				err := tlsConn.Handshake()
				if err != nil {
					conn.Close()
					log.Errorf("handle shake failed: %s", err)
					return
				}
				log.Infof("handshake finished for %v", conn.RemoteAddr())
				conn = tlsConn
				isTls.Set()
				goto WRAPPER
			case 'J': // 4a524d49 (JRMI)
				jrmiMagic, _ := peekableConn.Peek(4)
				if bytes.Equal(jrmiMagic, []byte("JRMI")) {
					log.Info("handle for JRMI")
					//err := rmiShakeHands(peekableConn)
					//if err != nil {
					//	log.Errorf("rmi handshak failed: %s", err)
					//}
					peekableConn.Close()
					return
				}
			}

			log.Infof("start to fallback http handlers for: %s", conn.RemoteAddr())
			//err = getHTTPHandler(isTls.IsSet())(peekableConn)
			//if err != nil {
			//	log.Errorf("handle http failed: %s", err)
			//	return
			//}
		}
	}()

	time.Sleep(1 * time.Second)
	rsp, err := utils.NewDefaultHTTPClient().Get("https://127.0.0.1:4443")
	if err != nil {
		t.Error(err)
		return
	}

	utils.HttpShow(rsp)
}

func TestLookupAll(t *testing.T) {

	type callbackInfo struct {
		dnsType    string
		domain     string
		ip         string
		fromServer string
		method     string
	}

	type args struct {
		host string
		opt  []netx.DNSOption
	}
	fakeDnsServer := MockDNSServerDefault("abc.com", func(record string, domain string) string {
		return "9.9.9.9"
	})

	tests := []struct {
		name       string
		args       args
		wantMethod string
		want       []string
		callback   callbackInfo // 添加字段用于存储回调信息
	}{
		{
			name: "set hosts 不会缓存 10.10.10.10(case A)",
			args: args{
				host: "abc.com",
				opt: []netx.DNSOption{
					netx.WithTemporaryHosts(map[string]string{"abc.com": "10.10.10.10"}),
					netx.WithDNSServers(fakeDnsServer),
					netx.WithDNSDisableSystemResolver(true),
				},
			},
			wantMethod: "hosts",
			want:       []string{"10.10.10.10"},
		},
		{
			name: "cancel hosts 没有缓存所以结果是 fake 的ip，本次会进行缓存(case A)",
			args: args{
				host: "abc.com",
				opt: []netx.DNSOption{
					netx.WithDNSServers(fakeDnsServer),
					netx.WithDNSDisableSystemResolver(true),
				},
			},
			wantMethod: "yakdns.udp",
			want:       []string{"9.9.9.9"},
		},

		{
			name: "test hosts cache (case A)",
			args: args{
				host: "abc.com",
				opt: []netx.DNSOption{
					netx.WithDNSServers(fakeDnsServer),
					netx.WithDNSDisableSystemResolver(true),
				},
			},
			wantMethod: "cache",
			want:       []string{"9.9.9.9"},
		},
		{
			name: "设置 hosts 与 host 不相同会缓存 9.9.9.9 (case A)",
			args: args{
				host: "abc.com",
				opt: []netx.DNSOption{
					netx.WithTemporaryHosts(map[string]string{"bcd.com": "10.10.10.10"}),
					netx.WithDNSServers(fakeDnsServer),
					netx.WithDNSDisableSystemResolver(true),
				},
			},
			wantMethod: "cache",
			want:       []string{"9.9.9.9"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建当前测试用例的副本
			currentTest := tt
			dnsOptions := append(currentTest.args.opt, netx.WithDNSCallback(func(dnsType, domain, ip, fromServer, method string) {
				currentTest.callback = callbackInfo{
					dnsType:    dnsType,
					domain:     domain,
					ip:         ip,
					fromServer: fromServer,
					method:     method,
				}
			}))

			if got := netx.LookupAll(currentTest.args.host, dnsOptions...); !reflect.DeepEqual(got, currentTest.want) {
				t.Errorf("LookupAll() = %v, want %v", got, currentTest.want)
			}
			// 验证回调信息
			// 这里可以添加更多的验证，比如验证 dnsType, domain 等
			if currentTest.callback.method != currentTest.wantMethod {
				t.Errorf("LookupAll() callback method = %v, want %v", currentTest.callback.method, currentTest.wantMethod)
			}

		})
	}
}
