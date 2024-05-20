package cveresources

import (
	"github.com/yaklang/yaklang/common/consts"
)

var CVETbles = []any{
	&CVE{}, &CWE{},
	&ProductsTable{},
}

func init() {
	consts.RegisterDatabaseSchema(consts.KEY_SCHEMA_CVE_DATABASE, CVETbles...)
}
