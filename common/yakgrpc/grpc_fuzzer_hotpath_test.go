package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
		t.Fatalf("expect error is nil, but got %v", err)
	}
	count := 0
	for {
		_, err := recv.Recv()
		if err != nil {
			break
		}
		count++
	}
	if count != 10 {
		t.Fatalf("expect 10, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_Mirror(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1-10)}}"

mirrorHTTPFlow = (req, rsp) => {
	return {"abc": "aaa"}
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
		check := false
		for _, kv := range rsp.GetExtractedResults() {
			if kv.GetKey() == "abc" {
				if kv.GetValue() == "aaa" {
					check = true
				}
			}
		}
		if !check {
			t.Fatal("mirror http flow output extractor data failed")
		}
	}
	if count != 10 {
		t.Fatalf("expect 10, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_Mirror2(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1-10)}}"

mirrorHTTPFlow = (req, rsp, variables) => {
	dump(variables)
	assert ("cc1" in variables) && (variables["cc1"] == "c");
	return {"abc": "aaa"}
}
`,
		ForceFuzz: true,
		Params: []*ypb.FuzzerParamItem{
			{Key: "cc1", Value: "c"},
		},
	})
	if err != nil {
		t.Fatalf("expect error is nil, got %v", err)
	}
	count := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		count++
		check := false
		for _, kv := range rsp.GetExtractedResults() {
			if kv.GetKey() == "abc" {
				if kv.GetValue() == "aaa" {
					check = true
				}
			}
		}
		if !check {
			t.Fatal("mirror http flow output extractor data failed")
		}
	}
	if count != 10 {
		t.Fatalf("expect 10, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_Mirror_PANIC(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1-10)}}"

mirrorHTTPFlow = (req, rsp) => {
	die(1)
	return {"abc": "aaa"}
}
`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect error is nil, got %v", err)
	}
	count := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		_ = rsp.Url
		fmt.Println(rsp.Url)
		count++
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
	dec, _ := codec.SM2DecryptC1C3C2(pri, data)
	if string(dec) != "aaa" {
		t.Fatalf("dec c1c3c2 error. dec: %v", string(dec))
	}
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
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
		t.Fatalf("expect error is nil, got %v", err)
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
		t.Fatal(err)
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
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(100), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle|1)}}{{yak(errFunc)}}{{yak(handle1)}}",
		HotPatchCode: `
handle = func(i){
    return i
}
handle1 = s => {die("expected panic")}
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
		if payloads[2] == "["+fuzztag.YakHotPatchErr+"expected panic]" {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("expect 3, got %v", count)
	}
}
func TestGRPCMUSTPASS_FuzzWithHotPatch(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	for _, itestCase := range []any{
		[]any{
			`handle=(a)=>{
				assert a =="a|b"
				return "ok"
			}`,
			`{{yak(handle|a|b)}}`,
		},
		[]any{
			`handle=(a,b)=>{
				assert a =="a" && b=="b|c"
				return "ok"
			}`,
			`{{yak(handle|a|b|c)}}`,
		},
		[]any{
			`handle=(a,b,c,d)=>{
				assert a =="a" && b=="b" && c=="" && d==""
				return "ok"
			}`,
			`{{yak(handle|a|b)}}`,
		},
		[]any{
			`handle=(params...)=>{
				data = ["a","b","c"]
				for i=0;i<3;i++ {
					assert params[i] == data[i]
				}
				return "ok"
			}`,
			`{{yak(handle|a|b|c)}}`,
		},
	} {
		testCase := itestCase.([]any)
		code := testCase[0].(string)
		template := testCase[1].(string)
		res, err := client.StringFuzzer(context.Background(), &ypb.StringFuzzerRequest{
			Template:     template,
			HotPatchCode: code,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Results) != 1 || string(res.Results[0]) != "ok" {
			t.Fatal(spew.Sprintf("hotpatch fail: %v,%v", template, code))
		}
	}
}
