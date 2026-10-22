package damenggorm

import "strings"

// NormalizeQuery applies SSA's uppercase object policy to its owned native SQL
// templates. It is not a driver hook: values, comments and bound data are not
// rewritten by the connection layer.
func NormalizeQuery(query string) string {
	var out strings.Builder
	for i := 0; i < len(query); {
		start := i
		if query[i] == '\'' || query[i] == '"' {
			quote := query[i]
			i++
			for i < len(query) {
				if query[i] == quote {
					i++
					if i < len(query) && query[i] == quote {
						i++
						continue
					}
					break
				}
				i++
			}
			part := query[start:i]
			if quote == '"' {
				part = strings.ToUpper(part)
			}
			out.WriteString(part)
			continue
		}
		if strings.HasPrefix(query[i:], "--") {
			for i < len(query) && query[i] != '\n' {
				i++
			}
			out.WriteString(query[start:i])
			continue
		}
		if strings.HasPrefix(query[i:], "/*") {
			i += 2
			for i < len(query) && !strings.HasPrefix(query[i:], "*/") {
				i++
			}
			if i < len(query) {
				i += 2
			}
			out.WriteString(query[start:i])
			continue
		}
		out.WriteByte(query[i])
		i++
	}
	return out.String()
}
