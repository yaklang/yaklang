// Package embedded demonstrates building an image once and loading it without
// regex compilation at program startup. Applications should supply their own
// ordered rules and regenerate the image whenever those rules change.
package embedded

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/minirehs"
)

//go:generate go run ../../tools/rehs-image -patterns patterns.json -out rules.rehs

//go:embed rules.rehs
var rulesImage []byte

// Load returns an independently owned database. An invalid bundled image is a
// startup error; it does not fall back to runtime compilation or a disk cache.
func Load() (minirehs.Database, error) { return minirehs.LoadImage(rulesImage) }
