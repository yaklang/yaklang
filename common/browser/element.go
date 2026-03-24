package browser

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

type BrowserElement struct {
	element *rod.Element
}

type BrowserElements []*BrowserElement

// localTypeKeyMap keeps the string -> input.Key mapping from rod input keymap.
var localTypeKeyMap = map[string]input.Key{
	// Functions row.
	"Escape": input.Escape,
	"F1":     input.F1,
	"F2":     input.F2,
	"F3":     input.F3,
	"F4":     input.F4,
	"F5":     input.F5,
	"F6":     input.F6,
	"F7":     input.F7,
	"F8":     input.F8,
	"F9":     input.F9,
	"F10":    input.F10,
	"F11":    input.F11,
	"F12":    input.F12,

	// Numbers row.
	"`":         input.Backquote,
	"~":         input.Key('~'),
	"1":         input.Digit1,
	"!":         input.Key('!'),
	"2":         input.Digit2,
	"@":         input.Key('@'),
	"3":         input.Digit3,
	"#":         input.Key('#'),
	"4":         input.Digit4,
	"$":         input.Key('$'),
	"5":         input.Digit5,
	"%":         input.Key('%'),
	"6":         input.Digit6,
	"^":         input.Key('^'),
	"7":         input.Digit7,
	"&":         input.Key('&'),
	"8":         input.Digit8,
	"*":         input.Key('*'),
	"9":         input.Digit9,
	"(":         input.Key('('),
	"0":         input.Digit0,
	")":         input.Key(')'),
	"-":         input.Minus,
	"_":         input.Key('_'),
	"=":         input.Equal,
	"+":         input.Key('+'),
	"\\":        input.Backslash,
	"|":         input.Key('|'),
	"Backspace": input.Backspace,

	// First row.
	"\t":  input.Tab,
	"Tab": input.Tab,
	"q":   input.KeyQ,
	"Q":   input.Key('Q'),
	"w":   input.KeyW,
	"W":   input.Key('W'),
	"e":   input.KeyE,
	"E":   input.Key('E'),
	"r":   input.KeyR,
	"R":   input.Key('R'),
	"t":   input.KeyT,
	"T":   input.Key('T'),
	"y":   input.KeyY,
	"Y":   input.Key('Y'),
	"u":   input.KeyU,
	"U":   input.Key('U'),
	"i":   input.KeyI,
	"I":   input.Key('I'),
	"o":   input.KeyO,
	"O":   input.Key('O'),
	"p":   input.KeyP,
	"P":   input.Key('P'),
	"[":   input.BracketLeft,
	"{":   input.Key('{'),
	"]":   input.BracketRight,
	"}":   input.Key('}'),

	// Second row.
	"CapsLock": input.CapsLock,
	"a":        input.KeyA,
	"A":        input.Key('A'),
	"s":        input.KeyS,
	"S":        input.Key('S'),
	"d":        input.KeyD,
	"D":        input.Key('D'),
	"f":        input.KeyF,
	"F":        input.Key('F'),
	"g":        input.KeyG,
	"G":        input.Key('G'),
	"h":        input.KeyH,
	"H":        input.Key('H'),
	"j":        input.KeyJ,
	"J":        input.Key('J'),
	"k":        input.KeyK,
	"K":        input.Key('K'),
	"l":        input.KeyL,
	"L":        input.Key('L'),
	";":        input.Semicolon,
	":":        input.Key(':'),
	"'":        input.Quote,
	"\"":       input.Key('"'),
	"\r":       input.Enter,
	"Enter":    input.Enter,

	// Third row.
	"Shift": input.ShiftLeft,
	"z":     input.KeyZ,
	"Z":     input.Key('Z'),
	"x":     input.KeyX,
	"X":     input.Key('X'),
	"c":     input.KeyC,
	"C":     input.Key('C'),
	"v":     input.KeyV,
	"V":     input.Key('V'),
	"b":     input.KeyB,
	"B":     input.Key('B'),
	"n":     input.KeyN,
	"N":     input.Key('N'),
	"m":     input.KeyM,
	"M":     input.Key('M'),
	",":     input.Comma,
	"<":     input.Key('<'),
	".":     input.Period,
	">":     input.Key('>'),
	"/":     input.Slash,
	"?":     input.Key('?'),

	// Last row.
	"Control":     input.ControlLeft,
	"Meta":        input.MetaLeft,
	"Alt":         input.AltLeft,
	" ":           input.Space,
	"Space":       input.Space,
	"AltGraph":    input.AltGraph,
	"ContextMenu": input.ContextMenu,

	// Center block.
	"PrintScreen": input.PrintScreen,
	"ScrollLock":  input.ScrollLock,
	"Pause":       input.Pause,
	"PageUp":      input.PageUp,
	"PageDown":    input.PageDown,
	"Insert":      input.Insert,
	"Delete":      input.Delete,
	"Home":        input.Home,
	"End":         input.End,
	"ArrowLeft":   input.ArrowLeft,
	"ArrowUp":     input.ArrowUp,
	"ArrowRight":  input.ArrowRight,
	"ArrowDown":   input.ArrowDown,

	// Numpad.
	"NumLock": input.NumLock,
}

