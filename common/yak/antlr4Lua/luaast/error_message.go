package luaast

//
//func ShowErrorMessage(codeSrc string, err *SyntaxErrorRecord) string {
//	code := strings.Split(codeSrc, "\n")
//	if err == nil {
//		return ``
//	}
//	if err.StartLineNumber < 0 || err.StartLineNumber > len(code) {
//		return `[YakError] ` + err.Message
//	}
//	if err.EndLineNumber < 0 || err.EndLineNumber > len(code) {
//		return `[YakError] ` + err.Message
//	}
//	startLineNumber, endLineNumber := err.StartLineNumber, err.EndLineNumber
//	startColumnNumber, endColumnNumber := err.StartColumnNumber, err.EndColumnNumber
//	displayedErrorHints := make([]string, endLineNumber+1-startLineNumber)
//
//	count := 0
//	prefix := strings.Repeat(" ", 6) + "│" + " "
//	for i := startLineNumber; i <= endLineNumber; i++ {
//		lineCode := code[i-1]
//		displayedErrorHints[count] = fmt.Sprintf("%5d │ %s", i, lineCode) + "\n"
//		if i == startLineNumber {
//			displayedErrorHints[count] += prefix + strings.Repeat(" ", startColumnNumber) + strings.Repeat("^", len(lineCode)-startColumnNumber)
//		} else if i == endLineNumber {
//			displayedErrorHints[count] += prefix + strings.Repeat("^", endColumnNumber)
//		} else {
//			displayedErrorHints[count] += prefix + strings.Repeat("^", len(lineCode))
//		}
//		count++
//	}
//	displayedErrorHint := strings.Trim(strings.Join(displayedErrorHints, "\n"), "\n")
//	if displayedErrorHint == "" {
//		return `[YakError] ` + strings.TrimSpace(err.Message)
//	} else {
//		return `[YakError] ` + strings.TrimSpace(err.Message) + ":\n" + displayedErrorHint
//	}
//}
//func ShowErrorMessageList(source string, errList []*SyntaxErrorRecord) string {
//	var res []string
//	for _, err := range errList {
//		res = append(res, ShowErrorMessage(source, err))
//	}
//	return strings.Join(res, "\n<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<\n")
//}
