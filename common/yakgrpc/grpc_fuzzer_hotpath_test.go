package yakgrpc

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"testing"
)

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request:      "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1-10)}}"`,
		ForceFuzz:    true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		fmt.Println(string(rsp.RequestRaw))
		fmt.Println(string(rsp.ResponseRaw))
	}
}
