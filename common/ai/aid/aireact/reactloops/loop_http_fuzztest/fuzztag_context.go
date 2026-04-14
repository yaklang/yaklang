package loop_http_fuzztest

import (
	_ "embed"
	"fmt"
	"html"
	"sort"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	loopHTTPFuzzFuzztagReferenceKey       = "fuzztag_reference"
	loopHTTPFuzzPayloadGroupsReferenceKey = "payload_groups_reference"
)

var (
	loopHTTPFuzzFuzztagReferenceOnce sync.Once
	loopHTTPFuzzFuzztagReferenceText string
)

func bootstrapLoopHTTPFuzzFuzztagContext(loopStateSetter interface{ Set(string, any) }, db *gorm.DB) {
	if loopStateSetter == nil {
		return
	}
	loopStateSetter.Set(loopHTTPFuzzFuzztagReferenceKey, getLoopHTTPFuzzFuzztagReference())
	loopStateSetter.Set(loopHTTPFuzzPayloadGroupsReferenceKey, buildLoopHTTPFuzzPayloadGroupsReference(db))
}

func getLoopHTTPFuzzFuzztagReference() string {
	loopHTTPFuzzFuzztagReferenceOnce.Do(func() {
		loopHTTPFuzzFuzztagReferenceText = loadLoopHTTPFuzzFuzztagReference()
	})
	return loopHTTPFuzzFuzztagReferenceText
}

//go:embed prompts/fuzztag.md
var fuzztagReferenceData []byte

func loadLoopHTTPFuzzFuzztagReference() string {
	content := strings.TrimSpace(html.UnescapeString(string(fuzztagReferenceData)))
	if content == "" {
		return ""
	}
	var out strings.Builder
	out.WriteString("This is the current built-in fuzztag reference. You may use any relevant fuzztag listed here, not only a small subset of examples.\n")
	out.WriteString("When you need batch generation for path/query/header/body/cookie/auth values, prefer writing a compact fuzztag rule instead of manually expanding all values.\n\n")
	out.WriteString(content)
	return out.String()
}

func buildLoopHTTPFuzzPayloadGroupsReference(db *gorm.DB) string {
	var out strings.Builder
	out.WriteString("Payload dictionary tags are available in these forms:\n")
	out.WriteString("- {{payload(group)}}: line-based expansion with deduplication\n")
	out.WriteString("- {{payload:nodup(group)}}: line-based expansion without deduplication\n")
	out.WriteString("- {{payload:full(group)}}: return the full block without splitting lines\n")

	if db == nil {
		out.WriteString("\nCurrent database payload groups are unavailable because DB is not configured.")
		return out.String()
	}

	groups, err := yakit.GetAllPayloadGroupName(db)
	if err != nil {
		log.Warnf("failed to query payload group names: %v", err)
		out.WriteString("\nCurrent database payload groups are unavailable because the query failed.")
		return out.String()
	}

	sort.Strings(groups)
	if len(groups) == 0 {
		out.WriteString("\nCurrent database payload groups: (none)")
		return out.String()
	}

	out.WriteString(fmt.Sprintf("\nCurrent database payload groups (%d):\n", len(groups)))
	for _, group := range groups {
		if strings.TrimSpace(group) == "" {
			continue
		}
		out.WriteString("- ")
		out.WriteString(group)
		out.WriteByte('\n')
	}
	return strings.TrimSpace(out.String())
}
