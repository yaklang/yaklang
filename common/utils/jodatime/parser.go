package jodatime

import (
	"time"
	"unicode/utf8"
	"unsafe"
)

func ParseInLocation(format, value, timezone string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation(GetLayout(format), value, location)

}

func ParseInLocationDirect(format, value string, timezone *time.Location) (time.Time, error) {
	return time.ParseInLocation(GetLayout(format), value, timezone)
}

// Parse parses a value into a time.time
func Parse(format, value string) (time.Time, error) {
	return time.Parse(GetLayout(format), value)
}

// GetLayout convert JodaTime layout to golang stdlib time layout
func GetLayout(format string) string {
	//replace ? or for rune ?
	formatRune := []rune(format)
	lenFormat := len(formatRune)
	layout := make([]byte, 0, 9)
	for i := 0; i < lenFormat; i++ {
		switch r := formatRune[i]; r {
		case 'h':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}
			switch j {
			case 1: // d
				layout = append(layout, '3')
			default:
				layout = append(layout, "03"...)
			}

			i = i + j - 1
		case 'H':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}

			layout = append(layout, "15"...)

			i = i + j - 1
		case 'm':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}
			switch j {
			case 1: // d
				layout = append(layout, '4')
			default:
				layout = append(layout, "04"...)
			}

			i = i + j - 1
		case 's':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}
			switch j {
			case 1: // d
				layout = append(layout, '5')
			default:
				layout = append(layout, "05"...)
			}

			i = i + j - 1
		case 'd':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}
			switch j {
			case 1: // d
				layout = append(layout, '2')
			default:
				layout = append(layout, "02"...)
			}
			i = i + j - 1
		case 'E':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}
			switch j {
			case 1, 2, 3: // d
				layout = append(layout, "Mon"...)
			default:
				layout = append(layout, "Monday"...)
			}
			i = i + j - 1
		case 'M':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}

			switch j {
			case 1: // d
				layout = append(layout, '1')
			case 2:
				layout = append(layout, "01"...)
			case 3:
				layout = append(layout, "Jan"...)
			case 4:
				layout = append(layout, "January"...)
			}
			i = i + j - 1

		case 'Y', 'y', 'x':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}
			switch j {
			case 2: // d
				layout = append(layout, "06"...)
			default: // dd
				layout = append(layout, "2006"...)
			}

			i = i + j - 1

		case 'S':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}

			for i := 0; i < j; i++ {
				layout = append(layout, '9')
			}

			i = i + j - 1

		case 'a':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}

			layout = append(layout, "PM"...)
		case 'Z':
			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					break
				}
			}

			switch j {
			case 1: // d
				layout = append(layout, "-0700"...)
			case 2: // d
				layout = append(layout, "-07:00"...)
			}

			i = i + j - 1
		case '\'': // ' (text delimiter)  or '' (real quote)

			// real quote
			if formatRune[i+1] == r {
				layout = append(layout, '\'')
				i = i + 1
				continue
			}

			j := 1
			for ; i+j < lenFormat; j++ {
				if formatRune[i+j] != r {
					layout = utf8.AppendRune(layout, formatRune[i+j])
					continue
				}
				break
			}
			i = i + j

		default:
			layout = utf8.AppendRune(layout, r)
		}
	}
	return UnsafeString(layout)

}

// UnsafeString returns the string under byte buffer
func UnsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
