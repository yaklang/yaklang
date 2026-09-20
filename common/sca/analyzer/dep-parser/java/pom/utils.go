package pom

import (
	"fmt"
	"strings"
)

func isProperty(version string) bool {
	if version != "" && strings.HasPrefix(version, "${") && strings.HasSuffix(version, "}") {
		return true
	}
	return false
}

func packageID(name, version string) string {
	return fmt.Sprintf("%s:%s", name, version)
}
