package browser

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

const runtimeDiagnosticsBootstrapJS = `(function () {
  if (window.__yakRuntimeDiagnostics) return;
  var state = { consoleErrors: [], errors: [], unhandledRejections: [] };
  Object.defineProperty(window, "__yakRuntimeDiagnostics", {
    value: state,
    configurable: true
  });
  function valueText(value) {
    if (value instanceof Error) return value.stack || value.message || String(value);
    if (typeof value === "string") return value;
    try { return JSON.stringify(value); } catch (_) { return String(value); }
  }
  function push(list, value) {
    if (list.length >= 50) list.shift();
    list.push(valueText(value).slice(0, 2000));
  }
  window.addEventListener("error", function (event) {
    push(state.errors, {
      message: event.message || "window error",
      source: event.filename || "",
      line: event.lineno || 0,
      column: event.colno || 0,
      stack: event.error && event.error.stack ? event.error.stack : ""
    });
  });
  window.addEventListener("unhandledrejection", function (event) {
    push(state.unhandledRejections, valueText(event.reason));
  });
  var originalConsoleError = console.error.bind(console);
  console.error = function () {
    var parts = [];
    for (var i = 0; i < arguments.length; i++) parts.push(valueText(arguments[i]));
    push(state.consoleErrors, parts.join(" "));
    return originalConsoleError.apply(console, arguments);
  };
})();`

type ViewportOverflowElement struct {
	Tag      string  `json:"tag"`
	ID       string  `json:"id,omitempty"`
	Class    string  `json:"class,omitempty"`
	Selector string  `json:"selector"`
	Left     float64 `json:"left"`
	Right    float64 `json:"right"`
	Width    float64 `json:"width"`
}

type ViewportDiagnostics struct {
	InnerWidth               int                       `json:"inner_width"`
	InnerHeight              int                       `json:"inner_height"`
	ClientWidth              int                       `json:"client_width"`
	ClientHeight             int                       `json:"client_height"`
	ScrollWidth              int                       `json:"scroll_width"`
	ScrollHeight             int                       `json:"scroll_height"`
	BodyScrollWidth          int                       `json:"body_scroll_width"`
	DevicePixelRatio         float64                   `json:"device_pixel_ratio"`
	OverflowX                bool                      `json:"overflow_x"`
	HorizontalOverflowPixels int                       `json:"horizontal_overflow_pixels"`
	VisibleElements          int                       `json:"visible_elements"`
	OverflowElements         []ViewportOverflowElement `json:"overflow_elements"`
	ConsoleErrors            []string                  `json:"console_errors"`
	RuntimeErrors            []string                  `json:"runtime_errors"`
	UnhandledRejections      []string                  `json:"unhandled_rejections"`
}

// InstallRuntimeDiagnostics records console.error, window errors and unhandled
// promise rejections before each new document is evaluated.
func (p *BrowserPage) InstallRuntimeDiagnostics() error {
	if p == nil {
		return fmt.Errorf("install runtime diagnostics: empty page")
	}
	return p.EvalOnNewDocument(runtimeDiagnosticsBootstrapJS)
}

// ViewportDiagnostics returns the actual viewport after CSS/media-query
// evaluation, horizontal overflow evidence, visible-element count and captured
// runtime errors. It is intentionally based on live browser metrics rather than
// the requested desktop-window size.
func (p *BrowserPage) ViewportDiagnostics() (*ViewportDiagnostics, error) {
	if p == nil {
		return nil, fmt.Errorf("viewport diagnostics: empty page")
	}
	const js = `JSON.stringify((function () {
	  var doc = document.documentElement;
	  var body = document.body;
	  var viewportWidth = doc ? doc.clientWidth : window.innerWidth;
	  var elements = Array.from(document.querySelectorAll("body *"));
	  var visible = 0;
	  var offenders = [];
	  for (var i = 0; i < elements.length; i++) {
	    var el = elements[i];
	    var rect = el.getBoundingClientRect();
	    var style = window.getComputedStyle(el);
	    if (rect.width > 0 && rect.height > 0 && style.display !== "none" && style.visibility !== "hidden") {
	      visible++;
	    }
	    if (rect.right > viewportWidth + 1 && offenders.length < 20) {
	      offenders.push({
	        tag: el.tagName ? el.tagName.toLowerCase() : "",
	        id: el.id || "",
	        class: typeof el.className === "string" ? el.className.slice(0, 200) : "",
	        selector: el.id ? "#" + el.id : (el.tagName ? el.tagName.toLowerCase() : ""),
	        left: Math.round(rect.left * 100) / 100,
	        right: Math.round(rect.right * 100) / 100,
	        width: Math.round(rect.width * 100) / 100
	      });
	    }
	  }
	  var scrollWidth = Math.max(doc ? doc.scrollWidth : 0, body ? body.scrollWidth : 0);
	  var diagnostics = window.__yakRuntimeDiagnostics || {};
	  var runtimeErrors = (diagnostics.errors || []).map(function (item) {
	    return typeof item === "string" ? item : JSON.stringify(item);
	  });
	  return {
	    inner_width: window.innerWidth,
	    inner_height: window.innerHeight,
	    client_width: doc ? doc.clientWidth : 0,
	    client_height: doc ? doc.clientHeight : 0,
	    scroll_width: doc ? doc.scrollWidth : 0,
	    scroll_height: doc ? doc.scrollHeight : 0,
	    body_scroll_width: body ? body.scrollWidth : 0,
	    device_pixel_ratio: window.devicePixelRatio || 1,
	    overflow_x: scrollWidth > viewportWidth + 1,
	    horizontal_overflow_pixels: Math.max(0, scrollWidth - viewportWidth),
	    visible_elements: visible,
	    overflow_elements: offenders,
	    console_errors: diagnostics.consoleErrors || [],
	    runtime_errors: runtimeErrors,
	    unhandled_rejections: diagnostics.unhandledRejections || []
	  };
	})())`
	raw, err := p.Evaluate(js)
	if err != nil {
		return nil, err
	}
	var diagnostics ViewportDiagnostics
	if err := json.Unmarshal([]byte(utils.InterfaceToString(raw)), &diagnostics); err != nil {
		return nil, fmt.Errorf("decode viewport diagnostics: %w", err)
	}
	if diagnostics.OverflowElements == nil {
		diagnostics.OverflowElements = []ViewportOverflowElement{}
	}
	if diagnostics.ConsoleErrors == nil {
		diagnostics.ConsoleErrors = []string{}
	}
	if diagnostics.RuntimeErrors == nil {
		diagnostics.RuntimeErrors = []string{}
	}
	if diagnostics.UnhandledRejections == nil {
		diagnostics.UnhandledRejections = []string{}
	}
	return &diagnostics, nil
}
