package cveresources

import (
	"github.com/yaklang/yaklang/common/schema"
)

var CVETbles = []any{
	&CVE{}, &CWE{},
	&ProductsTable{},
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_CVE_DATABASE, CVETbles...)
}
