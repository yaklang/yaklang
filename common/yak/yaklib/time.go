package yaklib

import (
	"time"
	"yaklang.io/yaklang/common/utils"
)

var TimeExports = map[string]interface{}{
	"Now":              time.Now,
	"now":              time.Now,
	"GetCurrentMonday": utils.GetCurrentWeekMonday,
	"GetCurrentDate":   utils.GetCurrentDate,
	"sleep":            sleep,
	"Sleep":            sleep,
	"Parse":            time.Parse,
	"ParseDuration":    time.ParseDuration,
	"Unix":             time.Unix,
	"After": func(i float64) <-chan time.Time {
		return time.After(utils.FloatSecondDuration(i))
	},
	"AfterFunc": time.AfterFunc,
	"NewTimer": func(i float64) *time.Timer {
		return time.NewTimer(utils.FloatSecondDuration(i))
	},
	"NewTicker": func(i float64) *time.Ticker {
		return time.NewTicker(utils.FloatSecondDuration(i))
	},
	"Until": time.Until,
	"Since": time.Since,
}

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