func (e *BrowserElement) Text() (string, error) {
	return e.element.Text()
}

func (e *BrowserElement) HTML() (string, error) {
	return e.element.HTML()
}

func (e *BrowserElement) Attribute(name string) (string, error) {
	value, err := e.element.Attribute(name)
	if err != nil {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	return *value, nil
}

// Property get property result of element like: (n) => this[n]
func (e *BrowserElement) Property(name string) (interface{}, error) {
	value, err := e.element.Property(name)
	if err != nil {
		return "", err
	}
	return value.Val(), nil
}

func (e *BrowserElement) Click() error {
	return e.element.Click(proto.InputMouseButtonLeft, 1)
}

func (e *BrowserElement) DoubleClick() error { return e.element.Click(proto.InputMouseButtonLeft, 2) }

func (e *BrowserElement) Input(text string) error {
	return e.element.Input(text)
}

func (e *BrowserElement) Type(keys string) error {
	chars := []rune(keys)
	typedKeys := make([]input.Key, 0, len(chars))
	for _, char := range chars {
		key := string(char)
		mapped, ok := localTypeKeyMap[key]
		if !ok {
			return fmt.Errorf("invalid key: %q", key)
		}
		typedKeys = append(typedKeys, mapped)
	}
	return e.element.Type(typedKeys...)
}

func (e *BrowserElement) Hover() error { return e.element.Hover() }

func (e *BrowserElement) Select(val string) error {
	return e.element.Select([]string{val}, true, rod.SelectorTypeText)
}

func (e *BrowserElement) ScrollIntoView() error {
	return e.element.ScrollIntoView()
}

func (e *BrowserElement) SetFiles(filePath string) error {
	return e.element.SetFiles([]string{filePath})
}

func (e *BrowserElement) Evaluate(js string) (interface{}, error) {
	wrapped := fmt.Sprintf(`() => { return (%s) }`, js)
	result, err := e.element.Eval(wrapped)
	if err != nil {
		return nil, fmt.Errorf("element evaluate js: %w", err)
	}
	return result.Value.Val(), nil
}

func (e *BrowserElement) Focus() error {
	return e.element.Focus()
}

func (e *BrowserElement) Visible() (bool, error) {
	visible, err := e.element.Visible()
	if err != nil {
		return false, err
	}
	return visible, nil
}

func (e *BrowserElement) WaitVisible() error {
	return e.element.WaitVisible()
}

// Enable Check if enabled
func (e *BrowserElement) Enable() (bool, error) {
	disabled, err := e.element.Disabled()
	if err != nil {
		return false, err
	}
	return !disabled, nil
}

// Checked Check if checked
func (e *BrowserElement) Checked() (bool, error) {
	checked, err := e.element.Property("checked")
	if err != nil {
		return false, err
	}
	if checked.Nil() {
		return false, nil
	}
	return checked.Bool(), nil
}

// GetCenterPosition get element center position
func (e *BrowserElement) GetCenterPosition() (float64, float64, error) {
	shape, err := e.element.Shape()
	if err != nil {
		return 0, 0, fmt.Errorf("get element shape: %w", err)
	}

	// OnePointInside picks the first polygon with non-trivial area
	// and returns the average(center) of its vertices.
	pt := shape.OnePointInside()
	if pt == nil {
		return 0, 0, fmt.Errorf("empty element shape")
	}
	if pt.X != pt.X || pt.Y != pt.Y { // NaN check without importing math
		return 0, 0, fmt.Errorf("computed center is NaN")
	}
	return pt.X, pt.Y, nil
}
