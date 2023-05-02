package cui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/atotto/clipboard"
	"github.com/rocket049/gocui"
	"github.com/tevino/abool"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"yaklang.io/yaklang/common/log"
	utils2 "yaklang.io/yaklang/common/utils"
)

var (
	framesMux = new(sync.Mutex)
	promptUse = abool.New()
)

type App struct {
	UI *gocui.Gui

	frameRobin *utils2.StringRoundRobinSelector

	frames              map[string]*AppFrame
	updateHandler       func(g *gocui.Gui) error
	beforeStartCallback []func(app *App) error
}

func NewApp() (*App, error) {
	ui, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return nil, fmt.Errorf("new app cui failed: %s", err)
	}
	return &App{
		UI:         ui,
		frames:     make(map[string]*AppFrame),
		frameRobin: utils2.NewStringRoundRobinSelector(),
	}, nil
}

func (a *App) AddToLayout(frame *AppFrame) {
	a.AddToLayoutWithTabSwitcher(frame, true)
}

func (a *App) AddToLayoutWithTabSwitcher(frame *AppFrame, tabToSwitch bool) {
	framesMux.Lock()
	defer framesMux.Unlock()

	if !strings.HasPrefix(frame.name, "__") && tabToSwitch {
		a.frameRobin.Add(frame.name)
	}

	a.frames[frame.name] = frame
}

func (a *App) BeforeStart(cbs ...func(app *App) error) {
	a.beforeStartCallback = append(a.beforeStartCallback, cbs...)
}

func (a *App) ModalMessage(title string, in io.Reader) error {
	return a.ModalMessageWithPostionSetterOnEvent(title, in, func(x, y int) (i int, i2 int, i3 int, i4 int) {
		return x / 4, y / 4, x - x/3, y - y/4
	}, nil, nil)
}

func (a *App) ModalMessageWithEvent(title string, in io.Reader, ok, cancel func(g *gocui.Gui)) error {
	return a.ModalMessageWithPostionSetterOnEvent(title, in, func(x, y int) (i int, i2 int, i3 int, i4 int) {
		return x / 4, y / 4, x - x/3, y - y/4
	}, ok, cancel)
}

func (a *App) ModalMessageWithPostionSetterOnEvent(title string, in io.Reader,
	pos PositionSetter,

	// 当选择
	ok func(g *gocui.Gui),
	cancel func(g *gocui.Gui),
) error {
	if promptUse.IsSet() {
		return errors.New("prompt is used")
	}

	promptUse.Set()

	getRandName := func() string {
		return "__" + utils2.RandStringBytes(10)
	}

	var (
		alertName        = getRandName()
		okButtonName     = getRandName()
		cancelButtonName = getRandName()
	)

	_ = cancelButtonName

	alert := NewAppFrame(a, alertName, pos)
	alert.Init(func(app *App, g *gocui.Gui, v *gocui.View) error {
		v.Title = title
		v.Wrap = true
		go func() {
			_, _ = io.Copy(v, in)
		}()

		getExitHandler := func(cb func(g *gocui.Gui)) func(gui *gocui.Gui, view *gocui.View) error {
			return func(gui *gocui.Gui, view *gocui.View) error {
				a.DeleteAppFrameByName(alertName)
				a.DeleteAppFrameByName(okButtonName)
				a.DeleteAppFrameByName(cancelButtonName)
				defer promptUse.UnSet()

				if cb != nil {
					cb(gui)
				}
				return nil
			}
		}

		// 绑定删除 alert 的操作
		_ = g.SetKeybinding(alertName, 'q', gocui.ModNone, getExitHandler(cancel))

		// 绑定取消操作
		_ = g.SetKeybinding(alertName, gocui.KeyEnter, gocui.ModNone, getExitHandler(ok))

		// 创建按钮
		aX0, aY0, aX1, aY1, err := g.ViewPosition(alertName)
		_ = aY0
		if err != nil {
			return nil
		}
		okButton := NewAppFrame(app, okButtonName, func(x, y int) (i int, i2 int, i3 int, i4 int) {
			return aX0, aY1, aX0 + (aX1-aX0)/2, aY1 + 2
		})
		okButton.Init(func(app *App, g *gocui.Gui, v *gocui.View) error {
			okBanner := "Ok[Enter]"
			width, _ := v.Size()
			prefix := strings.Repeat(" ", (width-len(okBanner))/2)
			_, _ = v.Write([]byte(prefix + okBanner))
			//_ = g.SetKeybinding(okButtonName, gocui.MouseLeft, gocui.ModNone, getExitHandler(ok))
			return nil
		})
		app.AddToLayout(okButton)

		cancelButton := NewAppFrame(app, cancelButtonName, func(x, y int) (i int, i2 int, i3 int, i4 int) {
			return aX0 + (aX1-aX0)/2, aY1, aX1, aY1 + 2
		})
		cancelButton.Init(func(app *App, g *gocui.Gui, v *gocui.View) error {
			okBanner := "Cancel[Q]"
			width, _ := v.Size()
			prefix := strings.Repeat(" ", (width-len(okBanner))/2)
			_, _ = v.Write([]byte(prefix + okBanner))
			//_ = g.SetKeybinding(okButtonName, gocui.MouseLeft, gocui.ModNone, getExitHandler(ok))
			return nil
		})
		app.AddToLayout(cancelButton)

		_, _ = g.SetCurrentView(alertName)
		return nil
	}, EnableScroller(alertName))
	a.AddToLayout(alert)
	return nil
}

