package utils

import (
	"yaklang/common/utils/jodatime"
	"time"
)

func JavaTimeFormatter(t time.Time, formatter string) string {
	return jodatime.Format(formatter, t)
}
