package mergeproto

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"os"
)

type Buffer struct {
	buf *bytes.Buffer
}

func NewBuffer() *Buffer {
	return &Buffer{buf: &bytes.Buffer{}}
}

func (b *Buffer) Printf(f string, i ...any) {
	_, _ = b.buf.WriteString(fmt.Sprintf(f+"\n", i...))
}

func (b *Buffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *Buffer) String() string {
	return b.buf.String()
}

func (b *Buffer) WriteProtoFile() error {
	err := os.WriteFile("../../yakgrpc/yakgrpc.proto", b.Bytes(), 0644)
	if err != nil {
		log.Fatalf("unable to write file: %v", err)
		return err
	}
	return nil
}
