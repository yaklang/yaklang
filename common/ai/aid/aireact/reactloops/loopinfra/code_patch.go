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

// ApplyCodePatch applies hunks to fullCode. Every hunk's OldText must match exactly
// once; any miss or ambiguous match fails the whole apply (file unchanged).
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

		matches := findAllSubstrings(fullCode, oldText)
		if len(matches) == 0 {
			return "", utils.Errorf("hunk %d (@@ %s): old text not found.\nPreview:\n%s",
				i+1, h.Header, utils.ShrinkTextBlock(oldText, 300))
		}
		if len(matches) > 1 {
			return "", utils.Errorf("hunk %d (@@ %s): old text matched %d times — enlarge @@ context for uniqueness",
				i+1, h.Header, len(matches))
		}
		start := matches[0]
		plans = append(plans, codePatchPlan{
			start:   start,
			end:     start + len(oldText),
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
