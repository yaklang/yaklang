package reactloops

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

type AttachedFileResourceHandler func(r aicommon.AIInvokeRuntime, loop *ReActLoop, resources []*aicommon.AttachedFileResourceData)

var attachedFileResourceHandlers AttachedFileResourceHandler

func RegisterAttachedFileResourceHandler(handler AttachedFileResourceHandler) {
	if handler == nil {
		return
	}
	attachedFileResourceHandlers = handler
}

// RunAttachedExtraResourcesInit parses attached resources through the structured
// resource registry, lets each resource bind loop-specific data, then groups
// rendered attachment content by resource type into the invoker timeline.
func RunAttachedExtraResourcesInit(
	r aicommon.AIInvokeRuntime,
	loop *ReActLoop,
	attachedDatas []*aicommon.AttachedResource,
) []aicommon.AttachedResourceData {
	if len(attachedDatas) == 0 || r == nil || loop == nil {
		return nil
	}

	sectionsByType := make(map[string][]string)
	resourcesByType := make(map[string][]aicommon.AttachedResourceData)
	var resources []aicommon.AttachedResourceData

	for idx, data := range attachedDatas {
		if data == nil {
			continue
		}
		loop.LoadingStatus(fmt.Sprintf(
			"正在加载附加资源 (%d) / Loading attached resource (%d)",
			idx+1, idx+1,
		))

		resource, err := aicommon.ParseAttachedResourceData(data)
		if err != nil {
			log.Warnf("attached extra resources: failed to parse type=%q value=%q: %v", strings.TrimSpace(data.Type), strings.TrimSpace(data.Value), err)
			continue
		}
		resources = append(resources, resource)
		resourcesByType[resource.Type()] = append(resourcesByType[resource.Type()], resource)
		if err := resource.BindLoopData(loop); err != nil {
			log.Warnf("attached extra resources: failed to bind type=%q: %v", resource.Type(), err)
			continue
		}
		rendered := strings.TrimSpace(resource.ToAttachData(loop))
		if rendered == "" {
			continue
		}
		sectionsByType[resource.Type()] = append(sectionsByType[resource.Type()], rendered)
	}

	for typ, sections := range sectionsByType {
		if len(sections) == 0 {
			continue
		}
		timelineKey := attachedResourceTimelineKey(typ)
		if typ == aicommon.AttachedResourceTypeFile {
			runAttachedFileResourceHandlers(r, loop, resourcesByType[typ])
		}
		r.AddToTimeline(timelineKey, strings.Join(sections, "\n\n---\n\n"))
		r.AddToTimeline("import notice", fmt.Sprintf("%s has been recorded; use it when reasoning about the user's attached resources.", timelineKey))
	}

	if len(sectionsByType) > 0 {
		loop.LoadingStatus("附加资源处理完成 / Finished loading attached resources")
	}
	return resources
}

func attachedResourceTimelineKey(typ string) string {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		typ = aicommon.AttachedResourceTypeDefault
	}

	var b strings.Builder
	b.WriteString("attached_")
	lastUnderscore := false
	for _, r := range strings.ToLower(typ) {
		switch {
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}

func runAttachedFileResourceHandlers(
	r aicommon.AIInvokeRuntime,
	loop *ReActLoop,
	resources []aicommon.AttachedResourceData,
) {
	if attachedFileResourceHandlers == nil {
		return
	}
	var fileResources []*aicommon.AttachedFileResourceData
	for _, resource := range resources {
		fileResource, ok := resource.(*aicommon.AttachedFileResourceData)
		if !ok || fileResource == nil {
			continue
		}
		fileResources = append(fileResources, fileResource)
	}
	if len(fileResources) == 0 {
		return
	}
	attachedFileResourceHandlers(r, loop, fileResources)
}
