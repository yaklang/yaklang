package vulinbox

import (
	"github.com/yaklang/yaklang/common/schema"
)

var VulinBoxTables = []any{
	&VulinUser{},
	&Session{},
	&UserOrder{},
	&UserCart{},
	&VulinVisitor{},
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_VULINBOX_DATABASE, VulinBoxTables...)
}
