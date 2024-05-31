package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"log"
	"os"
	"os/user"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/ai/tongyi/wanx"
)

func main() {
	model := wanx.WanxV1
	token := os.Getenv("DASHSCOPE_API_KEY")
	if token == "" {
		panic("token is empty")
	}

	cli := dashscopego.NewTongyiClient(model, token)

	req := &wanx.ImageSynthesisRequest{
		// Model: "wanx-v1",
		Model: model,
		Input: wanx.ImageSynthesisInput{
			Prompt: "A beautiful sunset",
		},
		Params: wanx.ImageSynthesisParams{
			N: 1,
		},
		// 是否下载图片, 默认不下载，仅获取url
		Download: true,
	}
	ctx := context.TODO()

	imgBlobs, err := cli.CreateImageGeneration(ctx, req)
	if err != nil {
		panic(err)
	}

	for _, blob := range imgBlobs {
		// 查看生成的图片url
		//nolint:forbidigo
		print(blob.ImgURL)
		// 下载图片到桌面
		if req.Download {
			saveImg2Desktop(blob.ImgType, blob.Data)
		}
	}
}

func saveImg2Desktop(_ string, data []byte) {
	buf := bytes.NewBuffer(data)
	img, _, err := image.Decode(buf)
	if err != nil {
		log.Fatal(err)
	}

	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	f, err := os.Create(filepath.Join(usr.HomeDir, "Desktop", "wanx_image.png"))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
