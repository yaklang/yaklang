package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"math/rand"
	"net"
	"os"
	"strings"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/ReneKroon/ttlcache"

	"sync"
	"time"
)

var (
	remoteAddrConvertor = ttlcache.NewCache()

	// 全局反连配置
	globalReverseServerStarted = utils.NewBool(false)
	localReverseHost           string
	remoteReverseIP            string
	remoteReversePort          int
	remoteAddr                 string
	remoteSecret               string
	//所有已创建的FacadeServer
	facadeServers    map[string]*facades.FacadeServer
	facadeServersMux *sync.Mutex
)

func init() {
	remoteAddrConvertor.SetTTL(30 * time.Second)
	facadeServers = make(map[string]*facades.FacadeServer)
	facadeServersMux = &sync.Mutex{}
}

func (s *Server) GetGlobalReverseServer(ctx context.Context, req *ypb.Empty) (*ypb.GetGlobalReverseServerResponse, error) {
	return &ypb.GetGlobalReverseServerResponse{
		PublicReverseIP:   remoteReverseIP,
		PublicReversePort: int32(remoteReversePort),
		LocalReverseAddr:  localReverseHost,
		LocalReversePort:  int32(s.reverseServer.Port),
	}, nil
}

func (s *Server) AvailableLocalAddr(ctx context.Context, empty *ypb.Empty) (*ypb.AvailableLocalAddrResponse, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var data []*ypb.NetInterface
	for _, iface := range ifs {
		addrs, err := iface.Addrs()
		if err != nil {
			log.Errorf("fetch iface addr failed: %s", err)
			continue
		}

		var ipAddr string
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			if utils.IsIPv4(ip.String()) {
				ipAddr = ip.String()
				break
			}
		}

		if ipAddr == "" {
			continue
		}

		data = append(data, &ypb.NetInterface{
			Name: iface.Name,
			IP:   ipAddr,
			Addr: ipAddr,
		})
	}

	return &ypb.AvailableLocalAddrResponse{Interfaces: data}, nil
}

