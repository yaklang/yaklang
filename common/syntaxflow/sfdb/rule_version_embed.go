//go:build !irify_exclude

package sfdb

import _ "embed"

//go:embed rule_versions.json
var ruleVersions []byte
