// ui.go
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"runtime"
	"strconv"
	"time"

	"github.com/google/gxui/drivers/gl"

	"github.com/google/gxui"
	"github.com/google/gxui/samples/flags"
	"github.com/google/gxui/themes/light"
	"yaklang/common/utils/bruteutils/grdp/core"
	"yaklang/common/utils/bruteutils/grdp/glog"
)

var (
	gc            Control
	driverc       gxui.Driver
	width, height int
)

func StartUI(w, h int) {
	width, height = w, h
	gl.StartDriver(appMain)
}
func appMain(driver gxui.Driver) {
	theme := light.CreateTheme(driver)
	window := theme.CreateWindow(width, height, "MSTSC")
	window.SetScale(flags.DefaultScaleFactor)

	img = theme.CreateImage()

	layoutImg := theme.CreateLinearLayout()
	layoutImg.SetSizeMode(gxui.Fill)
	layoutImg.SetHorizontalAlignment(gxui.AlignCenter)
	layoutImg.AddChild(img)
	layoutImg.SetVisible(false)
	ScreenImage = image.NewRGBA(image.Rect(0, 0, width, height))
	layoutImg.OnMouseDown(func(e gxui.MouseEvent) {
		gc.MouseDown(int(e.Button), e.Point.X, e.Point.Y)
	})
	layoutImg.OnMouseUp(func(e gxui.MouseEvent) {
		gc.MouseUp(int(e.Button), e.Point.X, e.Point.Y)
	})
	layoutImg.OnMouseMove(func(e gxui.MouseEvent) {
		//gc.MouseMove(e.Point.X, e.Point.Y)
	})
	layoutImg.OnMouseScroll(func(e gxui.MouseEvent) {
		//gc.MouseWheel(e.ScrollY, e.Point.X, e.Point.Y)
	})
	layoutImg.OnKeyDown(func(e gxui.KeyboardEvent) {
		fmt.Println("layoutImg OnKeyDown:", int(e.Key))
		key := int(e.Key)
		if key == 52 {
			key = 65293
		}
		gc.KeyDown(key, "")
	})
	layoutImg.OnKeyUp(func(e gxui.KeyboardEvent) {
		fmt.Println("layoutImg OnKeyUp:", int(e.Key))
		key := int(e.Key)
		if key == 52 {
			key = 65293
		}
		gc.KeyUp(key, "")
	})

	layout := theme.CreateLinearLayout()
	layout.SetSizeMode(gxui.Fill)
	layout.SetHorizontalAlignment(gxui.AlignCenter)

	label := theme.CreateLabel()
	label.SetText("Welcome Mstsc")
	label.SetColor(gxui.Red)
	ip := theme.CreateTextBox()
	user := theme.CreateTextBox()
	passwd := theme.CreateTextBox()
	ip.SetDesiredWidth(width / 4)
	user.SetDesiredWidth(width / 4)
	passwd.SetDesiredWidth(width / 4)
	//ip.SetText("192.168.18.100:5902")
	ip.SetText("192.168.0.132:3389")
	user.SetText("administrator")
	//user.SetText("wren")
	passwd.SetText("Jhadmin123")
	//passwd.SetText("wren")

	bok := theme.CreateButton()
	bok.SetText("OK")
	bok.OnClick(func(e gxui.MouseEvent) {
		err, info := NewInfo(ip.Text(), user.Text(), passwd.Text())
		info.Width, info.Height = width, height
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		driverc = driver
		err, gc = uiClient(info)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		glog.Info("ok:", gc)
		layout.SetVisible(false)
		layoutImg.SetVisible(true)
		ip.GainedFocus()
	})
	bcancel := theme.CreateButton()
	bcancel.SetText("Clear")
	bcancel.OnClick(func(e gxui.MouseEvent) {
		ip.SetText("")
		user.SetText("")
		passwd.SetText("")
	})
	blayout := theme.CreateLinearLayout()
	blayout.AddChild(bok)
	blayout.AddChild(bcancel)

	table := theme.CreateTableLayout()
	table.SetGrid(3, 20) // columns, rows
	table.SetChildAt(1, 4, 1, 1, ip)
	table.SetChildAt(1, 5, 1, 1, user)
	table.SetChildAt(1, 6, 1, 1, passwd)
	table.SetChildAt(1, 7, 1, 1, blayout)
	layout.AddChild(label)
	layout.AddChild(table)
	//layout.AddChild(blayout)

	window.AddChild(layout)
	window.AddChild(layoutImg)
	window.OnClose(func() {
		if gc != nil {
			gc.Close()
		}

		driver.Terminate()
	})
	update()
}

var (
	ScreenImage *image.RGBA
	img         gxui.Image
)

