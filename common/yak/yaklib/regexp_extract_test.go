package yaklib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMUSTPASS_Common_RegexpMatchPathParam(t *testing.T) {
	assert.Equal(t, []string{"/docs/api/re?"}, RegexpMatchPathParam("agsdjkfkaj yaklang.com/docs/api/re? asgdashjd"))
	assert.Equal(t, []string{"/docs/api/re"}, RegexpMatchPathParam("agsdjkfkaj yaklang.com/docs/api/re asgdashjd"))
	assert.Equal(t, []string{"/docs/api/re?name=anonymous"}, RegexpMatchPathParam("agsdjkfkaj yaklang.com/docs/api/re?name=anonymous"))
	assert.Equal(t, []string{"/docs/api/re?name=anonymous"}, RegexpMatchPathParam("asgdjashttp://yaklang.com/docs/api/re?name=anonymous asdasdggjkasd"))
	assert.Equal(t, []string{"/docs/api/re?name=anonymous"}, RegexpMatchPathParam("aasd http://yaklang.com/docs/api/re?name=anonymous#tqwyuetasgd"))
}