func (a *App) Run(ctx context.Context, cbs ...func()) error {
	a.UI.SetManagerFunc(func(gui *gocui.Gui) error {
		x, y := gui.Size()
		for name, frame := range a.frames {
			x0, y0, x1, y1 := frame.Position(x, y)
			view, err := gui.SetView(name, x0, y0, x1, y1)
			if err == gocui.ErrUnknownView {
				if frame.initCb != nil {
					err := frame.initCb(a, gui, view)
					if err != nil {
						return fmt.Errorf("init view[%s] failed: %s", name, err)
					}
				}
			} else if err != nil {
				return fmt.Errorf("generate view[%s] failed: %s", name, err)
			}

			if frame.updatedCb != nil {
				_ = frame.updatedCb(a, gui, view)
			}

		}

		return nil
	})

	err := a.bindKeys(cbs...)
	if err != nil {
		return fmt.Errorf("bind keys failed; %s", err)
	}

	for _, c := range a.beforeStartCallback {
		err := c(a)
		if err != nil {
			return fmt.Errorf("execute func beforeStart failed: %s", err)
		}
	}

	go func() {
		ticker := time.Tick(200 * time.Millisecond)
		for {
			select {
			case <-ticker:
				a.UI.Update(func(gui *gocui.Gui) error {
					if a.updateHandler != nil {
						return a.updateHandler(gui)
					}
					return nil
				})
			case <-ctx.Done():
				return
			}
		}
	}()

	errC := make(chan error)
	go func() {
		errC <- a.UI.MainLoop()
	}()

	select {
	case <-ctx.Done():
		a.Close()
		return gocui.ErrQuit
	case e := <-errC:
		return e
	}
}

