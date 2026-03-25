package browser

import (
	"fmt"
	"regexp"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

type BrowserElement struct {
	element *rod.Element
}

type BrowserElements []*BrowserElement

// typeableKeyStringRe matches keyboard-typeable text: common controls (tab, CR, LF, BS, ESC)
// plus printable ASCII (\x20–\x7E).
var typeableKeyStringRe = regexp.MustCompile("^[\t\r\n\x08\x1b\x20-\x7E]*$")

// runeToInputKey maps a rune to rod's Key. Validation is done by typeableKeyStringRe; a few
// control codes use named keys because their Key() is not the raw rune in go-rod's layout.
func runeToInputKey(r rune) input.Key {
	switch r {
	case '\b':
		return input.Backspace
	case '\n':
		return input.Enter
	case '\x1b':
		return input.Escape
	default:
		return input.Key(r)
	}
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
	if !typeableKeyStringRe.MatchString(keys) {
		return fmt.Errorf("invalid keys: must only contain keyboard-typeable characters (printable ASCII, tab, CR, LF, BS, ESC)")
	}
	typedKeys := make([]input.Key, 0, len(keys))
	for _, r := range keys {
		typedKeys = append(typedKeys, runeToInputKey(r))
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
