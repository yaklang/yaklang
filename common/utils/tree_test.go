package utils

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGeneratePathTrees(t *testing.T) {
	test := assert.New(t)

	f, err := GeneratePathTrees(
		"/root/",
		"/root/1",
		"/root/2",
		"/root/2/23r/23t",
		"/root/2/qey23/asd/asdfa/",
		"/root/3/asdf",
		"/root/4.1235/93kcasdf",
		"/xx/4.1235/93kcasdf",
	)
	if err != nil {
		test.FailNow(err.Error())
	}

	raw, err := json.MarshalIndent(f.Output(), "", "    ")
	if err != nil {
		test.FailNow(err.Error())
	}

	println(string(raw))
}
