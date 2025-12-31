package yakit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestYieldAIEvent(t *testing.T) {
	db := consts.GetGormProfileDatabase().Debug()
	QueryAIEvent(db, &ypb.AIEventFilter{
		EventUUIDS: []string{uuid.NewString(), uuid.NewString()},
	})

}
