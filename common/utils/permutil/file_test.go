package permutil

import (
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"testing"
)

func TestIsFileUnreadAndUnWritable(t *testing.T) {
	fp, err := os.CreateTemp("", "test.txt")
	if err != nil {
		panic(err)
	}
	fp.Write([]byte("Hello FP"))
	fp.Close()

	if IsFileUnreadAndUnWritable(fp.Name()) {
		panic("1")
	}

	os.Chmod(fp.Name(), 0111)
	if !IsFileUnreadAndUnWritable(fp.Name()) {
		panic(2)
	}

	os.Chmod(fp.Name(), 0644)
	if IsFileUnreadAndUnWritable(fp.Name()) {
		panic("3")
	}

	if IsFileUnreadAndUnWritable(fp.Name() + utils.RandStringBytes(12) + ".txt") {
		panic("3")
	}

	defer os.RemoveAll(fp.Name())
}
