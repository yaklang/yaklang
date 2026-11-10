package reactloops

import (
	"encoding/json"
	"fmt"
	"image"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func imageActionOptions() []aitool.ToolOption {
	return []aitool.ToolOption{
		aitool.WithStringParam("image_id", aitool.WithParam_Required(true)),
		aitool.WithStringParam("question"),
		aitool.WithIntegerParam("x"), aitool.WithIntegerParam("y"),
		aitool.WithIntegerParam("width"), aitool.WithIntegerParam("height"),
	}
}

var loopAction_InspectImage = &LoopAction{
	ActionType:  "inspect_image",
	Description: "Read a retained image into the next prompt. Multimodal main models see it directly; text-only main models receive a vision observation in Timeline. Optional crop is x,y,width,height in the referenced image's pixels. This does not take a new screenshot.",
	Options:     imageActionOptions(),
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, op *LoopActionHandlerOperator) {
		runImageAction(loop, action, op, false)
	},
}

var loopAction_AnalyzeImage = &LoopAction{
	ActionType:  "analyze_image",
	Description: "Ask the separate vision model a specific question about a retained image and record its textual observation in Timeline. Available even when the main model can see images. Optional crop uses the referenced image's pixels. A new question is analyzed again, not answered from an old description.",
	Options:     imageActionOptions(),
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, op *LoopActionHandlerOperator) {
		runImageAction(loop, action, op, true)
	},
}

func runImageAction(loop *ReActLoop, action *aicommon.Action, op *LoopActionHandlerOperator, forceVision bool) {
	c := aicommon.ImageConfigFrom(loop.GetConfig())
	if c == nil {
		op.Feedback("Runtime does not support image resources.")
		op.Continue()
		return
	}
	ctx := c.GetContext()
	if task := loop.GetCurrentTask(); task != nil && task.GetContext() != nil {
		ctx = task.GetContext()
	}
	err := func() error {
		catalog, err := c.GetImageCatalog()
		if err != nil {
			return err
		}
		resource, err := catalog.Find(action.GetString("image_id"))
		if err != nil {
			return err
		}
		img := resource.Image
		x, y, w, h := action.GetInt("x"), action.GetInt("y"), action.GetInt("width"), action.GetInt("height")
		if x != 0 || y != 0 || w != 0 || h != 0 {
			if x < 0 || y < 0 || w <= 0 || h <= 0 || x > img.Width || y > img.Height || w > img.Width-x || h > img.Height-y {
				return fmt.Errorf("crop must have positive dimensions inside the referenced image")
			}
			view, err := aimedia.Derive(ctx, img, image.Rect(x, y, x+w, y+h), int(c.GetImageInputConfig().MaxLongEdge))
			if err != nil {
				return err
			}
			view.Image.Source.OriginalImageID = img.ID
			view.Image.Source.OriginalWidth, view.Image.Source.OriginalHeight = img.Width, img.Height
			view.Image.Source.CropX, view.Image.Source.CropY = x, y
			view.Image.Source.CropWidth, view.Image.Source.CropHeight = w, h
			resource, err = catalog.Register(ctx, fmt.Sprintf("crop:%s:%d:%d:%d:%d", resource.ID, x, y, w, h), view.Image)
			if err != nil {
				return err
			}
			img = resource.Image
			loop.imageEvent("registered", map[string]any{"observation": resource, "scope": c.ImageScope()})
		}
		question := action.GetString("question")
		if question == "" {
			if task := loop.GetCurrentTask(); task != nil {
				question = task.GetUserInput()
			}
		}
		native, _, err := loop.imageRoute()
		if err != nil && !forceVision {
			return err
		}
		if forceVision || !native {
			return c.RecordImageObservation(ctx, "", img, question)
		}
		text := fmt.Sprintf("[image_inspection]:\nresource=%s image_id=%s\nQuestion: %s", resource.ID, img.ID, question)
		c.Timeline.PushTextWithImages(c.AcquireId(), text, img)
		return nil
	}()
	if err != nil {
		op.Feedback("Image could not be observed: " + err.Error())
	} else {
		op.Feedback("Image observation has been added to Timeline.")
	}
	op.Continue()
}

var loopAction_ListImages = &LoopAction{
	ActionType:  "list_images",
	Description: "List retained image resources. A resource being listed does not mean it is attached to this prompt. Use inspect_image to see it or analyze_image to ask the vision model a question.",
	Options:     []aitool.ToolOption{aitool.WithIntegerParam("offset"), aitool.WithIntegerParam("limit")},
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, op *LoopActionHandlerOperator) {
		c := aicommon.ImageConfigFrom(loop.GetConfig())
		if c == nil {
			op.Feedback("Runtime does not support image resources.")
			op.Continue()
			return
		}
		catalog, err := c.GetImageCatalog()
		if err != nil {
			op.Feedback(err.Error())
			op.Continue()
			return
		}
		items := catalog.Snapshot()
		offset := max(0, min(action.GetInt("offset"), len(items)))
		limit := action.GetInt("limit")
		if limit <= 0 {
			limit = 20
		}
		end := min(len(items), offset+min(limit, 20))
		var rows []map[string]any
		for _, r := range items[offset:end] {
			rows = append(rows, map[string]any{"id": r.ID, "image_id": r.Image.ID, "width": r.Image.Width, "height": r.Image.Height, "source": r.Image.Source})
		}
		raw, _ := json.Marshal(map[string]any{"images": rows, "total": len(items), "next_offset": end})
		op.Feedback(string(raw))
		op.Continue()
	},
}
