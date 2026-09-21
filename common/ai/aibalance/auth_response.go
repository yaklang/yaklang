package aibalance

import (
	"net/http"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Only the first attempt's 401 metadata is delayed. Successful streaming
// headers remain immediate; a non-recoverable 401 is flushed by the caller.
func deferAuthResponseCallbacks(config *aispec.AIConfig) (*aispec.AIConfig, func()) {
	copyConfig := *config
	var pending []func()
	var mu sync.Mutex
	dispatch := func(header []byte, callback func()) {
		if lowhttp.GetStatusCodeFromResponse(header) == http.StatusUnauthorized {
			mu.Lock()
			pending = append(pending, callback)
			mu.Unlock()
			return
		}
		callback()
	}
	copyConfig.RawHTTPResponseHeaderCallback = func(header []byte) {
		if config.RawHTTPResponseHeaderCallback != nil {
			header = append([]byte(nil), header...)
			dispatch(header, func() { config.RawHTTPResponseHeaderCallback(header) })
		}
	}
	copyConfig.RawHTTPResponseCallback = func(header, body []byte) {
		if config.RawHTTPResponseCallback != nil {
			header, body = append([]byte(nil), header...), append([]byte(nil), body...)
			dispatch(header, func() { config.RawHTTPResponseCallback(header, body) })
		}
	}
	copyConfig.RawHTTPRequestResponseCallback = func(request, header, body []byte, usage *aispec.ChatUsage) {
		if config.RawHTTPRequestResponseCallback != nil {
			request = append([]byte(nil), request...)
			header, body = append([]byte(nil), header...), append([]byte(nil), body...)
			dispatch(header, func() { config.RawHTTPRequestResponseCallback(request, header, body, usage) })
		}
	}
	return &copyConfig, func() {
		mu.Lock()
		callbacks := pending
		pending = nil
		mu.Unlock()
		for _, callback := range callbacks {
			callback()
		}
	}
}
