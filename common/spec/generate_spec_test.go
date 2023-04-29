package spec

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGeneratePalmRpcByYaml(t *testing.T) {
	test := assert.New(t)

	raw, err := JenGeneratePalmRpcByYaml([]byte(`
package_name: test
name: ManagerAPI
rpcs:
  - method: Shutdown
    request:
      - name: Name
        type: "[]string"
      - name: Z
        type: "string"
    response:
      - name: Ok
        type: bool
      - name: Reason
        type: string
models:
  - name: FileInfo
    fields:
      - name: Path
        type: str
`))
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	println(string(raw))
}
