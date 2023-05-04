// main.go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	socketio "github.com/googollee/go-socket.io"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/pdu"
)

func showPreview(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("static/html/index.html")
	if err != nil {
		w.Write([]byte(err.Error() + "\n"))
		return
	}
	w.Header().Add("Content-Type", "text/html")
	t.Execute(w, nil)

}

func socketIO() {
	server := socketio.NewServer(nil)
	server.OnConnect("/", func(so socketio.Conn) error {
		fmt.Println("OnConnect", so.ID())
		so.Emit("rdp-connect", true)
		return nil
	})
	server.OnEvent("/", "infos", func(so socketio.Conn, data interface{}) {
		fmt.Println("infos", so.ID())
		var info Info
		v, _ := json.Marshal(data)
		json.Unmarshal(v, &info)
		fmt.Println(so.ID(), "logon infos:", info)

		g := NewRdpClient(fmt.Sprintf("%s:%s", info.Ip, info.Port), info.Width, info.Height, glog.INFO)
		g.info = &info
		err := g.Login()
		if err != nil {
			fmt.Println("Login:", err)
			so.Emit("rdp-error", "{\"code\":1,\"message\":\""+err.Error()+"\"}")
			return
		}
		so.SetContext(g)
		g.pdu.On("error", func(e error) {
			fmt.Println("on error:", e)
			so.Emit("rdp-error", "{\"code\":1,\"message\":\""+e.Error()+"\"}")
			//wg.Done()
		}).On("close", func() {
			err = errors.New("close")
			fmt.Println("on close")
		}).On("success", func() {
			fmt.Println("on success")
		}).On("ready", func() {
			fmt.Println("on ready")
		}).On("update", func(rectangles []pdu.BitmapData) {
			glog.Info(time.Now(), "on update Bitmap:", len(rectangles))
			bs := make([]Bitmap, 0, len(rectangles))
			for _, v := range rectangles {
				IsCompress := v.IsCompress()
				data := v.BitmapDataStream
				glog.Debug("data:", data)
				if IsCompress {
					//data = decompress(&v)
					//IsCompress = false
				}

				glog.Debug(IsCompress, v.BitsPerPixel)
				b := Bitmap{int(v.DestLeft), int(v.DestTop), int(v.DestRight), int(v.DestBottom),
					int(v.Width), int(v.Height), int(v.BitsPerPixel), IsCompress, data}
				so.Emit("rdp-bitmap", []Bitmap{b})
				bs = append(bs, b)
			}
			so.Emit("rdp-bitmap", bs)
		})
	})

	server.OnEvent("/", "mouse", func(so socketio.Conn, x, y uint16, button int, isPressed bool) {
		glog.Info("mouse", x, ":", y, ":", button, ":", isPressed)
		p := &pdu.PointerEvent{}
		if isPressed {
			p.PointerFlags |= pdu.PTRFLAGS_DOWN
		}

		switch button {
		case 1:
			p.PointerFlags |= pdu.PTRFLAGS_BUTTON1
		case 2:
			p.PointerFlags |= pdu.PTRFLAGS_BUTTON2
		case 3:
			p.PointerFlags |= pdu.PTRFLAGS_BUTTON3
		default:
			p.PointerFlags |= pdu.PTRFLAGS_MOVE
		}

		p.XPos = x
		p.YPos = y
		g := so.Context().(*RdpClient)
		g.pdu.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
	})

	//keyboard
	server.OnEvent("/", "scancode", func(so socketio.Conn, button uint16, isPressed bool) {
		glog.Info("scancode:", "button:", button, "isPressed:", isPressed)

		p := &pdu.ScancodeKeyEvent{}
		p.KeyCode = button
		if !isPressed {
			p.KeyboardFlags |= pdu.KBDFLAGS_RELEASE
		}
		g := so.Context().(*RdpClient)
		g.pdu.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})

	})

	//wheel
	server.OnEvent("/", "wheel", func(so socketio.Conn, x, y, step uint16, isNegative, isHorizontal bool) {
		glog.Info("wheel", x, ":", y, ":", step, ":", isNegative, ":", isHorizontal)
		var p = &pdu.PointerEvent{}
		if isHorizontal {
			p.PointerFlags |= pdu.PTRFLAGS_HWHEEL
		} else {
			p.PointerFlags |= pdu.PTRFLAGS_WHEEL
		}

		if isNegative {
			p.PointerFlags |= pdu.PTRFLAGS_WHEEL_NEGATIVE
		}

		p.PointerFlags |= (step & pdu.WheelRotationMask)
		p.XPos = x
		p.YPos = y
		g := so.Context().(*RdpClient)
		g.pdu.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
	})

	server.OnError("/", func(so socketio.Conn, err error) {
		if so == nil || so.Context() == nil {
			return
		}
		fmt.Println("error:", err)
		g := so.Context().(*RdpClient)
		if g != nil {
			g.tpkt.Close()
		}
		so.Close()
	})

	server.OnDisconnect("/", func(so socketio.Conn, s string) {
		if so.Context() == nil {
			return
		}
		fmt.Println("OnDisconnect:", s)
		so.Emit("rdp-error", "{code:1,message:"+s+"}")

		g := so.Context().(*RdpClient)
		if g != nil {
			g.tpkt.Close()
		}
		so.Close()
	})
	go server.Serve()
	defer server.Close()

	http.Handle("/socket.io/", server)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.Handle("/css/", http.FileServer(http.Dir("static")))
	http.Handle("/js/", http.FileServer(http.Dir("static")))
	http.Handle("/img/", http.FileServer(http.Dir("static")))
	http.HandleFunc("/", showPreview)

	log.Println("Serving at localhost:8088...")
	log.Fatal(http.ListenAndServe(":8088", nil))
}
