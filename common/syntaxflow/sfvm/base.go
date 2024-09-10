package sfvm

import (
	"regexp"
)

var BinOpRegexp = regexp.MustCompile(`(?i)([A-Za-z_-]+)\[([\w\S+=*/]{1,3})]`)