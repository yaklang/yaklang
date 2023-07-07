package main

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mergeproto"
)

const protoFileDir = "common/yakgrpc/protos"
const outputFilePath = "common/yakgrpc/yakgrpc.proto"

func main() {
	b, err := mergeproto.GenProtoBytes(protoFileDir, "ypb")
	if err != nil {
		log.Errorf("unable to generate proto bytes: %v", err)
	}
	err = b.WriteProtoFile(outputFilePath)
	if err != nil {
		log.Errorf("unable to write proto file: %v", err)
	}
}
