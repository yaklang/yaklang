package cve

import (
	"github.com/yaklang/yaklang/common/schema"
)

var CVEDescriptionTables = []any{
	&CVEDescription{},
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_CVE_DESCRIPTION_DATABASE, CVEDescriptionTables...)
}
