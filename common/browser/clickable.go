package browser

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
)

const markClickableJS = `(function(){
  var marker = "data-yaklang-clickable-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2);
  function visible(el) {
	var st = window.getComputedStyle(el);
	if (!st || st.display === "none" || st.visibility === "hidden" || st.pointerEvents === "none") {
	  return false;
	}
	if (el.disabled || (el.getAttribute("aria-disabled") || "").toLowerCase() === "true") {
	  return false;
	}
    var box = el.getBoundingClientRect();
    return box.width >= 4 && box.height >= 4;
  }
  function isClickable(el) {
    var st = window.getComputedStyle(el);
    var role = (el.getAttribute("role") || "").toLowerCase();
    var tab = el.getAttribute("tabindex");
    var ce = (el.getAttribute("contenteditable") || "").toLowerCase();
    var type = ((el.type || "") + "").toLowerCase();
    return el.tagName === "A" || el.tagName === "BUTTON" || el.tagName === "SELECT" ||
      !!el.getAttribute("onclick") ||
      role === "button" || role === "menuitem" || role === "tab" || role === "treeitem" || role === "option" ||
      (st && st.cursor === "pointer") || tab === "0" ||
      ce === "true" || (el.hasAttribute("contenteditable") && ce !== "false") ||
      (el.tagName === "INPUT" && (type === "file" || type === "button" || type === "submit"));
  }
  function intentScore(el) {
    var t = ((el.innerText || el.textContent || el.getAttribute("aria-label") || el.getAttribute("title") || el.getAttribute("value") || el.getAttribute("placeholder") || "") + "").toLowerCase();
    t = t.replace(/\s+/g, " ");
    if (/注销|退出|logout|signout|删除|清空|批量|delete|remove|destroy/.test(t)) {
      return 0;
    }
    if (/保存|提交|确定|save|submit|confirm/.test(t) || /(^|\s)ok(\s|$)/.test(t)) {
      return 4;
    }
    if (/新增|添加|创建|新建|create|\badd\b|\bnew\b/.test(t)) {
      return 3;
    }
    if (/查询|搜索|筛选|search|filter|query/.test(t)) {
      return 3;
    }
    if (/更多|操作|\bmore\b/.test(t)) {
      return 2;
    }
    if (/下一页|\bnext\b/.test(t)) {
      return 2;
    }
    if (/上传|选择文件|upload|choose file/.test(t)) {
      return 2;
    }
    if (/展开|expand/.test(t)) {
      return 2;
    }
    if (/编辑|修改|\bedit\b|modify/.test(t)) {
      return 2;
    }
    return 1;
  }
  function nodeName(el) {
    var t = ((el.innerText || el.textContent || el.getAttribute("aria-label") || el.getAttribute("title") || el.getAttribute("value") || el.getAttribute("placeholder") || "") + "");
    t = t.replace(/\s+/g, " ").trim();
    if (t.length > 40) {
      t = t.slice(0, 40);
    }
    return t;
  }
  var ranked = [];
  var candidateSelector = "a,button,[role=button],[role=menuitem],[role=tab],[role=treeitem],[role=option],[onclick],li,div,span,td,input,select,[contenteditable=true],[contenteditable='']";
  function walk(root, inShadow) {
    if (!root) {
      return;
    }
	var all;
	try {
	  all = root.querySelectorAll("*");
    } catch (e) {
      return;
    }
	for (var i = 0; i < all.length; i++) {
	  var el = all[i];
	  if (el.matches(candidateSelector) && visible(el) && isClickable(el)) {
        ranked.push({el: el, shadow: inShadow});
      }
      try {
        if (el.shadowRoot) {
          walk(el.shadowRoot, true);
        }
      } catch (e2) {}
    }
  }
  walk(document, false);
  ranked.sort(function(a, b) { return intentScore(b.el) - intentScore(a.el); });
  var n = 0;
  var extras = [];
  var shadowElements = [];
  for (var j = 0; j < ranked.length && n < 40; j++) {
    var item = ranked[j];
    if (item.shadow) {
      var box = item.el.getBoundingClientRect();
		extras.push({
		  name: nodeName(item.el),
		  role: ((item.el.getAttribute("role") || item.el.tagName || "clickable") + "").toLowerCase(),
		  x: box.left + box.width / 2,
		  y: box.top + box.height / 2,
		  index: shadowElements.push(item.el) - 1,
		  shadow: true
      });
      n++;
      continue;
    }
	item.el.setAttribute(marker, String(n));
	n++;
  }
  window[marker] = shadowElements;
  return JSON.stringify({marker: marker, n: n, extras: extras});
})()`

const clickableMarkerPrefix = "data-yaklang-clickable-"

