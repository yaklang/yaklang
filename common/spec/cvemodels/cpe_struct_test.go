package cvemodels

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

import (
	"fmt"
)

func TestCPEStruct(t *testing.T) {
	cpe := &CpeStruct{
		Vendor:   "nginx",
		Product:  "nginx",
		Version:  "1.4",
		Language: "en",
	}

	re, err := cpe.Regexp()
	if err != nil {
		log.Error(err)
		return
	}
	fmt.Printf("%v\n", re.String())

	assert.True(t, re.MatchString("cpe:2.3:a:nginx:nginx:1.4:::en:"))
	assert.True(t, re.MatchString(cpe.CPE23String()))
}
