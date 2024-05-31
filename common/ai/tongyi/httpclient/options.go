package httpclient

import (
	"time"
)

type HeaderMap map[string]string

func WithHeader(header HeaderMap) HTTPOption {
	return func(c *HTTPCli) {
		for k, v := range header {
			c.req.Header.Set(k, v)
		}
	}
}

func WithTimeout(timeout time.Duration) HTTPOption {
	return func(c *HTTPCli) {
		c.client.Timeout = timeout
	}
}

func WithStream() HTTPOption {
	return func(c *HTTPCli) {
		c.req.Header.Set("Accept", "text/event-stream")
	}
}

func WithTokenHeaderOption(token string) HTTPOption {
	m := map[string]string{"Authorization": "Bearer " + token}
	return WithHeader(m)
}
