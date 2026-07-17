package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	codePatchBeginMarker = "*** Begin Patch"
	codePatchEndMarker   = "*** End Patch"
	codePatchUpdateFile  = "*** Update File:"
	codePatchAddFile     = "*** Add File:"
	codePatchDeleteFile  = "*** Delete File:"
)

// CodePatchHunk is one @@-delimited change block inside a Cursor-style Apply Patch.
type CodePatchHunk struct {
	Header  string // @@ line without leading @@ (may be empty)
	OldText string // context + deleted lines (exact match haystack)
	NewText string // context + added lines (replacement)
}

// LooksLikeCodePatch reports whether s contains a Cursor-style Begin Patch marker.
func LooksLikeCodePatch(s string) bool {
	return strings.Contains(s, codePatchBeginMarker)
}

// ParseCodePatch parses a Cursor-style Apply Patch body into hunks.
// "*** Update File:" paths are ignored; callers always apply against the loop's full_code.
func ParseCodePatch(s string) ([]CodePatchHunk, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, utils.Error("empty patch")
	}
	if !LooksLikeCodePatch(s) {
		return nil, utils.Error("not a code patch: missing *** Begin Patch")
	}

	begin := strings.Index(s, codePatchBeginMarker)
	body := s[begin+len(codePatchBeginMarker):]
	if end := strings.Index(body, codePatchEndMarker); end >= 0 {
		body = body[:end]
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, utils.Error("empty patch body")
	}

	lines := splitPatchLines(body)
	var hunks []CodePatchHunk
	var cur *CodePatchHunk
	var oldLines, newLines []string
	seenFileHeader := false

	flush := func() {
		if cur == nil {
			return
		}
		cur.OldText = joinPatchLines(oldLines)
		cur.NewText = joinPatchLines(newLines)
		if cur.OldText == "" && cur.NewText == "" {
			cur = nil
			oldLines, newLines = nil, nil
			return
		}
		hunks = append(hunks, *cur)
		cur = nil
		oldLines, newLines = nil, nil
	}

	startHunk := func(header string) {
		flush()
		cur = &CodePatchHunk{Header: strings.TrimSpace(header)}
		oldLines, newLines = nil, nil
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			if cur != nil {
				// Preserve blank lines inside an active hunk as context.
				oldLines = append(oldLines, "")
				newLines = append(newLines, "")
			}
		case strings.HasPrefix(trimmed, codePatchUpdateFile) ||
			strings.HasPrefix(trimmed, codePatchAddFile) ||
			strings.HasPrefix(trimmed, codePatchDeleteFile):
			flush()
			seenFileHeader = true
		case strings.HasPrefix(trimmed, "@@"):
			header := strings.TrimSpace(strings.TrimPrefix(trimmed, "@@"))
			startHunk(header)
		case cur == nil && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "+")):
			// Hunk without @@ header — start an anonymous hunk.
			startHunk("")
			fallthrough
		case cur != nil:
			if len(line) == 0 {
				continue
			}
			prefix := line[0]
			content := ""
			if len(line) > 1 {
				content = line[1:]
				// Cursor patch lines are " "+content / "-"+content / "+"+content.
				// If the model omitted the leading space after the marker, keep content as-is.
			} else {
				content = ""
			}
			switch prefix {
			case ' ':
				oldLines = append(oldLines, content)
				newLines = append(newLines, content)
			case '-':
				oldLines = append(oldLines, content)
			case '+':
				newLines = append(newLines, content)
			default:
				// Treat unmarked lines inside a hunk as context (tolerant of model drift).
				oldLines = append(oldLines, line)
				newLines = append(newLines, line)
			}
		default:
			// Ignore stray text before first hunk / file header.
			_ = seenFileHeader
		}
	}
	flush()

	if len(hunks) == 0 {
		return nil, utils.Error("patch contains no hunks")
	}
	return hunks, nil
}

// ApplyCodePatch applies hunks to fullCode. Every hunk's OldText must match
// exactly once. If byte-for-byte matching misses, a safe normalized match is
// attempted (line endings and trailing horizontal whitespace only).
func ApplyCodePatch(fullCode string, hunks []CodePatchHunk) (string, error) {
	if len(hunks) == 0 {
		return "", utils.Error("no hunks to apply")
	}

	plans := make([]codePatchPlan, 0, len(hunks))

	for i, h := range hunks {
		oldText := h.OldText
		newText := h.NewText

		// Pure insertion hunk: OldText empty means insert NewText.
		// We require at least some OldText for location, except empty-file case.
		if oldText == "" {
			if fullCode == "" {
				plans = append(plans, codePatchPlan{start: 0, end: 0, newText: newText, hunkIdx: i})
				continue
			}
			return "", utils.Errorf("hunk %d (@@ %s): empty old text — add context/- lines to locate the insert", i+1, h.Header)
		}

		matches := findCodeMatchRanges(fullCode, oldText)
		if len(matches) == 0 {
			return "", utils.Errorf("hunk %d (@@ %s): old text not found.\nPreview:\n%s",
				i+1, h.Header, utils.ShrinkTextBlock(oldText, 300))
		}
		if len(matches) > 1 {
			return "", utils.Errorf("hunk %d (@@ %s): old text matched %d times — enlarge @@ context for uniqueness",
				i+1, h.Header, len(matches))
		}
		match := matches[0]
		plans = append(plans, codePatchPlan{
			start:   match.start,
			end:     match.end,
			newText: newText,
			hunkIdx: i,
		})
	}

	// Apply from end to start so earlier offsets stay valid.
	sortPlansByStartDesc(plans)
	result := fullCode
	for _, p := range plans {
		result = result[:p.start] + p.newText + result[p.end:]
	}
	return result, nil
}

