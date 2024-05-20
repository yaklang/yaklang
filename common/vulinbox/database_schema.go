package vulinbox

import (
	"github.com/yaklang/yaklang/common/consts"
)

var VulinBoxTables = []any{
	&VulinServer{},
	&Session{},
	&UserOrder{},
	&UserCart{},
}

func init() {
	consts.RegisterDatabaseSchema(consts.KEY_SCHEMA_VULINBOX_DATABASE, VulinBoxTables...)
}
