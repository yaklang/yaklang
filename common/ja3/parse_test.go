package ja3

import (
	"github.com/davecgh/go-spew/spew"
	"io"
	"net/http"
	"testing"
)

// Cobalt Strike win10-x64 stageless beacon JA3
func TestParseJA3(t *testing.T) {
	ja3, err := ParseJA3("771,49196-49195-49200-49199-49188-49187-49192-49191-49162-49161-49172-49171-157-156-61-60-53-47-10,5-10-11-13-35-23-65281,29-23-24,0")
	if err != nil {
		panic(err)
	}
	spew.Dump(ja3)
}

// Cobalt Strike Centos https-ip team-server JA3S
func TestParseJA3S(t *testing.T) {
	ja3s, err := ParseJA3S("771,49200,23-65281")
	if err != nil {
		panic(err)
	}
	spew.Dump(ja3s)
}

func TestCustomJA3HttpRequest(t *testing.T) {
	spec, err := ParseJA3ToClientHelloSpec("771,49196-49195-49200-49199-49188-49187-49192-49191-49162-49161-49172-49171-157-156-61-60-53-47-10,5-10-11-13-35-23-65281,29-23-24,0")
	if err != nil {
		panic(err)
	}
	trans := GetTransportByClientHelloSpec(spec)
	req, err := http.NewRequest("GET", "https://tls.peet.ws/api/clean", nil)
	if err != nil {
		panic(err)
	}
	req.Header = http.Header{
		"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
		"Accept-Language": {"en-US,en;q=0.5"},
		"Cache-Control":   {"no-cache"},
		"Pragma":          {"no-cache"},
		"User-Agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:88.0) Gecko/20100101 Firefox/88.0"},
	}
	nativeClient := &http.Client{}
	nativeRsp, err := nativeClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer nativeRsp.Body.Close()
	nativeResp, err := io.ReadAll(nativeRsp.Body)
	println(string(nativeResp))

	res, err := trans.RoundTrip(req)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()
	resp, err := io.ReadAll(res.Body)
	println(string(resp))
}

func TestIfJA3GetBanned(t *testing.T) {

	req, err := http.NewRequest("GET", "https://www.howsmyssl.com/a/check", nil)
	if err != nil {
		panic(err)
	}
	req.Header = http.Header{
		"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
		"Accept-Language": {"en-US,en;q=0.5"},
		"Cache-Control":   {"no-cache"},
		"Pragma":          {"no-cache"},
		"User-Agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:88.0) Gecko/20100101 Firefox/88.0"},
	}
	nativeClient := &http.Client{}
	nativeRsp, err := nativeClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer nativeRsp.Body.Close()
	nativeResp, err := io.ReadAll(nativeRsp.Body)
	println(string(nativeResp))

}
