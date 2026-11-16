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

type attachedResourceUserPinNotice interface {
	UserPinNotice() string
}

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

	nonEmptyResources := aicommon.NonEmptyAttachedResources(attachedDatas)
	for idx := range nonEmptyResources {
		loop.UserStatus(
			fmt.Sprintf("正在加载第 %d 个附加资源", idx+1),
			fmt.Sprintf("Loading attached resource %d", idx+1),
		)
	}

	resources, parseErrors := aicommon.ParseAttachedResourceDataList(nonEmptyResources)
	for _, err := range parseErrors {
		log.Warnf("attached extra resources: %v", err)
	}
	resourcesByType := make(map[string][]aicommon.AttachedResourceData)
	for _, resource := range resources {
		resourcesByType[resource.Type()] = append(resourcesByType[resource.Type()], resource)
	}

	sectionsByType := make(map[string][]string)
	for typ, typedResources := range resourcesByType {
		for _, resource := range typedResources {
			if err := resource.BindLoopData(loop); err != nil {
				log.Warnf("attached extra resources: failed to bind type=%q: %v", typ, err)
				continue
			}
			recordAttachedResourceUserPin(r, resource)
			rendered := strings.TrimSpace(resource.ToAttachData(loop))
			if rendered != "" {
				sectionsByType[typ] = append(sectionsByType[typ], rendered)
			}
		}
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
	}

	if len(resources) > 0 {
		loop.UserStatus("附加资源处理完成", "Finished loading attached resources")
	}
	return resources
}

// recordAttachedResourceUserPin keeps the fact that a user attached a file
// outside ordinary compression while leaving the full rendering in Timeline.
func recordAttachedResourceUserPin(r aicommon.AIInvokeRuntime, resource aicommon.AttachedResourceData) {
	pinned, ok := resource.(attachedResourceUserPinNotice)
	if !ok || r == nil || r.GetConfig() == nil {
		return
	}
	notice := strings.TrimSpace(pinned.UserPinNotice())
	if notice == "" {
		return
	}
	provider, ok := r.GetConfig().(interface {
		GetTimeline() *aicommon.Timeline
	})
	if !ok || provider.GetTimeline() == nil {
		return
	}
	taskID := ""
	if task := r.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	provider.GetTimeline().EnsureTaskUserInput(taskID, notice, r.GetConfig().AcquireId)
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
