package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

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
		timelineKey := "attached_" + strings.ReplaceAll(typ, "-", "_")
		r.AddToTimeline(timelineKey, strings.Join(sections, "\n\n---\n\n"))
		r.AddToTimeline("import notice", fmt.Sprintf("%s has been recorded; use it when reasoning about the user's attached resources.", timelineKey))
	}

	if len(sectionsByType) > 0 {
		loop.LoadingStatus("附加资源处理完成 / Finished loading attached resources")
	}
	return resources
}
