package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_ErrorCode(t *testing.T) {
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
		HotPatchCode: `handle = result => x"{{int(1-10)}}"; panic(1aaa)`,
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
	if count != 1 {
		t.Fatalf("expect 1, got %v", count)
	}
}

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

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_Yield(t *testing.T) {
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
		HotPatchCode: `handle = (result, yield) => {for i in 10 { yield(string(i)) } }`,
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

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzWithHotPatch(t *testing.T) {
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
			`handle=(a)=>{
				assert a =="a|b|c"
				return "ok"
			}`,
			`{{yak(handle|a|b|c)}}`,
		},
		[]any{
			`handle=(a)=>{
				assert a =="a|b"
				return "ok"
			}`,
			`{{yak(handle|a|b)}}`,
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

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_before_and_after_legacy(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	token1 := utils.RandStringBytes(16)
	token2 := utils.RandStringBytes(16)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		if !bytes.Contains(req, []byte(token1)) {
			panic("token1 not found")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nyes")
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: fmt.Sprintf(`
beforeRequest = func(req){
    return poc.ReplaceHTTPPacketBody(req, "%s")
}
afterRequest = func(rsp){
    return poc.ReplaceHTTPPacketBody(rsp, "%s")
}
`, token1, token2),
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect error is nil, got %v", err)
	}
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		// check fuzzer response
		require.Contains(t, string(rsp.ResponseRaw), token2, "afterRequest hotpatch failed")

		// check history response
		out, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
			RuntimeId: rsp.RuntimeID,
		}, 1)
		require.NoError(t, err)
		require.Contains(t, string(out.Data[0].Response), token2, "afterRequest hotpatch failed")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_Mirror_Duplicated_ExtractorResults(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{p(a)}}",
		HotPatchCode: `mirrorHTTPFlow = (req, rsp, params) => {
	return params
}
`,
		ForceFuzz: true,
		Params: []*ypb.FuzzerParamItem{
			{
				Key:   "a",
				Value: "{{int(1-10)}}",
				Type:  "fuzztag",
			},
		},
	})
	require.NoError(t, err)
	count := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		count++
		spew.Dump(rsp.ExtractedResults)
		require.Len(t, rsp.ExtractedResults, 1)
		require.Equal(t, "a", rsp.ExtractedResults[0].Key)
		valueStr := rsp.ExtractedResults[0].Value
		value, err := strconv.Atoi(valueStr)
		require.NoError(t, err)
		require.GreaterOrEqual(t, value, 1)
		require.LessOrEqual(t, value, 10)
	}
	if count != 10 {
		t.Fatalf("expect 10, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_DynHotPatch(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request:      "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{int::1(1-10)}}{{yak:dyn::1(handle)}}",
		HotPatchCode: `handle = result => randstr(10)`,
		ForceFuzz:    true,
	})
	if err != nil {
		t.Fatalf("expect error is nil, but got %v", err)
	}
	count := 0
	for {
		fuzzRequest, err := recv.Recv()
		if err != nil {
			break
		}
		require.Len(t, fuzzRequest.Payloads, 2)
		count++
	}
	require.GreaterOrEqual(t, count, 10)
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_retryHandler(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	flag := utils.RandStringBytes(16)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count > 3 {
			w.Write([]byte(flag))
			return
		}
		w.Write([]byte("no ready"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1)}}"
flag = "` + string(flag) + `"
mirrorHTTPFlow = (req, rsp) => {
	// check if the response contains the flag
    if string(rsp).Contains(flag) {
		println(rsp)
		return {"abc": "aaa"} 
	}
	return {"abc": "no right"}
}

retryHandler = (https,retryCount, req, rsp,retry) => {
	if rsp.Contains("no ready") { retry() }
}

