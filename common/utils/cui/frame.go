package cui

import (
	"bytes"
	"fmt"
	"github.com/rocket049/gocui"
	"io"
)

type ViewHandler func(app *App, g *gocui.Gui, v *gocui.View) error
type LineSelectedHandler func(line string, app *App, g *gocui.Gui, v *gocui.View)

type PositionSetter func(x, y int) (int, int, int, int)

type AppFrame struct {
	app *App

	name     string
	Position PositionSetter

	initCb                ViewHandler
	updatedCb             ViewHandler
	lineSelectedCallbacks []LineSelectedHandler

	view *gocui.View
}

func NewAppFrame(app *App, name string, position PositionSetter) *AppFrame {
	frame := &AppFrame{
		app: app, name: name, Position: position,
	}
	return frame
}

func (a *AppFrame) Init(cb ...ViewHandler) {
	a.initCb = func(app *App, g *gocui.Gui, v *gocui.View) error {
		for _, callback := range cb {
			err := callback(app, g, v)
			if err != nil {
				panic(fmt.Sprintf("init failed: %s", err))
				return err
			}
		}
		return nil
	}
}

func (a *AppFrame) OnUpdated(cb ...ViewHandler) {
	a.updatedCb = func(app *App, g *gocui.Gui, v *gocui.View) error {
		for _, callback := range cb {
			err := callback(app, g, v)
			if err != nil {
				panic(fmt.Sprintf("update failed: %s", err))
				return err
			}
		}
		return nil
	}
}

func (a *AppFrame) OnLineSelected(cb ...LineSelectedHandler) {
	a.lineSelectedCallbacks = cb
}

func (a *AppFrame) onLineSelected(line string, g *gocui.Gui, v *gocui.View) {
	for _, c := range a.lineSelectedCallbacks {
		c(line, a.app, g, v)
	}
}

func (a *AppFrame) Update() {
	//_, _ = a.app.UI.SetCurrentView(a.name)
}

// 专门用来显示状态的 Framework
func NewStatusAppFrame(app *App, name string, position PositionSetter, showValue *[]byte, cb ...ViewHandler) *AppFrame {
	frame := NewAppFrame(app, name, position)
	cb = append(cb, EnableScroller(name))
	frame.Init(cb...)
	frame.OnUpdated(func(app *App, g *gocui.Gui, v *gocui.View) error {
		v.Clear()
		_, _ = io.Copy(v, bytes.NewBuffer(*showValue))
		return nil
	})
	return frame
}

func NewAppFrameWithViewWriter(app *App, name string, position PositionSetter, initCb ...ViewHandler) (*AppFrame, io.Writer) {
	reader, writer := io.Pipe()

	frame := NewAppFrame(app, name, position)

	initCb = append(initCb, func(app *App, g *gocui.Gui, v *gocui.View) error {
		go func() {
			_, _ = io.Copy(v, reader)
		}()
		return nil
	}, EnableScroller(name), EnableNoInputEditor(name))

	frame.Init(initCb...)
	return frame, writer
}
