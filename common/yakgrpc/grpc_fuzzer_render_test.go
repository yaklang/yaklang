package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_HTTPFuzzer_RenderFileFuzztag(t *testing.T) {
	var testFile []byte
	for i := 0; i < 10; i++ {
		testFile = append(testFile, 255)
	}

	fileName, err := utils.SaveTempFile(testFile, "fuzztag-test-file")
	if err != nil {
		panic(err)
	}

	fmt.Println(fileName)

	packet := fmt.Sprintf(`POST /post HTTP/1.1
Content-Type: application/json
Host: pie.dev
	
{{file(%s)}}`, fileName)

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := context.WithTimeout(context.Background(), 80*time.Second)
	fuzzerPacket, err := client.RenderHTTPFuzzerPacket(ctx, &ypb.RenderHTTPFuzzerPacketRequest{
		Packet: []byte(packet),
	})
	if err != nil {
		panic(err)
	}

	spew.Dump(fuzzerPacket.GetPacket())
	body := lowhttp.GetHTTPPacketBody(fuzzerPacket.GetPacket())
	if bytes.Compare(body, []byte("{{unquote(\"\\xff\\xff\\xff\\xff\\xff\\xff\\xff\\xff\\xff\\xff\")}}")) != 0 {
		t.Fatal("not equal")
	}
}
