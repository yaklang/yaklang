package aicommon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
)

// ProjectUIEvent is connection-specific and applies equally to live delivery and
// recovery. The journal always retains the original structured event.
func ProjectUIEvent(event *schema.AiOutputEvent, client UIClientCapabilities) *schema.AiOutputEvent {
	supported := client.Protocol == interaction.Protocol
	if event.Type == "ui_component" {
		var artifact UIArtifact
		if json.Unmarshal(event.Content, &artifact) != nil {
			return event
		}
		var catalog interaction.Catalog
		_ = json.Unmarshal(client.Catalog, &catalog)
		compatible := supported && catalog.SupportsVersion(artifact.Document.CatalogVersion)
		for _, component := range artifact.Document.RequiredComponents {
			compatible = compatible && catalog.Definition(component) != nil
		}
		if compatible {
			return event
		}
		return uiTextFallback(event, artifact.Document.Fallback)
	}
	if event.ContentType == interaction.ComponentContentType || (event.NodeId == "ui_component" && event.Type == "stream_start") {
		if !supported || len(client.Catalog) == 0 {
			return nil // The completed block supplies the plain-text fallback.
		}
	}
	if event.Type == "ui_question" && (!supported || !client.Questions) {
		var request interaction.QuestionRequest
		if json.Unmarshal(event.Content, &request) != nil {
			return event
		}
		var lines []string
		if request.Form != nil {
			lines = append(lines, request.Form.Title)
			fields, _ := request.Form.Schema["properties"].(map[string]any)
			keys := make([]string, 0, len(fields))
			for key := range fields {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				field, _ := fields[key].(map[string]any)
				title, _ := field["title"].(string)
				if title == "" {
					title = key
				}
				lines = append(lines, "- "+title)
			}
		}
		for _, question := range request.Questions {
			lines = append(lines, question.Title+": "+question.Prompt)
			for _, option := range question.Options {
				lines = append(lines, fmt.Sprintf("- %s", option.Label))
			}
		}
		return uiTextFallback(event, strings.Join(lines, "\n"))
	}
	if event.Type == "user_message" && !supported {
		var message UIUserMessage
		if json.Unmarshal(event.Content, &message) == nil {
			if message.TaskID != "" {
				return nil // Legacy clients display the ordinary dequeue event.
			}
			copy := *event
			copy.Type, copy.NodeId = schema.EVENT_TYPE_STRUCTURED, "user_intervention"
			copy.Content, _ = json.Marshal(map[string]string{"content": message.Text})
			return &copy
		}
	}
	return event
}

func uiTextFallback(event *schema.AiOutputEvent, text string) *schema.AiOutputEvent {
	if strings.TrimSpace(text) == "" {
		text = "此内容无法以组件呈现。请使用纯文本继续。"
	}
	copy := *event
	copy.Type, copy.NodeId = "result", "ui_text_fallback"
	copy.IsStream, copy.IsJson = false, true
	copy.ContentType = ""
	copy.Content, _ = json.Marshal(map[string]any{"result": text})
	return &copy
}
