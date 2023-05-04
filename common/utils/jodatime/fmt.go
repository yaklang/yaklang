package jodatime

/*
jodaTime provides a date formatter using the yoda syntax.
http://joda-time.sourceforge.net/apidocs/org/joda/time/format/DateTimeFormat.html
*/

import (
	"strconv"
	"time"
	"unicode/utf8"
)

/*
 Symbol  Meaning                      Presentation  Examples
 ------  -------                      ------------  -------
 G       era                          text          AD
 C       century of era (>=0)         number        20
 Y       year of era (>=0)            year          1996
 x       weekyear                     year          1996
 w       week of weekyear             number        27
 e       day of week                  number        2
 E       day of week                  text          Tuesday; Tue
 y       year                         year          1996
 D       day of year                  number        189
 M       month of year                month         July; Jul; 07
 d       day of month                 number        10
 a       halfday of day               text          PM
 K       hour of halfday (0~11)       number        0
 h       clockhour of halfday (1~12)  number        12
 H       hour of day (0~23)           number        0
 k       clockhour of day (1~24)      number        24
 m       minute of hour               number        30
 s       second of minute             number        55
 S       fraction of second           number        978
 z       time zone                    text          Pacific Standard Time; PST
 Z       time zone offset/id          zone          -0800; -08:00; America/Los_Angeles
 '       escape for text              delimiter
 ''      single quote                 literal       '
*/

// Format formats a date based on joda conventions
func Format(format string, date time.Time) string {
	formatRune := []rune(format)
	lenFormat := len(formatRune)
	out := make([]byte, 0, 24)
	for i := 0; i < len(formatRune); i++ {
		switch r := formatRune[i]; r {
		case 'Y', 'y', 'x': // Y YYYY YY year

			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1

			switch j {
			case 1, 3, 4: // Y YYY YYYY
				out = strconv.AppendInt(out, int64(date.Year()), 10)
			case 2: // YY
				year := int64(date.Year() % 100)
				if year < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(year), 10)
			}

		case 'D': // D DD day of year
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1

			switch j {
			case 1: // D
				out = strconv.AppendInt(out, int64(date.YearDay()), 10)
			case 2: // DD
				yearDay := date.YearDay()
				if yearDay < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(yearDay), 10)
			}

		case 'w': // w ww week of weekyear
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			_, w := date.ISOWeek()
			switch j {
			case 1: // w
				out = strconv.AppendInt(out, int64(w), 10)
			case 2: // ww
				if w < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(w), 10)
			}

		case 'M': // M MM MMM MMMM month of year
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Month()
			switch j {
			case 1: // M
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // MM
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			case 3: // MMM
				out = append(out, v.String()[0:3]...)
			case 4: // MMMM
				out = append(out, v.String()...)
			}

		case 'd': // d dd day of month
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Day()
			switch j {
			case 1: // d
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // dd
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			}

		case 'e': // e ee day of week(number)
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Weekday()
			switch j {
			case 1: // e
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // ee
				out = append(out, '0')
				out = strconv.AppendInt(out, int64(v), 10)
			}

		case 'E': // E EE
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Weekday()
			switch j {
			case 1, 2, 3: // E
				out = append(out, v.String()[0:3]...)
			case 4: // EE
				out = append(out, v.String()...)
			}
		case 'h': // h hh clockhour of halfday (1~12)
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Hour()
			if v > 12 {
				v = v - 12
			} else if v == 0 {
				v = 12
			}

			switch j {
			case 1: // h
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // hh
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			}

		case 'H': // H HH
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Hour()
			switch j {
			case 1: // H
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // HH
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			}

		case 'a': // a
			if date.Hour() > 12 {
				out = append(out, "PM"...)
			} else {
				out = append(out, "AM"...)
			}

		case 'm': // m mm minute of hour
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Minute()
			switch j {
			case 1: // m
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // mm
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			}
		case 's': // s ss
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Second()
			switch j {
			case 1: // s
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // ss
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			}

		case 'S': // S SS SSS
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}
			i = i + j - 1
			v := date.Nanosecond() / 1000000
			switch j {
			case 1: // S
				out = strconv.AppendInt(out, int64(v/100), 10)
			case 2: // SS
				v = v / 10
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			case 3: // SSS
				if v < 10 {
					out = append(out, "00"...)
				} else if v < 100 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			}

		case 'z': // z
			z, _ := date.Zone()
			out = append(out, z...)

		case 'Z': // Z ZZ
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			zs, z := date.Zone()
			sign := "+"
			if z < 0 {
				sign = "-"
				z = -z
			}

			v := z / 3600

			switch j {
			case 1: // Z
				out = append(out, sign...)
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
				out = append(out, "00"...)

			case 2: // ZZ
				out = append(out, sign...)
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
				out = append(out, ":00"...)

			case 3: // ZZZ
				if tz, ok := timeZone[zs]; ok {
					out = append(out, tz...)
				} else {
					// not in alias table, append short tz name
					out = append(out, tz...)
				}
			}

		case 'G': //era                          text
			out = append(out, "AD"...)

		case 'C': //century of era (>=0)         number
			out = append(out, strconv.Itoa(date.Year())[0:2]...)

		case 'K': // K KK hour of halfday (0~11)
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Hour()
			if v >= 12 {
				v = v - 12
			}

			switch j {
			case 1: // K
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // KK
				if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			}

		case 'k': // k kk clockhour of day (1~24)
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}

			}
			i = i + j - 1
			v := date.Hour()
			switch j {
			case 1: // k
				if v == 0 {
					v = 24
				}
				out = strconv.AppendInt(out, int64(v), 10)
			case 2: // kk
				if v == 0 {
					v = 24
				} else if v < 10 {
					out = append(out, '0')
				}
				out = strconv.AppendInt(out, int64(v), 10)
			}
		case '\'': // ' (text delimiter)  or '' (real quote)

			// real quote
			if formatRune[i+1] == r {
				out = append(out, '\'')
				i = i + 1
				continue
			}

			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					out = utf8.AppendRune(out, formatRune[i+j])
					continue
				}
				break
			}
			i = i + j
		default:
			out = utf8.AppendRune(out, r)
		}
	}
	return UnsafeString(out)

}

// TODO: refactor timezone
var timeZone = map[string]string{
	"GMT+8":   "Asia/Shanghai",
	"GMT+9":   "Asia/Tokyo",
	"GMT":     "Europe/London",
	"BST":     "Europe/London",
	"BSDT":    "Europe/London",
	"CET":     "Europe/Paris",
	"UTC":     "",
	"PST":     "America/Los_Angeles",
	"PDT":     "America/Los_Angeles",
	"LA":      "America/Los_Angeles",
	"LAX":     "America/Los_Angeles",
	"MST":     "America/Denver",
	"MDT":     "America/Denver",
	"CST":     "America/Chicago",
	"CDT":     "America/Chicago",
	"Chicago": "America/Chicago",
	"EST":     "America/New_York",
	"EDT":     "America/New_York",
	"NYC":     "America/New_York",
	"NY":      "America/New_York",
	"AEST":    "Australia/Sydney",
	"AEDT":    "Australia/Sydney",
	"AWST":    "Australia/Perth",
	"AWDT":    "Australia/Perth",
	"ACST":    "Australia/Adelaide",
	"ACDT":    "Australia/Adelaide",
}
