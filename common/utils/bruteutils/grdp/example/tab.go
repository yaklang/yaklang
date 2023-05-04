// tab.go
package main

import (
	"fmt"
)

type TabbedEditor struct {
	mixins.PanelHolder

	editors map[string]text.Editor

	driver      gxui.Driver
	cmdr        Commander
	theme       *basic.Theme
	syntaxTheme theme.Theme
	font        gxui.Font
	cur         string
}

func (e *TabbedEditor) CreatePanelTab() mixins.PanelTab {
	tab := basic.CreatePanelTab(e.theme)
	tab.OnMouseDown(func(ev gxui.MouseEvent) {
		if e.CurrentEditor() != nil {
			e.cur = e.CurrentEditor().Filepath()
		}
	})
	tab.OnMouseUp(func(gxui.MouseEvent) {
		if e.CurrentEditor() == nil {
			if len(e.editors) <= 1 {
				e.purgeSelf()
			} else {
				delete(e.editors, e.cur)
			}
		}
	})

	return tab
}
