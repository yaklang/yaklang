package cui

import (
	"fmt"
	"github.com/rocket049/gocui"
)

func ScrollView(v *gocui.View, dy int) error {
	if v != nil {
		v.Autoscroll = false
		ox, oy := v.Origin()

		nextOy := oy + dy

		if nextOy < 0 {
			nextOy = 0
		}

		if err := v.SetOrigin(ox, nextOy); err != nil {
			return err
		}
	}
	return nil
}

// 启动手动滚动 View 的模式，应该在 Init 中设置
func EnableScroller(name string) ViewHandler {
	return func(app *App, g *gocui.Gui, v *gocui.View) error {
		v.Autoscroll = true
		if err := app.UI.SetKeybinding(
			name, gocui.KeyArrowDown, gocui.ModNone,
			func(gui *gocui.Gui, view *gocui.View) error {
				return ScrollView(view, 1)
			},
		); err != nil {
			return fmt.Errorf("bind arrow down failed: %s", err)
		}

		if err := app.UI.SetKeybinding(
			name, gocui.KeyArrowUp, gocui.ModNone,
			func(gui *gocui.Gui, view *gocui.View) error {
				return ScrollView(view, -1)
			},
		); err != nil {
			return fmt.Errorf("bind arrow up failed: %s", err)
		}

		if err := app.UI.SetKeybinding(
			name, gocui.MouseWheelUp, gocui.ModNone,
			func(gui *gocui.Gui, view *gocui.View) error {
				return ScrollView(view, 1)
			},
		); err != nil {
			return fmt.Errorf("bind mouse wheel up failed: %s", err)
		}

		if err := app.UI.SetKeybinding(
			name, gocui.MouseWheelDown, gocui.ModNone,
			func(gui *gocui.Gui, view *gocui.View) error {
				return ScrollView(view, -1)
			},
		); err != nil {
			return fmt.Errorf("bind mouse wheel down failed: %s", err)
		}

		if err := app.UI.SetKeybinding(
			name, 'f', gocui.ModNone,
			func(gui *gocui.Gui, view *gocui.View) error {
				view.Autoscroll = true
				return nil
			},
		); err != nil {
			return fmt.Errorf("bind f[tail -f] failed: %s", err)
		}

		return nil
	}
}

var (
	NoInputEditor gocui.EditorFunc = func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
		return
	}
)

func EnableNoInputEditor(name string) ViewHandler {
	return func(app *App, g *gocui.Gui, v *gocui.View) error {
		v.Editable = true
		v.Editor = NoInputEditor
		return nil
	}
}
