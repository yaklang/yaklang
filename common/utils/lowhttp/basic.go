package lowhttp

var (
	basicRequest = []byte(`GET / HTTP/1.1
Host: example.com

`)
	onceFixRequest = false
)

func BasicRequest() []byte {
	if !onceFixRequest {
		onceFixRequest = true
		basicRequest = FixHTTPRequest(basicRequest)
	}
	return basicRequest
}
