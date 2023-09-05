package yakgrpc

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
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
	count := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		count++
		fmt.Println(string(rsp.RequestRaw))
		fmt.Println(string(rsp.ResponseRaw))
	}
	if count != 10 {
		t.Fatalf("expect 10, got %v", count)
	}
}

/*
handle = func(param) {
a = codec.Sm2EncryptC1C3C2("0487c856a4a19e2cdc4271e839ea0ca3f8e6622f5de3a3190bb339641e225d28ef3d26348621d373d40c750af60e8dfd2154f4fd1d43fc0405faeeb15235715512", param)~
dump(a)

return "aaa" + sprintf("_origin(%v)", param)
}
*/

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch2(t *testing.T) {
	pri, key, err := codec.GenerateSM2PrivateKeyHEX()
	if err != nil {
		panic(err)
	}
	data, _ := codec.SM2EncryptC1C3C2(key, []byte("aaa"))
	spew.Dump(data)
	dec, _ := codec.SM2DecryptC1C3C2(pri, data)
	spew.Dump(dec)
	if string(dec) != "aaa" {
		panic("dec c1c3c2 error")
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle|{{params(a)}})}}",
		HotPatchCode: `
handle = func(param) {
dump("************************************" * 2)
a = codec.Sm2EncryptC1C3C2("0487c856a4a19e2cdc4271e839ea0ca3f8e6622f5de3a3190bb339641e225d28ef3d26348621d373d40c750af60e8dfd2154f4fd1d43fc0405faeeb15235715512", "aaa")~
dump(a)

return "aaa" + sprintf("_origin(%v)", param)
}
`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	count := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		count++
		fmt.Println(string(rsp.RequestRaw))
		fmt.Println(string(rsp.ResponseRaw))
	}
	if count != 1 {
		t.Fatalf("expect 1, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch3ErrCheck(t *testing.T) {

	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err2 := io.ReadAll(r.Body)
		if err2 != nil {
			w.Write([]byte("err"))
			return
		}
		w.Write(body)
		return
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle|1)}}{{yak(errFunc)}}",
		HotPatchCode: `
handle = func(i){
    return i
}
`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	count := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		payloads := rsp.Payloads
		if payloads[0] == "1" {
			count++
		}
		if payloads[1] == "["+fuzztag.YakHotPatchErr+"function errFunc not found]" {
			count++
		}
		fmt.Println(string(rsp.RequestRaw))
		fmt.Println(string(rsp.ResponseRaw))
	}
	if count != 2 {
		t.Fatalf("expect 2, got %v", count)
	}
}
