package lowhttp

var (
	basicRequest = []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
)

// BasicRequest 返回一个基本的 HTTP 请求，用于测试，它实际上返回一个b"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
// Example:
// ```
// poc.BasicRequest() // b"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
// ```
func BasicRequest() []byte {
	return basicRequest
}
