package test

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"os"
	"testing"
	"time"
)

var flag string

func init() {
	s := "dGhpcyBpcyBnZW4gZW1iZWQgdGVzdCBmaWxl"
	f, _ := codec.DecodeBase64(s)
	flag = string(f)
}
func TestFs(t *testing.T) {
	content, err := FS.ReadFile("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, string(flag), string(content))
	p, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exeContent, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	strFlag := "this is a test flag string"
	if !bytes.Contains(exeContent, []byte(strFlag)) {
		t.Fatal(errors.New("string flag should be in the executable file"))
	}
	if bytes.Contains(exeContent, []byte(flag)) {
		t.Fatal(errors.New("flag should not be in the executable file"))
	}
}

func TestCache(t *testing.T) {
	cachedFs, err := gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", true)
	if err != nil {
		t.Fatal(err)
	}
	notcachedFs, err := gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", false)
	if err != nil {
		t.Fatal(err)
	}
	content, err := cachedFs.ReadFile("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != flag {
		t.Fatal(errors.New("read file by cached fs failed"))
	}
	content, err = notcachedFs.ReadFile("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != flag {
		t.Fatal(errors.New("read file by not cached fs failed"))
	}
	calcDuration := func(fs *gzip_embed.PreprocessingEmbed) int64 {
		start := time.Now()
		for i := 0; i < 100; i++ {
			_, err := fs.ReadFile("1.txt")
			if err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(start).Nanoseconds()
	}
	cachedDu := calcDuration(cachedFs)
	notcachedDu := calcDuration(notcachedFs)
	fmt.Printf("cached fs duration: %d, not cached fs duration: %d\n", cachedDu, notcachedDu)
	if cachedDu*10 >= notcachedDu {
		t.Fatal(errors.New("cached fs should be at least 10 times faster than not cached fs"))
	}
}
