// main.go
package main

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/pdu"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/sec"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/t125"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/tpkt"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/x224"
)

const (
	PROTOCOL_RDP       = x224.PROTOCOL_RDP
	PROTOCOL_SSL       = x224.PROTOCOL_SSL
	PROTOCOL_HYBRID    = x224.PROTOCOL_HYBRID
	PROTOCOL_HYBRID_EX = x224.PROTOCOL_HYBRID_EX
)

type RdpClient struct {
	Host   string // ip:port
	Width  int
	Height int
	info   *Info
	tpkt   *tpkt.TPKT
	x224   *x224.X224
	mcs    *t125.MCSClient
	sec    *sec.Client
	pdu    *pdu.Client
}

func NewRdpClient(host string, width, height int, logLevel glog.LEVEL) *RdpClient {
	return &RdpClient{
		Host:   host,
		Width:  width,
		Height: height,
	}
}
func (g *RdpClient) SetRequestedProtocol(p uint32) {
	g.x224.SetRequestedProtocol(p)
}

func BitmapDecompress(bitmap *pdu.BitmapData) []byte {
	return core.Decompress(bitmap.BitmapDataStream, int(bitmap.Width), int(bitmap.Height), Bpp(bitmap.BitsPerPixel))
}

func uiRdp(info *Info) (error, *RdpClient) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	BitmapCH = make(chan []Bitmap, 500)
	g := NewRdpClient(fmt.Sprintf("%s:%s", info.Ip, info.Port), info.Width, info.Height, glog.INFO)
	g.info = info
	err := g.Login()
	if err != nil {
		fmt.Println("Login:", err)
		return err, nil
	}

	g.pdu.On("error", func(e error) {
		fmt.Println("on error:", e)
	}).On("close", func() {
		err = errors.New("close")
		fmt.Println("on close")
	}).On("success", func() {
		fmt.Println("on success")
	}).On("ready", func() {
		fmt.Println("on ready")
	}).On("update", func(rectangles []pdu.BitmapData) {
		glog.Info(time.Now(), "on update Bitmap:", len(rectangles))
		bs := make([]Bitmap, 0, 50)
		for _, v := range rectangles {
			IsCompress := v.IsCompress()
			data := v.BitmapDataStream
			//glog.Info("data:", data)
			if IsCompress {
				data = BitmapDecompress(&v)
				IsCompress = false
			}

			//glog.Info(IsCompress, v.BitsPerPixel)
			b := Bitmap{int(v.DestLeft), int(v.DestTop), int(v.DestRight), int(v.DestBottom),
				int(v.Width), int(v.Height), Bpp(v.BitsPerPixel), IsCompress, data}
			//glog.Infof("b:%+v, %d==%d", b.DestLeft, len(b.Data), b.Width*b.Height*4)
			bs = append(bs, b)
		}
		ui_paint_bitmap(bs)
	})

	return nil, g
}

func (g *RdpClient) Login() error {
	domain, user, pwd := g.info.Domain, g.info.Username, g.info.Passwd
	glog.Info("Connect:", g.Host, "with", domain+"\\"+user, ":", pwd)
	conn, err := net.DialTimeout("tcp", g.Host, 3*time.Second)
	if err != nil {
		return fmt.Errorf("[dial err] %v", err)
	}
	//defer conn.Close()

	g.tpkt = tpkt.New(core.NewSocketLayer(conn), nla.NewNTLMv2(domain, user, pwd))
	g.x224 = x224.New(g.tpkt)
	g.mcs = t125.NewMCSClient(g.x224)
	g.sec = sec.NewClient(g.mcs)
	g.pdu = pdu.NewClient(g.sec)

	g.mcs.SetClientCoreData(uint16(g.Width), uint16(g.Height))

	g.sec.SetUser(user)
	g.sec.SetPwd(pwd)
	g.sec.SetDomain(domain)
	//g.sec.SetClientAutoReconnect(3, core.Random(16))

	g.tpkt.SetFastPathListener(g.sec)
	g.sec.SetFastPathListener(g.pdu)
	g.pdu.SetFastPathSender(g.tpkt)

	//g.x224.SetRequestedProtocol(x224.PROTOCOL_RDP)
	//g.x224.SetRequestedProtocol(x224.PROTOCOL_SSL)

	err = g.x224.Connect()
	if err != nil {
		return fmt.Errorf("[x224 connect err] %v", err)
	}
	glog.Info("wait connect ok")
	return nil
}

func (g *RdpClient) KeyUp(sc int, name string) {
	glog.Debug("KeyUp:", sc, "name:", name)

	p := &pdu.ScancodeKeyEvent{}
	p.KeyCode = uint16(sc)
	p.KeyboardFlags |= pdu.KBDFLAGS_RELEASE
	g.pdu.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
}
func (g *RdpClient) KeyDown(sc int, name string) {
	glog.Debug("KeyDown:", sc, "name:", name)

	p := &pdu.ScancodeKeyEvent{}
	p.KeyCode = uint16(sc)
	g.pdu.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
}

func (g *RdpClient) MouseMove(x, y int) {
	glog.Debug("MouseMove", x, ":", y)
	p := &pdu.PointerEvent{}
	p.PointerFlags |= pdu.PTRFLAGS_MOVE
	p.XPos = uint16(x)
	p.YPos = uint16(y)
	g.pdu.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
}

func (g *RdpClient) MouseWheel(scroll, x, y int) {
	glog.Info("MouseWheel", x, ":", y)
	p := &pdu.PointerEvent{}
	p.PointerFlags |= pdu.PTRFLAGS_WHEEL
	p.XPos = uint16(x)
	p.YPos = uint16(y)
	g.pdu.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
}

func (g *RdpClient) MouseUp(button int, x, y int) {
	glog.Debug("MouseUp", x, ":", y, ":", button)
	p := &pdu.PointerEvent{}

	switch button {
	case 0:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON1
	case 2:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON2
	case 1:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON3
	default:
		p.PointerFlags |= pdu.PTRFLAGS_MOVE
	}

	p.XPos = uint16(x)
	p.YPos = uint16(y)
	g.pdu.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
}
func (g *RdpClient) MouseDown(button int, x, y int) {
	glog.Info("MouseDown:", x, ":", y, ":", button)
	p := &pdu.PointerEvent{}

	p.PointerFlags |= pdu.PTRFLAGS_DOWN

	switch button {
	case 0:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON1
	case 2:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON2
	case 1:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON3
	default:
		p.PointerFlags |= pdu.PTRFLAGS_MOVE
	}

	p.XPos = uint16(x)
	p.YPos = uint16(y)
	g.pdu.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
}
func (g *RdpClient) Close() {
	if g != nil && g.tpkt != nil {
		g.tpkt.Close()
	}
}
