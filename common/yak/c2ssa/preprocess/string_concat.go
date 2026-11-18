package preprocess

import "strings"

// concatAdjacentStringLiterals implements translation phase 6: adjacent string
// literal tokens are concatenated, and the whitespace or comments between them
// are dropped. Raw string literals are left alone, because joining their source
// text would change the resulting contents.
func concatAdjacentStringLiterals(tokens []macroToken) []macroToken {
	var out []macroToken
	for i := 0; i < len(tokens); i++ {
		if tokens[i].kind != macroTokString || stringTokenIsRaw(tokens, i) {
			out = append(out, tokens[i])
			continue
		}
		merged := tokens[i]
		next := i + 1
		for {
			j := next
			for j < len(tokens) && (isMacroWhitespaceToken(tokens[j]) || tokens[j].kind == macroTokComment) {
				j++
			}
			if j >= len(tokens) || tokens[j].kind != macroTokString || stringTokenIsRaw(tokens, j) {
				break
			}
			merged = mergeStringTokens(merged, tokens[j])
			next = j + 1
		}
		out = append(out, merged)
		i = next - 1
	}
	return out
}

// stringTokenIsRaw reports whether tokens[index] belongs to a raw string
// literal. The tokenizer emits R"(text)" as an identifier followed by a string
// token, so the prefix can be either inside the token or in the token before it.
func stringTokenIsRaw(tokens []macroToken, index int) bool {
	if index < 0 || index >= len(tokens) {
		return false
	}
	prefix, _ := splitStringLiteralToken(tokens[index].text)
	if isRawStringPrefix(prefix) {
		return true
	}
	for i := index - 1; i >= 0; i-- {
		if isMacroWhitespaceToken(tokens[i]) || tokens[i].kind == macroTokComment {
			continue
		}
		return isRawStringPrefix(tokens[i].text)
	}
	return false
}

func isRawStringPrefix(text string) bool {
	switch text {
	case "R", "LR", "uR", "UR", "u8R":
		return true
	}
	return false
}

// mergeStringTokens joins two ordinary string literals. The encoding prefix of
// the left literal wins; if it has none, the right prefix is used.
func mergeStringTokens(left, right macroToken) macroToken {
	leftPrefix, leftBody := splitStringLiteralToken(left.text)
	rightPrefix, rightBody := splitStringLiteralToken(right.text)
	prefix := leftPrefix
	if prefix == "" {
		prefix = rightPrefix
	}
	merged := prefix + leftBody[:len(leftBody)-1] + rightBody[1:]
	return macroToken{kind: macroTokString, text: merged}
}

func splitStringLiteralToken(text string) (prefix, body string) {
	quote := strings.IndexByte(text, '"')
	if quote < 0 {
		return text, ""
	}
	return text[:quote], text[quote:]
}

// ConcatAdjacentStringLiterals applies translation phase 6 to src.
func ConcatAdjacentStringLiterals(src string) string {
	return tokensToString(concatAdjacentStringLiterals(tokenizeMacroSource(src)))
}
