package yaktest

import (
	"github.com/yaklang/yaklang/common/yak"
	_ "github.com/yaklang/yaklang/common/yakgrpc"
	"testing"
)

func TestSpaceEngine(t *testing.T) {
	engine, err := yak.Execute(`for i in spacengine.Query("Swagger", spacengine.hunter())~ {
    dump(i)
    db.SavePortFromResult(i)
}`)
	if err != nil {
		panic(err)
	}
	_ = engine
}
