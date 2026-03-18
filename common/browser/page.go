package browser

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
)

type BrowserPage struct {
	page    *rod.Page
	browser *BrowserInstance
	refMap  *RefMap
	timeout time.Duration
}

func newBrowserPage(page *rod.Page, browser *BrowserInstance, timeout time.Duration) *BrowserPage {
	return &BrowserPage{
		page:    page,
		browser: browser,
		refMap:  NewRefMap(),
		timeout: timeout,
	}
}

func (p *BrowserPage) Navigate(urlStr string) error {
	timedPage := p.page.Timeout(p.timeout)
	err := timedPage.Navigate(urlStr)
	if err != nil {
		return fmt.Errorf("navigate to %s: %w", urlStr, err)
	}
	err = timedPage.WaitLoad()
	if err != nil {
		if isNonFatalCDPError(err) {
			log.Debugf("non-fatal WaitLoad error (page likely loaded): %v", err)
			return nil
		}
		return fmt.Errorf("wait page load: %w", err)
	}
	return nil
}

func (p *BrowserPage) NavigateAndWait(urlStr string, waitSelector string) error {
	timedPage := p.page.Timeout(p.timeout)
	err := timedPage.Navigate(urlStr)
	if err != nil {
		return fmt.Errorf("navigate to %s: %w", urlStr, err)
	}
	if waitSelector != "" {
		err = timedPage.WaitElementsMoreThan(waitSelector, 0)
	} else {
		err = timedPage.WaitLoad()
	}
	if err != nil {
		if isNonFatalCDPError(err) {
			log.Debugf("non-fatal wait error (page likely loaded): %v", err)
			return nil
		}
		return fmt.Errorf("wait page load: %w", err)
	}
	return nil
}

func isNonFatalCDPError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Object reference chain is too long")
}

func (p *BrowserPage) Reload() error {
	return p.page.Reload()
}

func (p *BrowserPage) Back() error {
	return p.page.NavigateBack()
}

func (p *BrowserPage) Forward() error {
	return p.page.NavigateForward()
}

func (p *BrowserPage) Click(selectorOrRef string) error {
	ref, isRef := ParseRef(selectorOrRef)
	if isRef {
		return p.clickByRef(ref)
	}
	return p.clickBySelector(selectorOrRef)
}

func (p *BrowserPage) Fill(selectorOrRef string, text string) error {
	ref, isRef := ParseRef(selectorOrRef)
	if isRef {
		return p.fillByRef(ref, text)
	}
	return p.fillBySelector(selectorOrRef, text)
}

func (p *BrowserPage) Type(text string) error {
	return p.page.InsertText(text)
}

func (p *BrowserPage) Snapshot() (*SnapshotResult, error) {
	return takeSnapshot(p.page, p.refMap)
}

func (p *BrowserPage) Screenshot() ([]byte, error) {
	return p.page.Screenshot(true, nil)
}