// ApplyCodePatchFromString parses then applies a patch body onto fullCode.
func ApplyCodePatchFromString(fullCode, patchBody string) (string, error) {
	hunks, err := ParseCodePatch(patchBody)
	if err != nil {
		return "", err
	}
	return ApplyCodePatch(fullCode, hunks)
}

// SummarizeAppliedPatch returns a short, non-patch summary of applied changes for editor emit.
func SummarizeAppliedPatch(hunks []CodePatchHunk) string {
	if len(hunks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("applied %d patch hunk(s)\n", len(hunks)))
	for i, h := range hunks {
		header := h.Header
		if header == "" {
			header = "(no @@ header)"
		}
		b.WriteString(fmt.Sprintf("--- hunk %d: @@ %s\n", i+1, header))
		if h.NewText != "" {
			b.WriteString(utils.ShrinkTextBlock(h.NewText, 200))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func splitPatchLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

func joinPatchLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func findAllSubstrings(haystack, needle string) []int {
	if needle == "" {
		return nil
	}
	var out []int
	start := 0
	for {
		i := strings.Index(haystack[start:], needle)
		if i < 0 {
			break
		}
		abs := start + i
		out = append(out, abs)
		start = abs + 1
		if start >= len(haystack) {
			break
		}
	}
	return out
}

type codeTextRange struct {
	start int
	end   int
}

// findCodeMatchRanges first uses exact byte matching. Only when there are no
// exact matches does it normalize CRLF/CR to LF and ignore trailing spaces/tabs.
// The normalized offsets are mapped back to the original source so replacements
// preserve all text outside the matched range.
func findCodeMatchRanges(fullCode, oldText string) []codeTextRange {
	exact := findAllSubstrings(fullCode, oldText)
	if len(exact) > 0 {
		ranges := make([]codeTextRange, 0, len(exact))
		for _, start := range exact {
			ranges = append(ranges, codeTextRange{start: start, end: start + len(oldText)})
		}
		return ranges
	}

	normalizedFull, offsets := normalizeCodeForMatchWithOffsets(fullCode)
	normalizedOld := normalizeCodeForMatch(oldText)
	if normalizedOld == "" {
		return nil
	}
	normalizedMatches := findAllSubstrings(normalizedFull, normalizedOld)
	ranges := make([]codeTextRange, 0, len(normalizedMatches))
	for _, start := range normalizedMatches {
		end := start + len(normalizedOld)
		if start < 0 || end >= len(offsets) {
			continue
		}
		ranges = append(ranges, codeTextRange{start: offsets[start], end: offsets[end]})
	}
	return ranges
}

func normalizeCodeForMatch(s string) string {
	normalized, _ := normalizeCodeForMatchWithOffsets(s)
	return normalized
}

// normalizeCodeForMatchWithOffsets returns normalized code and a boundary map:
// offsets[i] is the exclusive original end after consuming normalized[:i].
func normalizeCodeForMatchWithOffsets(s string) (string, []int) {
	var b strings.Builder
	// offsets[i] is the exclusive original end after consuming normalized[:i].
	offsets := []int{0}

	for i := 0; i < len(s); {
		lineStart := i
		for i < len(s) && s[i] != '\r' && s[i] != '\n' {
			i++
		}
		lineEnd := i
		trimmedEnd := lineEnd
		for trimmedEnd > lineStart && (s[trimmedEnd-1] == ' ' || s[trimmedEnd-1] == '\t') {
			trimmedEnd--
		}
		wroteContent := trimmedEnd > lineStart
		for j := lineStart; j < trimmedEnd; j++ {
			b.WriteByte(s[j])
			offsets = append(offsets, j+1)
		}
		// Ending a match at this line's normalized EOL should consume trailing spaces.
		if wroteContent {
			offsets[len(offsets)-1] = lineEnd
		}

		if i >= len(s) {
			break
		}
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			i += 2
		} else {
			i++
		}
		b.WriteByte('\n')
		offsets = append(offsets, i)
	}

	return b.String(), offsets
}

type codePatchPlan struct {
	start, end int
	newText    string
	hunkIdx    int
}

func sortPlansByStartDesc(plans []codePatchPlan) {
	for i := 0; i < len(plans); i++ {
		for j := i + 1; j < len(plans); j++ {
			if plans[j].start > plans[i].start {
				plans[i], plans[j] = plans[j], plans[i]
			}
		}
	}
}