`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	responseCount := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		responseCount++
		check := false
		for _, kv := range rsp.GetExtractedResults() {
			if kv.GetKey() == "abc" {
				if kv.GetValue() == "aaa" {
					check = true
				}
			}
		}
		if !check {
			spew.Dump(rsp.ExtractedResults)
			t.Fatal("mirror http flow output extractor data failed")
		}
	}
	if responseCount != 1 {
		t.Fatalf("expect 1 response, got %v", responseCount)
	}
	if count < 3 {
		t.Fatalf("expect 3 retry response, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_retryHandler_2(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	flag := utils.RandStringBytes(16)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count > 3 {
			w.Write([]byte(flag))
			return
		}
		w.Write([]byte("no ready"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1)}}"
flag = "` + string(flag) + `"
mirrorHTTPFlow = (req, rsp) => {
	// check if the response contains the flag
    if string(rsp).Contains(flag) {
		println(rsp)
		return {"abc": "aaa"} 
	}
	return {"abc": "no right"}
}

retryHandler = (retryCount,req, rsp,retry) => {
	if rsp.Contains("no ready") { return retry()}
}

`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	responseCount := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		responseCount++
		check := false
		for _, kv := range rsp.GetExtractedResults() {
			if kv.GetKey() == "abc" {
				if kv.GetValue() == "aaa" {
					check = true
				}
			}
		}
		if !check {
			spew.Dump(rsp.ExtractedResults)
			t.Fatal("mirror http flow output extractor data failed")
		}
	}
	if responseCount != 1 {
		t.Fatalf("expect 1 response, got %v", responseCount)
	}
	if count < 3 {
		t.Fatalf("expect 3 retry response, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_retryHandler_3(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	flag := utils.RandStringBytes(16)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count > 3 {
			w.Write([]byte(flag))
			return
		}
		w.Write([]byte("no ready"))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1)}}"
flag = "` + string(flag) + `"
mirrorHTTPFlow = (req, rsp) => {
	// check if the response contains the flag
    if string(rsp).Contains(flag) {
		println(rsp)
		return {"abc": "aaa"} 
	}
	return {"abc": "no right"}
}

retryHandler = (req, rsp,retry)  => {
	if rsp.Contains("no ready") { return retry()}
}

`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	responseCount := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		responseCount++
		check := false
		for _, kv := range rsp.GetExtractedResults() {
			if kv.GetKey() == "abc" {
				if kv.GetValue() == "aaa" {
					check = true
				}
			}
		}
		if !check {
			spew.Dump(rsp.ExtractedResults)
			t.Fatal("mirror http flow output extractor data failed")
		}
	}
	if responseCount != 1 {
		t.Fatalf("expect 1 response, got %v", responseCount)
	}
	if count < 3 {
		t.Fatalf("expect 3 retry response, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_customFailureChecker(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	flag := utils.RandStringBytes(16)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(flag))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1)}}"
flag = "` + string(flag) + `"

customFailureChecker = (https, req, rsp, fail) => {
	if (rsp.Contains(flag)) { fail("错误内容。。。") }
}

`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	responseCount := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		responseCount++

		spew.Dump(rsp)
		require.NotEmpty(t, rsp.Reason)
		require.Contains(t, rsp.Reason, "request failed intentionally by custom failure checker")
		require.Contains(t, rsp.Reason, "错误内容。。。")
		require.Contains(t, string(rsp.ResponseRaw), flag) // 强制失败也有响应
		spew.Dump(rsp.ExtractedResults)
	}
	require.Equal(t, 1, responseCount)
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_customFailureChecker_3_args(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	flag := utils.RandStringBytes(16)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(flag))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1)}}"
flag = "` + string(flag) + `"

customFailureChecker = (req, rsp, fail) => {
	if (rsp.Contains(flag)) { fail("3 args 错误内容。。。") }
}

`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	responseCount := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		responseCount++
		require.NotEmpty(t, rsp.Reason)
		require.Contains(t, rsp.Reason, "request failed intentionally by custom failure checker")
		require.Contains(t, rsp.Reason, "3 args 错误内容。。。")
		require.Contains(t, string(rsp.ResponseRaw), flag)

	}
	require.Equal(t, 1, responseCount)
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_customFailureChecker_2_args(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	flag := utils.RandStringBytes(16)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(flag))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1)}}"
flag = "` + string(flag) + `"

customFailureChecker = (rsp, fail) => {
	if (rsp.Contains(flag)) { fail("2 args 错误内容。。。") }
}

`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	responseCount := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		responseCount++

		require.NotEmpty(t, rsp.Reason)
		require.Contains(t, rsp.Reason, "request failed intentionally by custom failure checker")
		require.Contains(t, rsp.Reason, "2 args 错误内容。。。")
		require.Contains(t, string(rsp.ResponseRaw), flag)

	}
	require.Equal(t, 1, responseCount)
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_customFailureChecker_no_fail(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	flag := utils.RandStringBytes(16)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(flag))
	})
	target := utils.HostPort(host, port)
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n{{yak(handle)}}",
		HotPatchCode: `handle = result => x"{{int(1)}}"
flag = "` + string(flag) + `"

customFailureChecker = (https, req, rsp, fail) => {
	// Do not call fail function
}

`,
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	responseCount := 0
	for {
		rsp, err := recv.Recv()
		if err != nil {
			break
		}
		responseCount++
		require.Empty(t, rsp.Reason)
		require.Contains(t, string(rsp.ResponseRaw), flag)
	}
	require.Equal(t, 1, responseCount)
}