func (a *App) bindKeys(cbs ...func()) error {
	if err := a.UI.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, func(gui *gocui.Gui, view *gocui.View) error {
		_ = a.ModalMessageWithEvent("ALERT", bytes.NewBufferString("你想要退出 Console UI 吗？"), func(g *gocui.Gui) {
			a.UI.Close()
		}, nil)
		//return gocui.ErrQuit
		return nil
	}); err != nil {
		return fmt.Errorf("bind control-C failed; %s", err)
	}

	for _, viewName := range a.frameRobin.List() {
		if err := a.UI.SetKeybinding(viewName, gocui.MouseLeft, gocui.ModNone,
			func(gui *gocui.Gui, view *gocui.View) error {
				if promptUse.IsSet() {
					return nil
				}

				if _, err := gui.SetCurrentView(view.Name()); err != nil {
					return fmt.Errorf("set current view [%s] failed: %s", view.Name(), err)
				}

				_, cy := view.Cursor()
				_, oy := view.Origin()
				if l, err := view.Line(cy); err == nil {
					log.Infof("curser oy: %v cy: %v, lines[cy] %s:", oy, cy, l)
					frame, _ := a.GetFrameByName(view.Name())
					if frame != nil {
						frame.onLineSelected(l, gui, view)
					}
				} else {
					log.Errorf("cursor cy: %v line[cy] failed: %s", cy, err)
				}
				view.Highlight = true
				view.SelBgColor = gocui.ColorGreen
				view.SelFgColor = gocui.ColorBlack

				for _, otherViewName := range a.frameRobin.List() {
					if otherViewName != view.Name() {
						otherView, _ := gui.View(otherViewName)
						if otherView != nil {
							otherView.Highlight = false
						}
					}
				}

				return nil
			}); err != nil {
			return fmt.Errorf("bind mouse left to [%s] failed: %s", viewName, err)
		}
		if err := a.UI.SetKeybinding(viewName, 'c', gocui.ModNone, func(gui *gocui.Gui, view *gocui.View) error {
			err := clipboard.WriteAll(view.Buffer())
			if err != nil {
				_ = a.ModalMessage("Alert", bytes.NewBuffer(
					[]byte(fmt.Sprintf("[%s] 's clipboard write operation failed: %s", view.Name(), err)),
				))
				return nil
			}
			_ = a.ModalMessage("Information", bytes.NewBuffer([]byte(fmt.Sprintf(
				"You have already copied view from [%s] to your clipboard, to paste other places", view.Name(),
			))))
			return nil
		}); err != nil {
			return fmt.Errorf("set mouse right binding for [%s] failed: %s", viewName, err)
		}
		if err := a.UI.SetKeybinding(viewName, gocui.KeyCtrlS, gocui.ModNone, func(gui *gocui.Gui, view *gocui.View) error {
			homeDir, _ := os.Getwd()
			if homeDir == "" {
				homeDir = ""
			}
			fileName := path.Join(homeDir, fmt.Sprintf("cui-%s-output-%s.txt", view.Name(), time.Now().Format("2006-01-02T15-04-05Z-0700")))
			err := ioutil.WriteFile(fileName, []byte(view.Buffer()), 0666)
			if err != nil {
				_ = a.ModalMessage("Alert", bytes.NewBufferString(
					fmt.Sprintf("write [%s] to file[%s] failed: %s", view.Name(), fileName, err),
				))
			}
			_ = a.ModalMessage("Information", bytes.NewBufferString(
				fmt.Sprintf("[%s]'s content have already to file: %s", view.Name(), fileName),
			))
			return nil
		}); err != nil {
			return fmt.Errorf("[%s] bind control s failed: %s", viewName, err)
		}
	}

	if err := a.UI.SetKeybinding(
		"", gocui.KeyTab, gocui.ModNone,
		func(gui *gocui.Gui, view *gocui.View) error {
			if promptUse.IsSet() {
				return nil
			}

			_, err := gui.SetCurrentView(a.frameRobin.Next())
			if err != nil {
				return nil
			}

			return nil
		}); err != nil {

	}

	//
	for _, c := range cbs {
		c()
	}

	return nil
}

func (a *App) GetFrameByName(name string) (*AppFrame, bool) {
	f, ok := a.frames[name]
	return f, ok
}

func (a *App) DeleteAppFrameByName(name string) {
	framesMux.Lock()
	defer framesMux.Unlock()

	// 如果本身不存在，就不继续操作
	if _, ok := a.frames[name]; !ok {
		return
	}

	delete(a.frames, name)

	ui := a.UI
	if ui == nil {
		return
	}

	ui.DeleteKeybindings(name)
	_ = ui.DeleteView(name)
}

func (a *App) Close() {
	a.updateHandler = func(g *gocui.Gui) error {
		return gocui.ErrQuit
	}
}
