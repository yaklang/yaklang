package filesys

import "strings"

func splitWithSeparator(path string, sep rune) (string, string) {
	if len(path) == 0 {
		return "", ""
	}
	idx := strings.LastIndex(path, string(sep))
	if idx == -1 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

func getExtension(path string) string {
	if len(path) == 0 {
		return ""
	}
	idx := strings.LastIndex(path, ".")
	if idx == -1 {
		return ""
	}
	return path[idx:]
}
