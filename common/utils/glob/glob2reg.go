package glob

type Options struct {
	Extended  bool
	GlobStar  bool
	Delimiter rune
}

// based on https://stackoverflow.com/a/400316/83005
func escapeDelimiter(delimiter rune) (outsideClass, insideClass string) {
	switch delimiter {
	case '^', '\\':
		return "\\" + string(delimiter), "\\" + string(delimiter)
	case '.', '$', '*', '+', '?', '(', ')', '[', '{', '|':
		return "\\" + string(delimiter), string(delimiter)
	case '-', ']':
		return string(delimiter), "\\" + string(delimiter)
	default:
		return string(delimiter), string(delimiter)
	}
}

// Glob2Regex converts the glob string into a regexp string with options "extended" & "glob-star" enabled, and with
// the default delimiter '/'
func Glob2Regex(glob string) string {
	return Glob2RegexWithOptions(glob, Options{
		Extended:  true,
		GlobStar:  false,
		Delimiter: '/',
	})
}

// Glob2RegexWithOptions converts glob to regexp string with the given options
func Glob2RegexWithOptions(glob string, config Options) string {
	reStr := ""

	delimiter := '/'
	if config.Delimiter != 0 {
		delimiter = config.Delimiter
	}

	delimiterOutsideClass, delimiterInsideClass := escapeDelimiter(delimiter)

	inGroup := false

	globLen := len(glob)

	for i := 0; i < globLen; i++ {
		c := glob[i]

		switch c {
		case '/', '$', '^', '+', '.', '(', ')', '=', '!', '|':
			reStr += "\\" + string(c)

		case '?':
			if config.Extended {
				reStr += "."
				break
			}

			reStr += "\\" + string(c)

		case '[', ']':
			if config.Extended {
				reStr += string(c)
				break
			}

			reStr += "\\" + string(c)

		case '{':
			if config.Extended {
				inGroup = true
				reStr += "("
				break
			}

			reStr += "\\" + string(c)

		case '}':
			if config.Extended {
				inGroup = false
				reStr += ")"
				break
			}

			reStr += "\\" + string(c)

		case ',':
			if inGroup {
				reStr += "|"
				break
			}

			reStr += "\\" + string(c)

		case '*':
			// Move over all consecutive "*"'s.
			// Also store the previous and next characters
			var nextChar, prevChar rune
			if i > 0 {
				prevChar = rune(glob[i-1])
			}
			starCount := 1
			for i < globLen-1 && glob[i+1] == '*' {
				starCount++
				i++
			}

			if i < globLen-1 {
				nextChar = rune(glob[i+1])
			}

			if !config.GlobStar {
				// globstar is disabled, so treat any number of "*" as one
				reStr += ".*"
			} else {
				// globstar is enabled, so determine if this is a globstar segment
				isGlobstar := starCount > 1 && // multiple "*"'s
					(prevChar == delimiter || prevChar == 0) && // from the start of the segment
					(nextChar == delimiter || nextChar == 0) // to the end of the segment

				if isGlobstar {
					// it's a globstar, so match zero or more path segments
					reStr += "(?:(?:[^" + delimiterInsideClass + "]*(?:" + delimiterOutsideClass + "|$))*)"
					i++ // move over the delimiter
				} else {
					// it's not a globstar, so only match one path segment
					reStr += "(?:[^" + delimiterInsideClass + "]*)"
				}
			}

		default:
			reStr += string(c)
		}
	}

	return "^" + reStr + "$"
}
