package sharkcli

// Order already decoded HTTP/1 wrappers as their start line, headers and body.
// This changes presentation only; it neither parses bytes nor grants ranges.
func http1MessageDetailPriority(name, firstLineKind, key string) (int, bool) {
	var order []string
	switch name {
	case "HTTP Request", "HTTP Response":
		order = []string{"FirstLine", "Headers", "Body", "Trailers"}
	case "FirstLine":
		switch firstLineKind {
		case "HTTP Request":
			order = []string{"Method", "Path", "Version"}
		case "HTTP Response":
			order = []string{"Version", "Status", "Message"}
		}
	}
	if order == nil {
		return 0, false
	}
	for rank, field := range order {
		if key == field {
			return rank, true
		}
	}
	// Preserve unfamiliar observations after the defined message components.
	return 1000 + fieldPriority(key), true
}