func (s *Server) ConfigGlobalReverse(req *ypb.ConfigGlobalReverseParams, stream ypb.Yak_ConfigGlobalReverseServer) error {
	localReverseHost = req.GetLocalAddr()
	if localReverseHost == "" {
		localReverseHost = "127.0.0.1"
	}
	os.Setenv(consts.YAK_BRIDGE_LOCAL_REVERSE_ADDR, utils.HostPort(localReverseHost, s.reverseServer.Port))

	if globalReverseServerStarted.IsSet() {
		return nil
	}

	globalReverseServerStarted.Set()
	defer globalReverseServerStarted.UnSet()

	remoteIP, err := cybertunnel.GetTunnelServerExternalIP(req.GetConnectParams().GetAddr(), req.GetConnectParams().GetSecret())
	if err != nil {
		return err
	}
	remoteReverseIP = remoteIP.String()
	s.reverseServer.ExternalHost = remoteReverseIP
	defer func() {
		remoteReverseIP = ""
		s.reverseServer.ExternalHost = ""
	}()

	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()

		for {
			err := stream.Send(&ypb.Empty{})
			if err != nil {
				return
			}
			select {
			case <-stream.Context().Done():
				return
			default:
				time.Sleep(1 * time.Second)
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer func() {
			remoteReversePort = 0
			remoteAddr = ""
			remoteSecret = ""
			os.Setenv(consts.YAK_BRIDGE_REMOTE_REVERSE_ADDR, "")
			os.Setenv(consts.YAK_BRIDGE_ADDR, "")
			os.Setenv(consts.YAK_BRIDGE_SECRET, "")
		}()

		remoteReversePort = s.reverseServer.Port
		remoteAddr = req.GetConnectParams().GetAddr()
		remoteSecret = req.GetConnectParams().GetSecret()
		os.Setenv(consts.YAK_BRIDGE_REMOTE_REVERSE_ADDR, utils.HostPort(remoteReverseIP, remoteReversePort))
		os.Setenv(consts.YAK_BRIDGE_ADDR, remoteAddr)
		os.Setenv(consts.YAK_BRIDGE_SECRET, remoteSecret)

		for {
			err := cybertunnel.MirrorLocalPortToRemote(
				"tcp", s.reverseServer.Port, remoteReversePort,
				fmt.Sprintf("yakit-global-%v", uuid.NewV4().String()),
				req.GetConnectParams().GetAddr(), req.GetConnectParams().GetSecret(),
				stream.Context(), func(remoteAddr string, localAddr string) {
					remoteAddrConvertor.SetWithTTL(localAddr, remoteAddr, 30*time.Second)
				},
			)
			if err != nil {
				log.Error(err)
			}
			select {
			case <-stream.Context().Done():
				return
			default:
				time.Sleep(1 * time.Second)
				remoteReversePort = 63535 + rand.Intn(1400)
			}
		}
	}()
	wg.Wait()
	return nil
}

func (s *Server) StartFacades(req *ypb.StartFacadesParams, stream ypb.Yak_StartFacadesServer) error {
	ctx := stream.Context()
	defer log.Info("exit facades local server all...")

	wg := new(sync.WaitGroup)
	defer wg.Wait()

	var err error
	_ = err

	// 判断一下本地是有服务器应该启动
	if !req.EnableDNSLogServer && req.GetLocalFacadePort() <= 0 {
		_ = stream.Send(yaklib.NewYakitLogExecResult("warning", "no reverse server enabled"))
		return utils.Errorf("no dns/rmi/http(s)(facades) enabled...")
	}

	if req.ConnectParam != nil {
		if _, err := s.GetTunnelServerExternalIP(ctx, req.ConnectParam); err != nil {
			_ = stream.Send(yaklib.NewYakitLogExecResult("warning", "yak bridge params failed: %s", err))
			return utils.Errorf("connect bridge server failed: %s", err)
		}
	}

	// 验证远程 Bridge 是否生效
	if req.GetVerify() {
		// 验证域名是否可用
		log.Infof("start to verify external domain: %v", req.GetExternalDomain())
		r, err := s.VerifyTunnelServerDomain(ctx, &ypb.VerifyTunnelServerDomainParams{
			ConnectParams: req.GetConnectParam(),
			Domain:        req.GetExternalDomain(),
		})
		if err != nil {
			log.Error(err)
			return utils.Errorf("verify tunnel server domain failed: %s", err)
		}
		if !r.Ok {
			log.Error(r.Reason)
			return utils.Errorf("failed to verify tunnel server with domain: %s, reason: \n%s", r.Domain, r.Reason)
		}
	}

	//
	if req.EnableDNSLogServer {
		// 启动 dns log 服务器
		if req.GetDNSLogRemotePort() > 0 {

			// 连接服务端，启动端口转发
			wg.Add(1)
			go func() {
				defer wg.Done()

				defer log.Info("dnslog remote mirror exit")

				for {
					err := cybertunnel.MirrorLocalPortToRemote(
						"udp", int(req.GetDNSLogLocalPort()), int(req.GetDNSLogRemotePort()),
						"dns", req.GetConnectParam().GetAddr(), req.GetConnectParam().GetSecret(),
						stream.Context(),
						func(remoteAddr string, localAddr string) {
							remoteAddrConvertor.SetWithTTL(localAddr, remoteAddr, 30*time.Second)
						},
					)
					if err != nil {
						log.Errorf("mirror dns/udp failed: %s", err)
					}

					select {
					case <-stream.Context().Done():
						return
					default:
						log.Infof("retry... mirror dns/udp")
						_ = stream.Send(yaklib.NewYakitLogExecResult("warning", "mirror dns(udp) failed: %s", err))
						time.Sleep(1 * time.Second)
					}
				}
			}()

		}

		// 启动 DNS 服务器
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			defer log.Info("dnslog server exit")

			rsp, err := s.GetTunnelServerExternalIP(ctx, req.GetConnectParam())
			if err != nil {
				log.Errorf("dnslog need tunnel server external ip, but failed: %s", err)
				return
			}
			dns, err := facades.NewDNSServer(req.GetExternalDomain(), rsp.GetIP(), "0.0.0.0", int(req.GetDNSLogLocalPort()))
			if err != nil {
				log.Errorf("create dns server failed: %s", err)
				return
			}
			err = dns.Serve(ctx)
			if err != nil {
				log.Errorf("serve/start dns server failed: %s", err)
				return
			}
		}()
	}

	if req.LocalFacadePort > 0 {

		if req.GetFacadeRemotePort() > 0 {
			// 连接服务端，启动端口转发
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer log.Info("facade remote mirror exit")

				for {
					err := cybertunnel.MirrorLocalPortToRemote(
						"tcp", int(req.GetLocalFacadePort()), int(req.GetFacadeRemotePort()),
						"facades", req.GetConnectParam().GetAddr(), req.GetConnectParam().GetSecret(),
						stream.Context(),
						func(remoteAddr string, localAddr string) {
							remoteAddrConvertor.SetWithTTL(localAddr, remoteAddr, 30*time.Second)
						},
					)
					if err != nil {
						log.Errorf("mirror tcp/facades:rmi/http(s) failed: %s", err)
					}

					select {
					case <-stream.Context().Done():
						return
					default:
						log.Infof("retry... mirror facades...")
						_ = stream.Send(yaklib.NewYakitLogExecResult("warning", "mirror rmi/http(tls)(tcp) failed: %s", err))
						time.Sleep(1 * time.Second)
					}
				}
			}()
		}

		host, port := req.GetLocalFacadeHost(), req.GetLocalFacadePort()
		server := facades.NewFacadeServer(host, int(port))
		server.OnHandle(func(n *facades.Notification) {
			res, ok := remoteAddrConvertor.Get(n.RemoteAddr)
			if ok {
				n.RemoteAddr = fmt.Sprint(res)
			}
			raw, err := json.Marshal(n)
			if err != nil {
				log.Errorf("marshal error: %s", err)
				return
			}
			_, _ = yakit.NewRisk(
				n.RemoteAddr,
				yakit.WithRiskParam_Title(fmt.Sprintf("reverse [%v] connection from %s", n.Type, n.RemoteAddr)),
				yakit.WithRiskParam_TitleVerbose(fmt.Sprintf(`接收到来自 [%v] 的反连[%v]`, n.RemoteAddr, n.Type)),
				yakit.WithRiskParam_RiskType(fmt.Sprintf(`reverse-%v`, n.Type)),
				yakit.WithRiskParam_Details(n),
			)
			err = stream.Send(yaklib.NewYakitLogExecResult("facades-msg", string(raw)))
			if err != nil {
				log.Errorf("feedback to client failed: %s", err)
				return
			}
		})
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer log.Info("facade local server exit")
			err := server.ServeWithContext(stream.Context())
			if err != nil {
				log.Errorf("serve facades server(rmi/http(s)) failed: %s", err)
			}
		}()
	}

	return nil
}

