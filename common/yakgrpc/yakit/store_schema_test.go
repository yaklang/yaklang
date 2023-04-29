package yakit

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUpdateYakitStore(t *testing.T) {
	test := assert.New(t)

	err := UpdateYakitStore(nil, "")
	if err != nil {
		test.FailNow(err.Error())
	}
}

func TestLoadYakitThirdpartySourceScripts(t *testing.T) {
	LoadYakitThirdpartySourceScripts(
		context.Background(),
		"https://github.com/yaklang/yakit-store",
	)
}

func TestLoadYakitThirdpartySourceScripts1(t *testing.T) {
	LoadYakitFromLocalDir(
		"/Users/v1ll4n/Projects/yak-script",
	)
}
