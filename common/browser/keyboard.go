// Package browser
// @Author bcy2007  2026/3/25 16:00
package browser

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod/lib/input"
)

func (p *BrowserPage) Press(key string) error {
	k, ok := parseInputKey(key)
	if !ok {
		return fmt.Errorf("invalid key: %v", key)
	}
	return p.page.Keyboard.Type(k)
}

func (p *BrowserPage) pressAction(keys string) error {
	// todo
	// press action not work in mac
	// in fact keyboard press not work, it looks like type once
	// to be tested in win
	actions := strings.Split(keys, "+")
	for i := 0; i < len(actions); i++ {
		err := p.KeyDown(actions[i])
		if err != nil {
			return err
		}
	}
	for i := len(actions) - 1; i >= 0; i-- {
		_ = p.KeyUp(actions[i])
	}
	return nil
}

func (p *BrowserPage) KeyDown(key string) error {
	k, ok := parseInputKey(key)
	if !ok {
		return fmt.Errorf("invalid key: %v", key)
	}
	return p.page.Keyboard.Press(k)
}

func (p *BrowserPage) KeyUp(key string) error {
	k, ok := parseInputKey(key)
	if !ok {
		return fmt.Errorf("invalid key: %v", key)
	}
	return p.page.Keyboard.Release(k)
}

func parseInputKey(key string) (input.Key, bool) {
	// keep whitespace like " " meaningful (Space)
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" && key != "" {
		// key is all-whitespace, allow single space only
		if key == " " {
			return input.Space, true
		}
		return 0, false
	}

	if mapped, ok := namedKeys[k]; ok {
		return mapped, true
	}

	// Single rune (printable or control) falls back to input.Key(r)
	rs := []rune(k)
	if len(rs) == 1 {
		return input.Key(rs[0]), true
	}

	return 0, false
}

var namedKeys = map[string]input.Key{
	// Functions row
	"escape": input.Escape,
	"esc":    input.Escape,
	"f1":     input.F1,
	"f2":     input.F2,
	"f3":     input.F3,
	"f4":     input.F4,
	"f5":     input.F5,
	"f6":     input.F6,
	"f7":     input.F7,
	"f8":     input.F8,
	"f9":     input.F9,
	"f10":    input.F10,
	"f11":    input.F11,
	"f12":    input.F12,

	// Numbers row
	"`":         input.Backquote,
	"backquote": input.Backquote,
	"1":         input.Digit1,
	"2":         input.Digit2,
	"3":         input.Digit3,
	"4":         input.Digit4,
	"5":         input.Digit5,
	"6":         input.Digit6,
	"7":         input.Digit7,
	"8":         input.Digit8,
	"9":         input.Digit9,
	"0":         input.Digit0,
	"-":         input.Minus,
	"minus":     input.Minus,
	"=":         input.Equal,
	"equal":     input.Equal,
	`\`:         input.Backslash,
	"backslash": input.Backslash,
	"backspace": input.Backspace,

	// First row
	"tab":          input.Tab,
	"q":            input.KeyQ,
	"w":            input.KeyW,
	"e":            input.KeyE,
	"r":            input.KeyR,
	"t":            input.KeyT,
	"y":            input.KeyY,
	"u":            input.KeyU,
	"i":            input.KeyI,
	"o":            input.KeyO,
	"p":            input.KeyP,
	"[":            input.BracketLeft,
	"]":            input.BracketRight,
	"bracketleft":  input.BracketLeft,
	"bracketright": input.BracketRight,

	// Second row
	"capslock":  input.CapsLock,
	"a":         input.KeyA,
	"s":         input.KeyS,
	"d":         input.KeyD,
	"f":         input.KeyF,
	"g":         input.KeyG,
	"h":         input.KeyH,
	"j":         input.KeyJ,
	"k":         input.KeyK,
	"l":         input.KeyL,
	";":         input.Semicolon,
	"'":         input.Quote,
	"semicolon": input.Semicolon,
	"quote":     input.Quote,
	"enter":     input.Enter,

	// Third row
	"shift":      input.ShiftLeft,
	"shiftleft":  input.ShiftLeft,
	"shiftright": input.ShiftRight,
	"z":          input.KeyZ,
	"x":          input.KeyX,
	"c":          input.KeyC,
	"v":          input.KeyV,
	"b":          input.KeyB,
	"n":          input.KeyN,
	"m":          input.KeyM,
	",":          input.Comma,
	"comma":      input.Comma,
	".":          input.Period,
	"period":     input.Period,
	"/":          input.Slash,
	"slash":      input.Slash,

	// Last row
	"ctrl":         input.ControlLeft,
	"control":      input.ControlLeft,
	"controlleft":  input.ControlLeft,
	"controlright": input.ControlRight,
	"meta":         input.MetaLeft,
	"metaleft":     input.MetaLeft,
	"metaright":    input.MetaRight,
	"alt":          input.AltLeft,
	"altleft":      input.AltLeft,
	"altright":     input.AltRight,
	"altgraph":     input.AltGraph,
	" ":            input.Space,
	"space":        input.Space,
	"contextmenu":  input.ContextMenu,

	// Center block
	"printscreen": input.PrintScreen,
	"scrolllock":  input.ScrollLock,
	"pause":       input.Pause,
	"pageup":      input.PageUp,
	"pagedown":    input.PageDown,
	"insert":      input.Insert,
	"delete":      input.Delete,
	"home":        input.Home,
	"end":         input.End,
	"arrowleft":   input.ArrowLeft,
	"arrowup":     input.ArrowUp,
	"arrowright":  input.ArrowRight,
	"arrowdown":   input.ArrowDown,

	// Numpad
	"numlock":        input.NumLock,
	"numpaddivide":   input.NumpadDivide,
	"numpadmultiply": input.NumpadMultiply,
	"numpadsubtract": input.NumpadSubtract,
	"numpad7":        input.Numpad7,
	"numpad8":        input.Numpad8,
	"numpad9":        input.Numpad9,
	"numpad4":        input.Numpad4,
	"numpad5":        input.Numpad5,
	"numpad6":        input.Numpad6,
	"numpadadd":      input.NumpadAdd,
	"numpad1":        input.Numpad1,
	"numpad2":        input.Numpad2,
	"numpad3":        input.Numpad3,
	"numpad0":        input.Numpad0,
	"numpaddecimal":  input.NumpadDecimal,
	"numpadenter":    input.NumpadEnter,
}
