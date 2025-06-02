package aireducer

import (
	_ "embed"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

//go:embed testdata/demo.txt.zip
var demoFileZipContent []byte

func TestAIReducer(t *testing.T) {
	zfs, err := filesys.NewZipFSFromString(string(demoFileZipContent))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := zfs.ReadFile("demo.txt")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	reducer, err := NewReducerFromString(
		string(raw),
		WithReducerCallback(func(config *Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
			count++
			spew.Dump(string(chunk.Data()))
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = reducer
	reducer.Run()
	if count <= 1 {
		t.Fatal("Reducer did not process any chunks, expected more than 1")
	}
}