func update() {
	go func() {
		for {
			select {
			case bs := <-BitmapCH:
				paint_bitmap(bs)
			default:
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

func ToRGBA(pixel int, i int, data []byte) (r, g, b, a uint8) {
	a = 255
	switch pixel {
	case 1:
	case 2:
		rgb565 := core.Uint16BE(data[i], data[i+1])
		r, g, b = core.RGB565ToRGB(rgb565)
	case 3:
	case 4:
		fallthrough
	default:
		r, g, b = data[i+2], data[i+1], data[i]
	}

	return
}

func paint_bitmap(bs []Bitmap) {
	var (
		pixel      int
		i          int
		r, g, b, a uint8
	)

	for _, bm := range bs {
		i = 0
		pixel = bm.BitsPerPixel
		m := image.NewRGBA(image.Rect(0, 0, bm.Width, bm.Height))
		for y := 0; y < bm.Height; y++ {
			for x := 0; x < bm.Width; x++ {
				r, g, b, a = ToRGBA(pixel, i, bm.Data)
				c := color.RGBA{r, g, b, a}
				i += pixel
				m.Set(x, y, c)
			}
		}

		draw.Draw(ScreenImage, ScreenImage.Bounds().Add(image.Pt(bm.DestLeft, bm.DestTop)), m, m.Bounds().Min, draw.Src)
	}

	driverc.Call(func() {
		texture := driverc.CreateTexture(ScreenImage, 1)
		img.SetTexture(texture)
	})

}

var BitmapCH chan []Bitmap

func ui_paint_bitmap(bs []Bitmap) {
	BitmapCH <- bs
}

func uiClient(info *Info) (error, Control) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var (
		err error
		g   Control
	)
	if true {
		err, g = uiRdp(info)
	} else {
		err, g = uiVnc(info)
	}

	return err, g
}

type Bitmap struct {
	DestLeft     int    `json:"destLeft"`
	DestTop      int    `json:"destTop"`
	DestRight    int    `json:"destRight"`
	DestBottom   int    `json:"destBottom"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	BitsPerPixel int    `json:"bitsPerPixel"`
	IsCompress   bool   `json:"isCompress"`
	Data         []byte `json:"data"`
}

func Bpp(BitsPerPixel uint16) (pixel int) {
	switch BitsPerPixel {
	case 15:
		pixel = 1

	case 16:
		pixel = 2

	case 24:
		pixel = 3

	case 32:
		pixel = 4

	default:
		glog.Error("invalid bitmap data format")
	}
	return
}

func Hex2Dec(val string) int {
	n, err := strconv.ParseUint(val, 16, 32)
	if err != nil {
		fmt.Println(err)
	}
	return int(n)
}

type Control interface {
	Login() error
	SetRequestedProtocol(p uint32)
	KeyUp(sc int, name string)
	KeyDown(sc int, name string)
	MouseMove(x, y int)
	MouseWheel(scroll, x, y int)
	MouseUp(button int, x, y int)
	MouseDown(button int, x, y int)
	Close()
}

/*func translateKeyboardKey(in gxui.KeyboardKey) int {
	switch in {
	case gxui.KeySpace:
		return glfw.KeySpace
	case gxui.KeyApostrophe:
		return glfw.KeyApostrophe
	case gxui.KeyComma:
		return glfw.KeyComma
	case gxui.KeyMinus:
		return glfw.KeyMinus
	case gxui.KeyPeriod:
		return glfw.KeyPeriod
	case gxui.KeySlash:
		return glfw.KeySlash
	case gxui.Key0:
		//return glfw.Key0
		return glfw.Key(11)
	case gxui.Key1:
		return 0x2
	case gxui.Key2:
		return glfw.Key2
	case gxui.Key3:
		return glfw.Key3
	case gxui.Key4:
		return glfw.Key4
	case gxui.Key5:
		return glfw.Key5
	case gxui.Key6:
		return glfw.Key6
	case gxui.Key7:
		return glfw.Key7
	case gxui.Key8:
		return glfw.Key8
	case gxui.Key9:
		return glfw.Key9
	case gxui.KeySemicolon:
		return glfw.KeySemicolon
	case gxui.KeyEqual:
		return glfw.KeyEqual
	case gxui.KeyA:
		return glfw.KeyA
	case gxui.KeyB:
		return glfw.KeyB
	case gxui.KeyC:
		return glfw.KeyC
	case gxui.KeyD:
		return glfw.KeyD
	case gxui.KeyE:
		return glfw.KeyE
	case gxui.KeyF:
		return glfw.KeyF
	case gxui.KeyG:
		return glfw.KeyG
	case gxui.KeyH:
		return glfw.KeyH
	case gxui.KeyI:
		return glfw.KeyI
	case gxui.KeyJ:
		return glfw.KeyJ
	case gxui.KeyK:
		return glfw.KeyK
	case gxui.KeyL:
		return glfw.KeyL
	case gxui.KeyM:
		return glfw.KeyM
	case gxui.KeyN:
		return glfw.KeyN
	case gxui.KeyO:
		return glfw.KeyO
	case gxui.KeyP:
		return glfw.KeyP
	case gxui.KeyQ:
		return glfw.KeyQ
	case gxui.KeyR:
		return glfw.KeyR
	case gxui.KeyS:
		return glfw.KeyS
	case gxui.KeyT:
		return glfw.KeyT
	case gxui.KeyU:
		return glfw.KeyU
	case gxui.KeyV:
		return glfw.KeyV
	case gxui.KeyW:
		return glfw.KeyW
	case gxui.KeyX:
		return glfw.KeyX
	case gxui.KeyY:
		return glfw.KeyY
	case gxui.KeyZ:
		return glfw.KeyZ
	case gxui.KeyLeftBracket:
		return glfw.KeyLeftBracket
	case gxui.KeyBackslash:
		return glfw.KeyBackslash
	case gxui.KeyRightBracket:
		return glfw.KeyRightBracket
	case gxui.KeyGraveAccent:
		return glfw.KeyGraveAccent
	case gxui.KeyWorld1:
		return glfw.KeyWorld1
	case gxui.KeyWorld2:
		return glfw.KeyWorld2
	case gxui.KeyEscape:
		return glfw.KeyEscape
	case gxui.KeyEnter:
		return glfw.KeyEnter
	case gxui.KeyTab:
		return glfw.KeyTab
	case gxui.KeyBackspace:
		return glfw.KeyBackspace
	case gxui.KeyInsert:
		return glfw.KeyInsert
	case gxui.KeyDelete:
		return glfw.KeyDelete
	case gxui.KeyRight:
		return glfw.KeyRight
	case gxui.KeyLeft:
		return glfw.KeyLeft
	case gxui.KeyDown:
		return glfw.KeyDown
	case gxui.KeyUp:
		return glfw.KeyUp
	case gxui.KeyPageUp:
		return glfw.KeyPageUp
	case gxui.KeyPageDown:
		return glfw.KeyPageDown
	case gxui.KeyHome:
		return glfw.KeyHome
	case gxui.KeyEnd:
		return glfw.KeyEnd
	case gxui.KeyCapsLock:
		return glfw.KeyCapsLock
	case gxui.KeyScrollLock:
		return glfw.KeyScrollLock
	case gxui.KeyNumLock:
		return glfw.KeyNumLock
	case gxui.KeyPrintScreen:
		return glfw.KeyPrintScreen
	case gxui.KeyPause:
		return glfw.KeyPause
	case gxui.KeyF1:
		return glfw.KeyF1
	case gxui.KeyF2:
		return glfw.KeyF2
	case gxui.KeyF3:
		return glfw.KeyF3
	case gxui.KeyF4:
		return glfw.KeyF4
	case gxui.KeyF5:
		return glfw.KeyF5
	case gxui.KeyF6:
		return glfw.KeyF6
	case gxui.KeyF7:
		return glfw.KeyF7
	case gxui.KeyF8:
		return glfw.KeyF8
	case gxui.KeyF9:
		return glfw.KeyF9
	case gxui.KeyF10:
		return glfw.KeyF10
	case gxui.KeyF11:
		return glfw.KeyF11
	case gxui.KeyF12:
		return glfw.KeyF12
	case gxui.KeyKp0:
		return glfw.KeyKP0
	case gxui.KeyKp1:
		return glfw.KeyKP1
	case gxui.KeyKp2:
		return glfw.KeyKP2
	case gxui.KeyKp3:
		return glfw.KeyKP3
	case gxui.KeyKp4:
		return glfw.KeyKP4
	case gxui.KeyKp5:
		return glfw.KeyKP5
	case gxui.KeyKp6:
		return glfw.KeyKP6
	case gxui.KeyKp7:
		return glfw.KeyKP7
	case gxui.KeyKp8:
		return glfw.KeyKP8
	case gxui.KeyKp9:
		return glfw.KeyKP9
	case gxui.KeyKpDecimal:
		return glfw.KeyKPDecimal
	case gxui.KeyKpDivide:
		return glfw.KeyKPDivide
	case gxui.KeyKpMultiply:
		return glfw.KeyKPMultiply
	case gxui.KeyKpSubtract:
		return glfw.KeyKPSubtract
	case gxui.KeyKpAdd:
		return glfw.KeyKPAdd
	case gxui.KeyKpEnter:
		return glfw.KeyKPEnter
	case gxui.KeyKpEqual:
		return glfw.KeyKPEqual
	case gxui.KeyLeftShift:
		return glfw.KeyLeftShift
	case gxui.KeyLeftControl:
		return glfw.KeyLeftControl
	case gxui.KeyLeftAlt:
		return glfw.KeyLeftAlt
	case gxui.KeyLeftSuper:
		return glfw.KeyLeftSuper
	case gxui.KeyRightShift:
		return glfw.KeyRightShift
	case gxui.KeyRightControl:
		return glfw.KeyRightControl
	case gxui.KeyRightAlt:
		return glfw.KeyRightAlt
	case gxui.KeyRightSuper:
		return glfw.KeyRightSuper
	case gxui.KeyMenu:
		return glfw.KeyMenu
	default:
		return glfw.KeyUnknown
	}
}*/
