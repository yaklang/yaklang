package aicommon

func make429Response(headers ...string) *AIResponse {
	header := "HTTP/1.1 429 Too Many Requests\r\n"
	for _, h := range headers {
		header += h + "\r\n"
	}
	header += "\r\n"
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte(header), []byte(`{"error":"rate limited"}`))
	rsp.Close()
	return rsp
}

func make200Response() *AIResponse {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
		[]byte(`{"choices":[]}`),
	)
	rsp.Close()
	return rsp
}
