package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"yaklang/common/utils/bruteutils/grdp/protocol/rfb"

	"yaklang/common/utils/bruteutils/grdp/core"
	"yaklang/common/utils/bruteutils/grdp/glog"
	"yaklang/common/utils/bruteutils/grdp/protocol/nla"
	"yaklang/common/utils/bruteutils/grdp/protocol/pdu"
	"yaklang/common/utils/bruteutils/grdp/protocol/sec"
	"yaklang/common/utils/bruteutils/grdp/protocol/t125"
	"yaklang/common/utils/bruteutils/grdp/protocol/tpkt"
	"yaklang/common/utils/bruteutils/grdp/protocol/x224"
)

type Client struct {
	Host string // ip:port
	tpkt *tpkt.TPKT
	x224 *x224.X224
	mcs  *t125.MCSClient
	sec  *sec.Client
	pdu  *pdu.Client
	vnc  *rfb.RFB
}

func NewClient(host string, logLevel glog.LEVEL) *Client {
	glog.SetLevel(logLevel)
	logger := log.New(os.Stdout, "", 0)
	glog.SetLogger(logger)
	return &Client{
		Host: host,
	}
}

func (g *Client) Login(domain, user, pwd string) error {
	conn, err := net.DialTimeout("tcp", g.Host, 3*time.Second)
	if err != nil {
		return fmt.Errorf("[dial err] %v", err)
	}
	defer conn.Close()
	glog.Info(conn.LocalAddr().String())
	//domain := strings.Split(g.Host, ":")[0]

	g.tpkt = tpkt.New(core.NewSocketLayer(conn), nla.NewNTLMv2(domain, user, pwd))
	g.x224 = x224.New(g.tpkt)
	g.mcs = t125.NewMCSClient(g.x224)
	g.sec = sec.NewClient(g.mcs)
	g.pdu = pdu.NewClient(g.sec)

	g.sec.SetUser(user)
	g.sec.SetPwd(pwd)
	g.sec.SetDomain(domain)
	//g.sec.SetClientAutoReconnect()

	g.tpkt.SetFastPathListener(g.sec)
	g.sec.SetFastPathListener(g.pdu)
	g.pdu.SetFastPathSender(g.tpkt)

	//g.x224.SetRequestedProtocol(x224.PROTOCOL_SSL)
	//g.x224.SetRequestedProtocol(x224.PROTOCOL_RDP)

	err = g.x224.Connect()
	if err != nil {
		return fmt.Errorf("[x224 connect err] %v", err)
	}
	glog.Info("wait connect ok")
	wg := &sync.WaitGroup{}
	wg.Add(1)

	g.pdu.On("error", func(e error) {
		err = e
		glog.Error("error", e)
		wg.Done()
	}).On("close", func() {
		err = errors.New("close")
		glog.Info("on close")
		//wg.Done()
	}).On("success", func() {
		err = nil
		glog.Info("on success")
		//wg.Done()
	}).On("ready", func() {
		glog.Info("on ready")
	}).On("update", func(rectangles []pdu.BitmapData) {
		glog.Info("on update:", rectangles)
	})

	wg.Wait()
	return err
}

func (g *Client) LoginVNC() error {
	conn, err := net.DialTimeout("tcp", g.Host, 3*time.Second)
	if err != nil {
		return fmt.Errorf("[dial err] %v", err)
	}
	defer conn.Close()
	glog.Info(conn.LocalAddr().String())
	//domain := strings.Split(g.Host, ":")[0]

	g.vnc = rfb.NewRFB(rfb.NewRFBConn(conn))
	wg := &sync.WaitGroup{}
	wg.Add(1)

	g.vnc.On("error", func(e error) {
		glog.Info("on error")
		err = e
		glog.Error(e)
		wg.Done()
	}).On("close", func() {
		err = errors.New("close")
		glog.Info("on close")
		//wg.Done()
	}).On("success", func() {
		err = nil
		glog.Info("on success")
		//wg.Done()
	}).On("ready", func() {
		glog.Info("on ready")
	}).On("update", func(b *rfb.BitRect) {
		glog.Info("on update:", b)
	})
	glog.Info("on Wait")
	wg.Wait()
	return err
}

func main() {
	g := NewClient("192.168.18.107:3389", glog.DEBUG)
	err := g.Login("", "wren", "wren")
	//g := NewClient("192.168.18.100:5902", glog.DEBUG)
	//err := g.LoginVNC()
	if err != nil {
		fmt.Println("Login:", err)
	}

}
