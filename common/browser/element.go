package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type BrowserElement struct {
	element *rod.Element
}

type BrowserElements []*BrowserElement

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

func (e *BrowserElement) Click() error {
	return e.element.Click(proto.InputMouseButtonLeft, 1)
}

func (e *BrowserElement) Input(text string) error {
	return e.element.Input(text)
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
