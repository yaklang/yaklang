package webfingerprint_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
)

func TestParseToCPE(t *testing.T) {
	trueCases := map[string]struct {
		origin     string
		likeSearch string
	}{
		"cpe:/a:*:aaaa:1.1": {
			origin:     "cpe:/a:*:aaaa:1.1:*:*:*",
			likeSearch: "%cpe:/a:%:aaaa:1.1%",
		},
		"cpe:/a:%:aaaaa:1.1:%:%:%": {
			origin:     "cpe:/a:*:aaaaa:1.1:*:*:*",
			likeSearch: "%cpe:/a:%:aaaaa:1.1%",
		},
		"cpe:2.3:h:*:hikvision:*:*": {
			origin:     "cpe:/h:*:hikvision:*:*:*:*",
			likeSearch: "%cpe:/h:%:hikvision%",
		},
	}

	for origin, result := range trueCases {
		r, err := webfingerprint.ParseToCPE(origin)
		assert.Nil(t, err)

		assert.Equal(t, r.String(), result.origin)
		assert.Equal(t, r.LikeSearchString(), result.likeSearch)
	}
}

func TestParseToCPE1(t *testing.T) {
	cpes := []string{
		"cpe:/a:debian:debian_linux:Debian:*",
	}
	db := consts.GetGormCVEDatabase()

	for _, cpe := range cpes {
		r, err := webfingerprint.ParseToCPE(cpe)
		assert.Nil(t, err)
		for res := range cvequeryops.QueryCVEYields(db, cvequeryops.ProductWithVersion(r.Product, r.Version)) {
			fmt.Println(res.Product)
		}
	}
}