func (p *BrowserPage) ScreenshotBase64() (string, error) {
	bin, err := p.page.Screenshot(true, nil)
	if err != nil {
		return "", fmt.Errorf("take screenshot: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(bin), nil
}

func (p *BrowserPage) Evaluate(js string) (interface{}, error) {
	wrapped := fmt.Sprintf(`() => { return (%s) }`, js)
	result, err := p.page.Eval(wrapped)
	if err != nil {
		return nil, fmt.Errorf("evaluate js: %w", err)
	}
	return result.Value.Val(), nil
}

func (p *BrowserPage) HTML() (string, error) {
	return p.page.HTML()
}

func (p *BrowserPage) Title() (string, error) {
	info, err := p.page.Info()
	if err != nil {
		return "", fmt.Errorf("get page info: %w", err)
	}
	return info.Title, nil
}

func (p *BrowserPage) URL() string {
	info, err := p.page.Info()
	if err != nil {
		return ""
	}
	return info.URL
}

func (p *BrowserPage) WaitSelector(selector string) error {
	timedPage := p.page.Timeout(p.timeout)
	return timedPage.WaitElementsMoreThan(selector, 0)
}

func (p *BrowserPage) WaitVisible(selector string) error {
	timedPage := p.page.Timeout(p.timeout)
	el, err := timedPage.Element(selector)
	if err != nil {
		return fmt.Errorf("find element %s: %w", selector, err)
	}
	return el.WaitVisible()
}

func (p *BrowserPage) Element(selector string) (*BrowserElement, error) {
	el, err := p.page.Element(selector)
	if err != nil {
		return nil, fmt.Errorf("find element %s: %w", selector, err)
	}
	return &BrowserElement{element: el}, nil
}

func (p *BrowserPage) Elements(selector string) (BrowserElements, error) {
	elements, err := p.page.Elements(selector)
	if err != nil {
		return nil, fmt.Errorf("find elements %s: %w", selector, err)
	}
	var result BrowserElements
	for _, el := range elements {
		result = append(result, &BrowserElement{element: el})
	}
	return result, nil
}

func (p *BrowserPage) GetCookies() ([]*proto.NetworkCookie, error) {
	cookies, err := p.page.Cookies(nil)
	if err != nil {
		return nil, fmt.Errorf("get cookies: %w", err)
	}
	return cookies, nil
}

func (p *BrowserPage) SetCookies(cookies []*proto.NetworkCookieParam) error {
	return p.page.SetCookies(cookies)
}

func (p *BrowserPage) Close() error {
	return p.page.Close()
}

func (p *BrowserPage) clickBySelector(selector string) error {
	el, err := p.page.Timeout(p.timeout).Element(selector)
	if err != nil {
		return fmt.Errorf("find element %s for click: %w", selector, err)
	}
	err = el.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("click element %s: %w", selector, err)
	}
	return nil
}

func (p *BrowserPage) fillBySelector(selector string, text string) error {
	el, err := p.page.Timeout(p.timeout).Element(selector)
	if err != nil {
		return fmt.Errorf("find element %s for fill: %w", selector, err)
	}
	err = el.SelectAllText()
	if err != nil {
		log.Debugf("select all text in %s: %v", selector, err)
	}
	return el.Input(text)
}

func (p *BrowserPage) clickByRef(ref string) error {
	entry, ok := p.refMap.Get(ref)
	if !ok {
		return fmt.Errorf("ref %s not found, run Snapshot() first", ref)
	}

	x, y, err := p.resolveElementCenter(entry)
	if err != nil {
		return fmt.Errorf("resolve element center for ref %s: %w", ref, err)
	}

	err = p.dispatchClick(x, y)
	if err != nil {
		return fmt.Errorf("click at (%f, %f) for ref %s: %w", x, y, ref, err)
	}
	return nil
}

func (p *BrowserPage) fillByRef(ref string, text string) error {
	entry, ok := p.refMap.Get(ref)
	if !ok {
		return fmt.Errorf("ref %s not found, run Snapshot() first", ref)
	}

	nodeID := proto.DOMBackendNodeID(entry.BackendNodeID)
	resolveResult, err := proto.DOMResolveNode{BackendNodeID: nodeID}.Call(p.page)
	if err != nil {
		return fmt.Errorf("resolve node for ref %s: %w", ref, err)
	}

	_, err = proto.RuntimeCallFunctionOn{
		ObjectID:            resolveResult.Object.ObjectID,
		FunctionDeclaration: `function() { this.focus(); this.value = ''; }`,
	}.Call(p.page)
	if err != nil {
		log.Debugf("focus and clear for ref %s: %v", ref, err)
	}

	return p.page.InsertText(text)
}

func (p *BrowserPage) resolveElementCenter(entry *RefEntry) (float64, float64, error) {
	nodeID := proto.DOMBackendNodeID(entry.BackendNodeID)

	box, err := proto.DOMGetBoxModel{BackendNodeID: nodeID}.Call(p.page)
	if err != nil {
		return 0, 0, fmt.Errorf("get box model: %w", err)
	}

	if box.Model == nil || len(box.Model.Content) < 8 {
		return 0, 0, fmt.Errorf("invalid box model content")
	}

	quad := box.Model.Content
	x := (quad[0] + quad[2] + quad[4] + quad[6]) / 4
	y := (quad[1] + quad[3] + quad[5] + quad[7]) / 4

	if math.IsNaN(x) || math.IsNaN(y) {
		return 0, 0, fmt.Errorf("computed center is NaN")
	}

	return x, y, nil
}

func (p *BrowserPage) dispatchClick(x, y float64) error {
	err := proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMousePressed,
		X:          x,
		Y:          y,
		Button:     proto.InputMouseButtonLeft,
		ClickCount: 1,
	}.Call(p.page)
	if err != nil {
		return err
	}

	return proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMouseReleased,
		X:          x,
		Y:          y,
		Button:     proto.InputMouseButtonLeft,
		ClickCount: 1,
	}.Call(p.page)
}
