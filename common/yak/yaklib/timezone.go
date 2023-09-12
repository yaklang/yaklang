package yaklib

import (
	"time"
)

var TimeZoneExports = map[string]interface{}{
	"Get": time.LoadLocation,
	"Now": func(i string) time.Time {
		loc, err := time.LoadLocation(i)
		if err != nil {
			return time.Now()
		}
		return time.Now().In(loc)
	},
}
