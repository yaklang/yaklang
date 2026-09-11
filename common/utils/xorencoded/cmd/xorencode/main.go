// xorencode 是一个构建期 CLI 工具，用于将明文配置文件 XOR 编码生成 .enc 文件。
//
// 用法:
//
//	xorencode -input config.json -output config.json.enc -key yaklang-wsm-v1
//	xorencode -input ssti.txt -output ssti.txt.enc -key yaklang-dicts-v1
//
// 通常通过 //go:generate 指令调用，例如:
//
//	//go:generate xorencode -input embed_data/ssti.txt -output embed_data/ssti.txt.enc -key yaklang-dicts-v1
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/utils/xorencoded"
)

func main() {
	input := flag.String("input", "", "input file (plaintext)")
	output := flag.String("output", "", "output file (XOR encoded)")
	key := flag.String("key", "", "XOR key")
	flag.Parse()

	if *input == "" || *output == "" || *key == "" {
		fmt.Fprintln(os.Stderr, "usage: xorencode -input <file> -output <file> -key <key>")
		os.Exit(1)
	}

	if err := xorencoded.EncodeFileToFile(*input, *output, []byte(*key)); err != nil {
		fmt.Fprintf(os.Stderr, "xorencode failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("xorencode: %s -> %s\n", *input, *output)
}
