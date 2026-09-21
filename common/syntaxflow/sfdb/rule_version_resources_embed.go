//go:build !gzip_embed && !irify_exclude

package sfdb

import "embed"

//go:embed rule_versions.json
var ruleVersionFS embed.FS