type markClickableExtra struct {
	Name   string  `json:"name"`
	Role   string  `json:"role"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Index  int     `json:"index"`
	Shadow bool    `json:"shadow"`
}

type markClickableResult struct {
	Marker string               `json:"marker"`
	Extras []markClickableExtra `json:"extras"`
}

func parseMarkClickableResult(raw any) markClickableResult {
	s := ""
	switch v := raw.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return markClickableResult{}
	}
	if strings.TrimSpace(s) == "" {
		return markClickableResult{}
	}
	var parsed markClickableResult
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return markClickableResult{}
	}
	if !validClickableMarker(parsed.Marker) {
		parsed.Marker = ""
	}
	return parsed
}

func validClickableMarker(marker string) bool {
	if !strings.HasPrefix(marker, clickableMarkerPrefix) || len(marker) == len(clickableMarkerPrefix) {
		return false
	}
	for _, r := range marker {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

type markedClickable struct {
	element *rod.Element
	rank    int
}

func clickableMarkerCleanupJS(marker string) string {
	quoted, _ := json.Marshal(marker)
	return fmt.Sprintf(`(function(){var a=%s;document.querySelectorAll("["+a+"]").forEach(function(el){el.removeAttribute(a);});delete window[a];return 0;})()`, quoted)
}

func (p *BrowserPage) resolveShadowClickable(marker string, extra markClickableExtra) int {
	if p == nil || p.page == nil || !extra.Shadow || extra.Index < 0 {
		return 0
	}
	el, err := p.page.ElementByJS(rod.Eval(`(key, index) => window[key] && window[key][index]`, marker, extra.Index))
	if err != nil || el == nil {
		return 0
	}
	node, err := el.Describe(0, false)
	if err != nil || node == nil {
		return 0
	}
	return int(node.BackendNodeID)
}

// ListClickable snapshots interactive AX nodes then adds pointer/click DOM
// nodes that lack an ARIA interactive role. Open shadow roots are nominated
// via coordinates because light-DOM queries cannot pierce them.
func (p *BrowserPage) ListClickable() []map[string]any {
	if p == nil || p.page == nil {
		return []map[string]any{}
	}
	items := p.ListInteractive()
	seen := map[int]struct{}{}
	if p.refMap != nil {
		n := p.refMap.Count()
		for i := 1; i <= n; i++ {
			entry, ok := p.refMap.Get(fmt.Sprintf("e%d", i))
			if ok && entry != nil && entry.BackendNodeID > 0 {
				seen[entry.BackendNodeID] = struct{}{}
			}
		}
	}
	raw, err := p.Evaluate(markClickableJS)
	if err != nil {
		log.Debugf("ListClickable mark: %v", err)
		return items
	}
	marked := parseMarkClickableResult(raw)
	if marked.Marker == "" {
		log.Debugf("ListClickable mark returned an invalid marker")
		return items
	}
	defer func() {
		_, _ = p.Evaluate(clickableMarkerCleanupJS(marked.Marker))
	}()
	els, err := p.page.Elements("[" + marked.Marker + "]")
	if err != nil {
		log.Debugf("ListClickable query: %v", err)
		return items
	}
	ranked := make([]markedClickable, 0, len(els))
	for _, el := range els {
		if el == nil {
			continue
		}
		rank := len(ranked)
		if attr, attrErr := el.Attribute(marked.Marker); attrErr == nil && attr != nil {
			if n, convErr := strconv.Atoi(*attr); convErr == nil {
				rank = n
			}
		}
		ranked = append(ranked, markedClickable{element: el, rank: rank})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].rank < ranked[j].rank })
	for _, candidate := range ranked {
		el := candidate.element
		if el == nil {
			continue
		}
		node, descErr := el.Describe(0, false)
		if descErr != nil || node == nil || node.BackendNodeID == 0 {
			continue
		}
		backendID := int(node.BackendNodeID)
		if _, ok := seen[backendID]; ok {
			continue
		}
		name := ""
		if text, textErr := el.Text(); textErr == nil {
			name = strings.Join(strings.Fields(text), " ")
			name = clipRunes(name, 40)
		}
		role := "clickable"
		if p.refMap == nil {
			p.refMap = NewRefMap()
		}
		ref := p.refMap.Assign(&RefEntry{
			BackendNodeID: backendID,
			Role:          role,
			Name:          name,
		})
		seen[backendID] = struct{}{}
		items = append(items, map[string]any{
			"ref":  "@" + ref,
			"role": role,
			"name": name,
		})
		if len(items) >= 60 {
			break
		}
	}
	if len(items) < 60 {
		for _, extra := range marked.Extras {
			backendID := p.resolveShadowClickable(marked.Marker, extra)
			if backendID > 0 {
				if _, ok := seen[backendID]; ok {
					continue
				}
				seen[backendID] = struct{}{}
			}
			if p.refMap == nil {
				p.refMap = NewRefMap()
			}
			role := extra.Role
			if role == "" {
				role = "clickable"
			}
			ref := p.refMap.Assign(&RefEntry{
				BackendNodeID: backendID,
				Role:          role,
				Name:          extra.Name,
				CoordX:        extra.X,
				CoordY:        extra.Y,
			})
			items = append(items, map[string]any{
				"ref":  "@" + ref,
				"role": role,
				"name": extra.Name,
			})
			if len(items) >= 60 {
				break
			}
		}
	}
	return items
}
