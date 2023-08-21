package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"runtime/debug"
	"testing"
	"time"
)

//func TestServer_Pipe(t *testing.T) {
//	r, w, err := os.Pipe()
//	if err != nil {
//		panic(err)
//	}
//	origin := os.Stdout
//	defer func() {
//		if err := recover(); err != nil {
//			log.Error(err)
//		}
//		os.Stdout = origin
//	}()
//	os.Stdout = w
//	var buf bytes.Buffer
//	go io.Copy(&buf, r)
//	time.Sleep(time.Second)
//	fmt.Println("Hello World")
//	fmt.Println("--------------------------------------------------")
//	time.Sleep(time.Second)
//	spew.Dump(buf.Bytes())
//}

func TestServer_AttachCombinedOutput(t *testing.T) {
	// 输出调用栈
	debug.PrintStack()
	println("run TestServer_AttachCombinedOutput")
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.AttachCombinedOutput(context.Background(), &ypb.AttachCombinedOutputRequest{})
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			fmt.Println("HHHHHHHEEELLLLLOOOO WWWWWOOOOORRRRRLLLLLLLDDDDDDDD")
			time.Sleep(1000 * time.Millisecond)
		}
	}()

	right := false
	for {
		result, err := stream.Recv()
		if err != nil {
			break
		}
		if result.Raw != nil {
			right = true
			println(string(result.Raw))
		}
	}
	time.Sleep(time.Second * 3)
	if !right {
		panic("stream cannot recv temp file")
	}
}
