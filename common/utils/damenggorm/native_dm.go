//go:build dameng

package damenggorm

import (
	"context"
	"database/sql/driver"
	"net"

	dm "gitee.com/chunanyong/dm"
)

// This is the only production file that imports the upstream native driver.
// In particular, its optional Linux/macOS cipher plugin loader must not change
// linking requirements for ordinary SQLite/PostgreSQL engine builds.
func init() {
	nativeDriver.matches = func(d driver.Driver) bool {
		_, ok := d.(*dm.DmDriver)
		return ok
	}
	nativeDriver.registerDial = func(name string, dial func(context.Context, string) (net.Conn, error)) {
		dm.RegisterDialContext(name, dial)
	}
}
