package aicommon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAIResponse_GetHTTPStatusCode_Nil(t *testing.T) {
	var resp *AIResponse
	assert.Equal(t, 0, resp.GetHTTPStatusCode(), "nil response should return 0")
}

func TestAIResponse_GetHTTPStatusCode_Empty(t *testing.T) {
	resp := &AIResponse{}
	assert.Equal(t, 0, resp.GetHTTPStatusCode(), "empty response should return 0")
}

func TestAIResponse_GetHTTPStatusCode_200(t *testing.T) {
	resp := &AIResponse{}
	resp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
		[]byte(`{"choices":[]}`),
	)
	assert.Equal(t, 200, resp.GetHTTPStatusCode())
}

func TestAIResponse_GetHTTPStatusCode_429(t *testing.T) {
	resp := &AIResponse{}
	header := "HTTP/1.1 429 Too Many Requests\r\n" +
		"Content-Type: application/json; charset=utf-8\r\n" +
		"X-AIBalance-Info: 5\r\n" +
		"Retry-After: 10\r\n" +
		"\r\n"
	body := `{"error":{"message":"Rate limit exceeded","type":"rate_limit_exceeded","queue_length":5}}`
	resp.SetRawHTTPResponseData([]byte(header), []byte(body))

	assert.Equal(t, 429, resp.GetHTTPStatusCode())
}

func TestAIResponse_GetHTTPStatusCode_401(t *testing.T) {
	resp := &AIResponse{}
	resp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 401 Unauthorized\r\nContent-Type: application/json\r\n\r\n"),
		[]byte(`{"error":"unauthorized"}`),
	)
	assert.Equal(t, 401, resp.GetHTTPStatusCode())
}

func TestAIResponse_GetHTTPStatusCode_502(t *testing.T) {
	resp := &AIResponse{}
	resp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"),
		nil,
	)
	assert.Equal(t, 502, resp.GetHTTPStatusCode())
}

func TestAIResponse_GetHTTPHeader_Nil(t *testing.T) {
	var resp *AIResponse
	assert.Equal(t, "", resp.GetHTTPHeader("X-AIBalance-Info"))
}

func TestAIResponse_GetHTTPHeader_Empty(t *testing.T) {
	resp := &AIResponse{}
	assert.Equal(t, "", resp.GetHTTPHeader("X-AIBalance-Info"))
}

func TestAIResponse_GetHTTPHeader_Present(t *testing.T) {
	resp := &AIResponse{}
	header := "HTTP/1.1 429 Too Many Requests\r\n" +
		"Content-Type: application/json\r\n" +
		"X-AIBalance-Info: 42\r\n" +
		"Retry-After: 10\r\n" +
		"\r\n"
	resp.SetRawHTTPResponseData([]byte(header), nil)

	assert.Equal(t, "42", resp.GetHTTPHeader("X-AIBalance-Info"))
	assert.Equal(t, "10", resp.GetHTTPHeader("Retry-After"))
	assert.Equal(t, "application/json", resp.GetHTTPHeader("Content-Type"))
}

func TestAIResponse_GetHTTPHeader_Missing(t *testing.T) {
	resp := &AIResponse{}
	header := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n"
	resp.SetRawHTTPResponseData([]byte(header), nil)

	assert.Equal(t, "", resp.GetHTTPHeader("X-AIBalance-Info"))
}

func TestAIResponse_SetRawHTTPResponseData_Overwrite(t *testing.T) {
	resp := &AIResponse{}
	resp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 200 OK\r\n\r\n"),
		[]byte("body1"),
	)
	assert.Equal(t, 200, resp.GetHTTPStatusCode())

	resp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 429 Too Many Requests\r\nX-AIBalance-Info: 3\r\n\r\n"),
		[]byte("body2"),
	)
	assert.Equal(t, 429, resp.GetHTTPStatusCode())
	assert.Equal(t, "3", resp.GetHTTPHeader("X-AIBalance-Info"))
}

func TestAIResponse_GetRawHTTPResponseDump_Concat(t *testing.T) {
	resp := &AIResponse{}
	header := "HTTP/1.1 429 Too Many Requests\r\nX-AIBalance-Info: 7\r\n\r\n"
	body := `{"error":"rate limited"}`
	resp.SetRawHTTPResponseData([]byte(header), []byte(body))

	dump := resp.GetRawHTTPResponseDump()
	assert.Contains(t, dump, "429")
	assert.Contains(t, dump, "X-AIBalance-Info: 7")
	assert.Contains(t, dump, `"error":"rate limited"`)
}

func TestAIResponse_ConcurrentSetAndGet(t *testing.T) {
	resp := &AIResponse{}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			resp.SetRawHTTPResponseData(
				[]byte("HTTP/1.1 429 Too Many Requests\r\nX-AIBalance-Info: 5\r\n\r\n"),
				[]byte(`{"error":"rate limited"}`),
			)
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		resp.GetHTTPStatusCode()
		resp.GetHTTPHeader("X-AIBalance-Info")
		resp.GetRawHTTPResponseDump()
	}
	<-done
}
