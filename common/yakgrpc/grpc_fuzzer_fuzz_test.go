package yakgrpc

import (
	"fmt"
	"github.com/yaklang/yaklang/common/mutate"
	"testing"
)

func TestFuzzParams(f *testing.T) {
	raw := `POST /file.php HTTP/1.1
Host: www.example.com
Content-Type: application/json; charset=UTF-8
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36

{"abc": 123, "keyInQuery": {"xxx":"xxxx","yyy":"yyyy"}}`
	freq, _ := mutate.NewFuzzHTTPRequest([]byte(raw))
	for _, param := range freq.GetPostCommonParams() {
		req := param.Fuzz("________________").FirstFuzzHTTPRequest()
		//if err != nil {
		//	f.Errorf("error: %s", err)
		//}
		fmt.Println(string(req.GetBody()))
	}
}
