// Package browser
// @Author bcy2007  2026/3/23 11:59
package browser

import (
	"github.com/go-rod/rod/lib/proto"
	"time"
)

// mouse in page

// Move mouse move to target position
func (p *BrowserPage) Move(x, y float64) error {
	return p.mouse.MoveTo(proto.Point{X: x, Y: y})
}

func (p *BrowserPage) Down() error {
	return p.mouse.Down(proto.InputMouseButtonLeft, 1)
}

func (p *BrowserPage) Up() error {
	return p.mouse.Up(proto.InputMouseButtonLeft, 1)
}

func (p *BrowserPage) Drag(fromX, fromY, toX, toY float64) error {
	err := p.Move(fromX, fromY)
	if err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	err = p.Down()
	if err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	err = p.Move(toX, toY)
	if err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	return p.Up()
}
