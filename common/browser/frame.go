package browser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
)

const listFramesJS = `(function(){
  var origin = location.origin;
  var frames = [];
  var els = document.querySelectorAll("iframe");
  for (var i = 0; i < els.length; i++) {
    var el = els[i];
    var st = window.getComputedStyle(el);
    var box = el.getBoundingClientRect();
    var abs = "";
    try { abs = el.src || ""; } catch (e) { abs = el.getAttribute("src") || ""; }
    abs = (abs || "").trim();
    if (!abs) {
      continue;
    }
    var visible = !!(st && st.display !== "none" && st.visibility !== "hidden" && box.width >= 4 && box.height >= 4);
    if (!visible) {
      continue;
    }
    var host = "";
    var same = false;
    try {
      var u = new URL(abs, location.href);
      host = u.host;
      same = u.origin === origin;
    } catch (e2) {}
    frames.push({
      index: frames.length,
      src: abs,
      url: abs,
      host: host,
      same_origin: same,
      visible: true,
      area: Math.round(Math.max(0, box.width) * Math.max(0, box.height))
    });
  }
  return JSON.stringify(frames);
})()`

type frameInfo struct {
	Index      int     `json:"index"`
	Src        string  `json:"src"`
	URL        string  `json:"url"`
	Host       string  `json:"host"`
	SameOrigin bool    `json:"same_origin"`
	Visible    bool    `json:"visible"`
	Area       float64 `json:"area"`
}

func parseFrameArg(v any) (idx int, src string, hasIdx bool) {
	switch t := v.(type) {
	case nil:
		return 0, "", false
	case int:
		return t, "", true
	case int8:
		return int(t), "", true
	case int16:
		return int(t), "", true
	case int32:
		return int(t), "", true
	case int64:
		return int(t), "", true
	case uint:
		return int(t), "", true
	case uint8:
		return int(t), "", true
	case uint16:
		return int(t), "", true
	case uint32:
		return int(t), "", true
	case uint64:
		return int(t), "", true
	case float32:
		return int(t), "", true
	case float64:
		return int(t), "", true
	case json.Number:
		n, err := t.Int64()
		if err == nil {
			return int(n), "", true
		}
		return 0, t.String(), false
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, "", false
		}
		if n, err := strconv.Atoi(s); err == nil {
			return n, "", true
		}
		return 0, s, false
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		if s == "" || s == "<nil>" {
			return 0, "", false
		}
		if n, err := strconv.Atoi(s); err == nil {
			return n, "", true
		}
		return 0, s, false
	}
}

func (p *BrowserPage) evalOn(page *rod.Page, js string) (any, error) {
	if p == nil || page == nil {
		return nil, fmt.Errorf("empty page")
	}
	if err := p.requireNoDialog("evaluate js"); err != nil {
		return nil, err
	}
	wrapped := fmt.Sprintf(`() => { return (%s) }`, js)
	result, err := page.Timeout(p.timeout).Eval(wrapped)
	if err != nil {
		if d, ok := p.GetPendingDialog(); ok {
			return nil, dialogBlockingError(d, err)
		}
		return nil, fmt.Errorf("evaluate js: %w", err)
	}
	return result.Value.Val(), nil
}

func parseFrameList(raw any) []frameInfo {
	if raw == nil {
		return nil
	}
	var b []byte
	switch v := raw.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		enc, err := json.Marshal(v)
		if err != nil {
			log.Debugf("ListFrames marshal: %v", err)
			return nil
		}
		b = enc
	}
	s := strings.TrimSpace(string(b))
	if s == "" || s == "<nil>" || s == "null" {
		return nil
	}
	var frames []frameInfo
	if err := json.Unmarshal(b, &frames); err != nil {
		log.Infof("ListFrames parse: %v raw=%s", err, truncateRunes(s, 200))
		return nil
	}
	return frames
}

