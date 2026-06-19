package lowhttp

var (
	basicRequest  = []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	basicResponse = []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
)

// BasicResponse 返回一个基本的 HTTP 响应，用于测试，它实际上返回一个b"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
// 参数:
//   - 无
//
// 返回值:
//   - 基本 HTTP 响应报文字节数组
//
// Example:
// ```
// poc.BasicResponse() // b"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
// ```
func BasicResponse() []byte { return basicResponse }

// BasicRequest 返回一个基本的 HTTP 请求，用于测试，它实际上返回一个b"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
// 参数:
//   - 无
//
// 返回值:
//   - 基本 HTTP 请求报文字节数组
//
// Example:
// ```
// poc.BasicRequest() // b"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
// ```
func BasicRequest() []byte { return basicRequest }
