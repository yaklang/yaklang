package cve

import (
	"github.com/yaklang/yaklang/common/consts"
)

var CVEDescriptionTables = []any{
	&CVEDescription{},
}

func init() {
	consts.RegisterDatabaseSchema(consts.KEY_SCHEMA_CVE_DESCRIPTION_DATABASE, CVEDescriptionTables...)
}
