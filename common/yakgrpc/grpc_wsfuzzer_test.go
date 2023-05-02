package yakgrpc

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/anypb"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestWsFuzzer(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.CreateWebsocketFuzzer(utils.TimeoutContextSeconds(20))
	if err != nil {
		panic(err)
	}
	stream.Send(&ypb.ClientWebsocketRequest{
		IsTLS: true,
		UpgradeRequest: []byte(`
GET /ws HTTP/1.1
Host: v1ll4n.local:8885
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: no-cache
Connection: Upgrade
Cookie: PHPSESSID=upube8i55iuim3khf5bnvttab7; security=low
Origin: https://v1ll4n.local:8885
Pragma: no-cache
Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
Sec-WebSocket-Key: 62HzcscpHVLdq0MlgjMA/A==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36
`),
		TotalTimeoutSeconds: 20,
	})
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	time.Sleep(time.Second)
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	time.Sleep(time.Second)
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	time.Sleep(time.Second)
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	time.Sleep(time.Second)
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	time.Sleep(time.Second)
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	time.Sleep(time.Second)
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	time.Sleep(time.Second)
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})
	stream.Send(&ypb.ClientWebsocketRequest{ToServer: []byte(`HfasdfasdHfasdfasdHfasdfasdHfasdfasd`)})

}

func TestPBTest(t *testing.T) {
	a := "080110012001405f4a5f7b226d657373616765223a22476f6c616e6720576562736f636b6574204d6573736167653a20323032322d30392d30362032323a35333a32392e363336303732202b3038303020435354206d3d2b343632362e333935343136323130227d0a5001"
	raw, _ := codec.DecodeHex(a)
	spew.Dump(raw)

	var err error
	anyPB := &anypb.Any{}
	err = proto.Unmarshal(raw, anyPB)
	if err != nil {
		panic(err)
	}

	spew.Dump(anyPB)
	fields := anyPB.ProtoReflect().GetUnknown()
	spew.Dump(fields)
	for {
		index /*int32*/, data, n := protowire.ConsumeTag(fields)
		if n < 0 {
			break
		}

		fields = fields[n:]
		n = protowire.ConsumeFieldValue(index, data, fields)
		value := fields[:n]
		fields = fields[n:]
		spew.Dump(index, data, n, value)

		time.Sleep(time.Second)
	}
	//spew.Dump(anyPB.AsMap())
	jsonRaw, err := protojson.Marshal(anyPB)
	if err != nil {
		panic(err)
	}

	spew.Dump(jsonRaw)
}

func TestIsProtobuf(t *testing.T) {
	a := "080110012001405f4a5f7b226d657373616765223a22476f6c616e6720576562736f636b6574204d6573736167653a20323032322d30392d30362032323a35333a32392e363336303732202b3038303020435354206d3d2b343632362e333935343136323130227d0a5001"
	raw, _ := codec.DecodeHex(a)
	spew.Dump(raw)
	if !utils.IsProtobuf(raw) {
		panic(1)
	}

	a = "080110012001405f4a5f7b226d657373616765223a22476f6c616e6720576562736f636b6574204d6573736167653a20323032322d30392d30362032323a35333a32392e363336303732202b3038303020435354206d3d2b343632362e333935343136323130227d0a500111"
	raw, _ = codec.DecodeHex(a)
	spew.Dump(raw)
	if utils.IsProtobuf(raw) {
		panic(1)
	}
}