func truncateRunes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func frameToMap(f frameInfo) map[string]any {
	return map[string]any{
		"index":       f.Index,
		"src":         f.Src,
		"url":         firstNonEmpty(f.URL, f.Src),
		"host":        f.Host,
		"same_origin": f.SameOrigin,
		"visible":     f.Visible,
		"area":        f.Area,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func urlsMatchFrame(want, got string) bool {
	want = strings.TrimSpace(want)
	got = strings.TrimSpace(got)
	if want == "" || got == "" {
		return false
	}
	if want == got {
		return true
	}
	return strings.Contains(got, want) || strings.Contains(want, got)
}

func pickFrame(frames []frameInfo, urlOrIndex any) (frameInfo, bool) {
	if len(frames) == 0 {
		return frameInfo{}, false
	}
	idx, src, hasIdx := parseFrameArg(urlOrIndex)
	if src == "" && !hasIdx {
		best := -1
		bestArea := -1.0
		for i, f := range frames {
			if !f.SameOrigin {
				continue
			}
			if f.Area > bestArea {
				bestArea = f.Area
				best = i
			}
		}
		if best >= 0 {
			return frames[best], true
		}
		return frameInfo{}, false
	}
	if hasIdx {
		if idx < 0 || idx >= len(frames) {
			return frameInfo{}, false
		}
		return frames[idx], true
	}
	for _, f := range frames {
		if urlsMatchFrame(src, f.Src) || urlsMatchFrame(src, f.URL) {
			return f, true
		}
	}
	return frameInfo{}, false
}

func elementSrc(el *rod.Element) string {
	if el == nil {
		return ""
	}
	if v, err := el.Property("src"); err == nil {
		s := strings.TrimSpace(v.Str())
		if s == "" {
			s = strings.Trim(strings.TrimSpace(v.String()), `"`)
		}
		if s != "" {
			return s
		}
	}
	if v, err := el.Attribute("src"); err == nil && v != nil {
		return strings.TrimSpace(*v)
	}
	return ""
}

func elementArea(el *rod.Element) float64 {
	if el == nil {
		return 0
	}
	shape, err := el.Shape()
	if err != nil || shape == nil || len(shape.Quads) == 0 || len(shape.Quads[0]) < 8 {
		return 0
	}
	q := shape.Quads[0]
	minX, maxX := q[0], q[0]
	minY, maxY := q[1], q[1]
	for i := 0; i < len(q); i += 2 {
		if q[i] < minX {
			minX = q[i]
		}
		if q[i] > maxX {
			maxX = q[i]
		}
		if q[i+1] < minY {
			minY = q[i+1]
		}
		if q[i+1] > maxY {
			maxY = q[i+1]
		}
	}
	w := maxX - minX
	h := maxY - minY
	if w < 0 || h < 0 {
		return 0
	}
	return w * h
}

func collectFrames(root *rod.Page, timeout time.Duration, rootURL string) []frameInfo {
	if root == nil {
		return nil
	}
	els, err := root.Timeout(timeout).Elements("iframe")
	if err != nil {
		return nil
	}
	rootOrigin := originOf(rootURL)
	var frames []frameInfo
	for _, el := range els {
		if el == nil {
			continue
		}
		src := elementSrc(el)
		if src == "" {
			continue
		}
		if u, err := url.Parse(src); err == nil && !u.IsAbs() {
			if base, berr := url.Parse(rootURL); berr == nil {
				src = base.ResolveReference(u).String()
			}
		}
		vis := true
		if ok, verr := el.Visible(); verr == nil {
			vis = ok
		}
		if !vis {
			continue
		}
		host := ""
		same := false
		if u, err := url.Parse(src); err == nil {
			host = u.Host
			same = originOf(src) != "" && originOf(src) == rootOrigin
		}
		frames = append(frames, frameInfo{
			Index:      len(frames),
			Src:        src,
			URL:        src,
			Host:       host,
			SameOrigin: same,
			Visible:    vis,
			Area:       elementArea(el),
		})
	}
	return frames
}

func (p *BrowserPage) listFrameInfos() []frameInfo {
	root := p.rootRod()
	if root == nil {
		return nil
	}
	rootURL := ""
	if info, err := root.Info(); err == nil && info != nil {
		rootURL = info.URL
	}
	frames := collectFrames(root, p.timeout, rootURL)
	if len(frames) > 0 {
		return frames
	}
	raw, err := p.evalOn(root, listFramesJS)
	if err != nil {
		log.Debugf("ListFrames: %v", err)
		return nil
	}
	return parseFrameList(raw)
}

// ListFrames returns visible iframes with a non-empty src. Cross-origin
// frames are included with same_origin=false and must not be entered.
func (p *BrowserPage) ListFrames() []map[string]any {
	if p == nil {
		return []map[string]any{}
	}
	frames := p.listFrameInfos()
	out := make([]map[string]any, 0, len(frames))
	for _, f := range frames {
		out = append(out, frameToMap(f))
	}
	return out
}

// UseFrame switches Snapshot/ListClickable/Click/Fill onto a same-origin iframe.
// urlOrIndex may be a list index, a src/url substring, or empty to pick the
// largest visible same-origin frame.
func (p *BrowserPage) UseFrame(urlOrIndex any) error {
	if p == nil {
		return fmt.Errorf("empty page")
	}
	root := p.rootRod()
	if root == nil {
		return fmt.Errorf("empty page")
	}
	frames := p.listFrameInfos()
	chosen, ok := pickFrame(frames, urlOrIndex)
	if !ok {
		return fmt.Errorf("frame not found")
	}
	if !chosen.SameOrigin {
		return fmt.Errorf("refuse cross-origin frame host=%s", chosen.Host)
	}
	els, err := root.Timeout(p.timeout).Elements("iframe")
	if err != nil {
		return fmt.Errorf("find iframe: %w", err)
	}
	var target *rod.Element
	for _, el := range els {
		if el == nil {
			continue
		}
		src, srcErr := el.Property("src")
		if srcErr != nil {
			continue
		}
		got := strings.TrimSpace(src.String())
		if urlsMatchFrame(chosen.Src, got) || urlsMatchFrame(chosen.URL, got) {
			target = el
			break
		}
	}
	if target == nil && chosen.Index >= 0 && chosen.Index < len(els) {
		target = els[chosen.Index]
	}
	if target == nil {
		return fmt.Errorf("iframe element not found for %s", chosen.Src)
	}
	framePage, err := target.Frame()
	if err != nil {
		return fmt.Errorf("enter frame: %w", err)
	}
	rootOrigin := ""
	if info, infoErr := root.Info(); infoErr == nil && info != nil {
		rootOrigin = originOf(info.URL)
	}
	frameURL := ""
	if info, infoErr := framePage.Info(); infoErr == nil && info != nil {
		frameURL = info.URL
	}
	if rootOrigin != "" && frameURL != "" && originOf(frameURL) != "" && originOf(frameURL) != rootOrigin {
		return fmt.Errorf("refuse cross-origin frame host=%s", chosen.Host)
	}
	p.page = framePage
	p.mouse = framePage.Mouse
	if p.refMap != nil {
		p.refMap.Reset()
	}
	return nil
}

// UseMainFrame restores Snapshot/ListClickable/Click/Fill to the top document.
func (p *BrowserPage) UseMainFrame() error {
	if p == nil || p.rootRod() == nil {
		return fmt.Errorf("empty page")
	}
	p.useRootSession()
	if p.refMap != nil {
		p.refMap.Reset()
	}
	return nil
}