func (s *Server) StartFacadesWithYsoObject(req *ypb.StartFacadesWithYsoParams, stream ypb.Yak_StartFacadesWithYsoObjectServer) error {
	ctx := stream.Context()
	//校验必要参数
	reversePort := req.GetReversePort()
	if reversePort <= 0 {
		return utils.Errorf("reversePort is not valid")
	}
	optionsReqMsg := req.GetGenerateClassParams()
	if optionsReqMsg == nil {
		return utils.Error("not set class params")
	}

	////生成Class
	//bytesRsp, err := s.GenerateYsoBytes(ctx, optionsReqMsg)
	//if err != nil {
	//	return utils.Errorf("generate class error: %v", err)
	//}

	//如果启用公网反连，公网服务器监听指定端口，本地监听随机端口，否则本地监听指定端口
	wg := new(sync.WaitGroup)
	var globalError error
	var listenPort int
	if req.GetIsRemote() {
		listenPort = utils.GetRandomAvailableTCPPort()
		//启动端口流量转发
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer log.Info("facade remote mirror exit")

			err := cybertunnel.MirrorLocalPortToRemote(
				"tcp", listenPort, int(reversePort),
				"facades_"+req.GetToken(), req.GetBridgeParam().GetAddr(), req.GetBridgeParam().GetSecret(),
				ctx,
				func(remoteAddr string, localAddr string) {
					remoteAddrConvertor.SetWithTTL(localAddr, remoteAddr, 30*time.Second)
				},
			)
			if err != nil {
				log.Errorf("mirror tcp/facades:rmi/http(s) failed: %v", err)
			}

			select {
			case <-ctx.Done():
				return
			default:
				err := utils.Errorf("mirror rmi/http(tls)(tcp) failed: %v", err)
				log.Info(err)
				stream.Send(yaklib.NewYakitLogExecResult("mirror_error", err.Error()))
			}

		}()
	} else {
		listenPort = int(reversePort)
	}

	//启动Facade Server
	reverseHost := req.GetReverseHost()
	//var className, classPath string
	//if strings.HasSuffix(bytesRsp.GetFileName(), ".class") {
	//	classPath = bytesRsp.GetFileName()
	//	className = bytesRsp.GetFileName()[:len(bytesRsp.GetFileName())-6]
	//} else {
	//	return utils.Error("facade server need class")
	//}
	httpAddr := fmt.Sprintf("http://%s:%d/", reverseHost, reversePort)
	server := facades.NewFacadeServer("0.0.0.0", listenPort,
		facades.SetReverseAddress(httpAddr),
	)
	facadeServersMux.Lock()
	facadeServers[req.GetToken()] = server
	facadeServersMux.Unlock()
	server.OnHandle(func(n *facades.Notification) {
		res, ok := remoteAddrConvertor.Get(n.RemoteAddr)
		if ok {
			n.RemoteAddr = fmt.Sprint(res)
		}
		var data interface{}
		if n.Type == "http" || n.Type == "https" {
			data = &struct {
				*facades.Notification
				Raw string `json:"raw"`
			}{
				Notification: n,
				Raw:          string(n.Raw),
			}
		} else {
			data = n
		}
		raw, err := json.Marshal(data)
		if err != nil {
			log.Errorf("marshal error: %s", err)
			return
		}
		_, _ = yakit.NewRisk(
			n.RemoteAddr,
			yakit.WithRiskParam_Title(fmt.Sprintf("reverse [%v] connection from %s", n.Type, n.RemoteAddr)),
			yakit.WithRiskParam_TitleVerbose(fmt.Sprintf(`接收到来自 [%v] 的反连[%v]`, n.RemoteAddr, n.Type)),
			yakit.WithRiskParam_RiskType(fmt.Sprintf(`reverse-%v`, n.Type)),
			yakit.WithRiskParam_Details(n),
		)
		err = stream.Send(yaklib.NewYakitLogExecResult("facades-msg", string(raw)))
		if err != nil {
			log.Errorf("feedback to client failed: %s", err)
			return
		}
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := server.ServeWithContext(stream.Context())
		if err != nil {
			//客户端关闭
			if strings.Contains(err.Error(), "use of closed network connection") {
				log.Info("connection closed")
			} else {
				//启动失败
				err := utils.Errorf("start facade server error: %v", err)
				stream.Send(yaklib.NewYakitLogExecResult("error", err.Error()))
			}
		}
	}()
	wg.Wait()
	delete(facadeServers, req.GetToken())
	return globalError
}

func (s *Server) ApplyClassToFacades(ctx context.Context, req *ypb.ApplyClassToFacadesParamsWithVerbose) (*ypb.Empty, error) {
	token := req.GetToken()
	server, ok := facadeServers[token]
	if !ok {
		return nil, utils.Errorf("Server is not exist for token: %s", token)
	}
	bytesRsp, err := s.GenerateYsoBytes(ctx, req.GetGenerateClassParams())
	if err != nil {
		return nil, utils.Errorf("generate class error: %v", err)
	}
	var className, classPath string
	if strings.HasSuffix(bytesRsp.GetFileName(), ".class") {
		classPath = bytesRsp.GetFileName()
		className = bytesRsp.GetFileName()[:len(bytesRsp.GetFileName())-6]
	} else {
		return nil, utils.Error("facade server need class")
	}
	httpAddr := server.ReverseAddr
	server.Config(
		facades.SetHttpResource(classPath, bytesRsp.GetBytes()),
		facades.SetLdapResourceAddr(className, httpAddr),
		facades.SetRmiResourceAddr(className, httpAddr),
	)
	return &ypb.Empty{}, nil
}
