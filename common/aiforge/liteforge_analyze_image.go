package aiforge

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed liteforge_schema/liteforge_image.schema.json
var IMAGE_OUTPUT_SCHEMA string

// TemporalQualifier represents the temporal nature of a relationship
type TemporalQualifier string

const (
	TemporalStarts     TemporalQualifier = "starts"
	TemporalEnds       TemporalQualifier = "ends"
	TemporalContinuous TemporalQualifier = "continuous"
	TemporalMomentary  TemporalQualifier = "momentary"
)

// TimeOfDay represents the time of day in the scene
type TimeOfDay string

const (
	TimeDaylight TimeOfDay = "daylight"
	TimeDusk     TimeOfDay = "dusk"
	TimeNight    TimeOfDay = "night"
	TimeDawn     TimeOfDay = "dawn"
	TimeUnknown  TimeOfDay = "unkown" // Note: keeping the typo from schema for compatibility
)

// ElementRole represents the importance of an element in the scene
type ElementRole string

const (
	RoleMain       ElementRole = "main"
	RoleSupporting ElementRole = "supporting"
	RoleBackground ElementRole = "background"
)

// TextRole represents the functional role of text
type TextRole string

const (
	TextRoleTitle     TextRole = "title"
	TextRoleCaption   TextRole = "caption"
	TextRoleLogo      TextRole = "logo"
	TextRoleLabel     TextRole = "label"
	TextRoleParagraph TextRole = "paragraph"
)

// Relationship represents a connection between visual elements
type Relationship struct {
	SubjectID         string            `json:"subject_id"`
	Predicate         string            `json:"predicate"`
	ObjectID          string            `json:"object_id"`
	SpatialQualifier  string            `json:"spatial_qualifier,omitempty"`
	TemporalQualifier TemporalQualifier `json:"temporal_qualifier,omitempty"`
	Confidence        float64           `json:"confidence,omitempty"`
}

// SceneContext describes the overall context of the scene
type SceneContext struct {
	LocationType   string    `json:"location_type"`
	TimeOfDay      TimeOfDay `json:"time_of_day"`
	OverallEmotion string    `json:"overall_emotion,omitempty"`
	InferredIntent string    `json:"inferred_intent,omitempty"`
}

// TextElement represents OCR-detected text in the image
type TextElement struct {
	ID          string     `json:"id"`
	Text        string     `json:"text"`
	Role        TextRole   `json:"role"`
	BoundingBox [4]float64 `json:"bounding_box"`
	Confidence  float64    `json:"confidence,omitempty"`
}

// VisualElement represents a visual object in the scene
type VisualElement struct {
	ID               string                 `json:"id"`
	Label            string                 `json:"label"`
	Confidence       float64                `json:"confidence"`
	Description      string                 `json:"description"`
	Role             ElementRole            `json:"role"`
	BoundingBox      [4]float64             `json:"bounding_box"`
	Attributes       map[string]interface{} `json:"attributes,omitempty"`
	State            []string               `json:"state,omitempty"`
	SegmentationMask string                 `json:"segmentation_mask,omitempty"`
}

// ImageAnalysisResult represents the complete result of image analysis
type ImageAnalysisResult struct {
	CumulativeSummary string          `json:"cumulative_summary"`
	Relationships     []Relationship  `json:"relationships,omitempty"`
	SceneContext      SceneContext    `json:"scene_context,omitempty"`
	TextElements      []TextElement   `json:"text_elements,omitempty"`
	VisualElements    []VisualElement `json:"visual_elements,omitempty"`
}

func (i *ImageAnalysisResult) OCR() string {
	if i == nil {
		return ""
	}
	ocrList := i.OCRList()
	return strings.Join(ocrList, "\n")
}

func (i *ImageAnalysisResult) OCRList() []string {
	if i == nil || len(i.TextElements) == 0 {
		return []string{}
	}

	var ocrList []string
	for _, textElement := range i.TextElements {
		if textElement.Text != "" {
			ocrList = append(ocrList, textElement.Text)
		}
	}
	return ocrList
}

func (i *ImageAnalysisResult) GetCumulativeSummary() string {
	return i.CumulativeSummary
}

func (i *ImageAnalysisResult) Dump() string {
	if i == nil {
		return ""
	}

	var result strings.Builder

	// Visual elements
	if len(i.VisualElements) > 0 {
		result.WriteString("visual:\n")
		for _, element := range i.VisualElements {
			result.WriteString(fmt.Sprintf("  %s[%s]: %s\n", element.Label, element.ID, element.Description))
		}
		result.WriteString("\n")
	}

	// Text elements
	if len(i.TextElements) > 0 {
		result.WriteString("text:\n")
		for _, textElement := range i.TextElements {
			result.WriteString(fmt.Sprintf("  %s[%s]: %s\n", textElement.Role, textElement.ID, textElement.Text))
		}
		result.WriteString("\n")
	}

	// Relationships
	if len(i.Relationships) > 0 {
		result.WriteString("relationships:\n")
		for _, rel := range i.Relationships {
			// Find subject and object labels
			subjectLabel := i.findElementLabel(rel.SubjectID)
			objectLabel := i.findElementLabel(rel.ObjectID)

			relationStr := fmt.Sprintf("  %s[%s] %s %s[%s]",
				subjectLabel, rel.SubjectID, rel.Predicate, objectLabel, rel.ObjectID)

			if rel.SpatialQualifier != "" {
				relationStr += fmt.Sprintf(" (%s)", rel.SpatialQualifier)
			}

			result.WriteString(relationStr + "\n")
		}
		result.WriteString("\n")
	}

	// Scene context
	if i.SceneContext.LocationType != "" || i.SceneContext.TimeOfDay != "" {
		result.WriteString("scene context:\n")
		if i.SceneContext.LocationType != "" {
			result.WriteString(fmt.Sprintf("  location: %s\n", i.SceneContext.LocationType))
		}
		if i.SceneContext.TimeOfDay != "" {
			result.WriteString(fmt.Sprintf("  time: %s\n", i.SceneContext.TimeOfDay))
		}
		if i.SceneContext.OverallEmotion != "" {
			result.WriteString(fmt.Sprintf("  overall emotion: %s\n", i.SceneContext.OverallEmotion))
		}
		if i.SceneContext.InferredIntent != "" {
			result.WriteString(fmt.Sprintf("  inferred intent: %s\n", i.SceneContext.InferredIntent))
		}
		result.WriteString("\n")
	}

	// Cumulative summary
	if i.CumulativeSummary != "" {
		result.WriteString("cumulative summary:\n")
		result.WriteString(fmt.Sprintf("  %s\n", i.CumulativeSummary))
	}

	return result.String()
}

// findElementLabel finds the label for a given element ID (visual or text element)
func (i *ImageAnalysisResult) findElementLabel(elementID string) string {
	if i == nil {
		return "unknown"
	}

	// Check visual elements
	for _, element := range i.VisualElements {
		if element.ID == elementID {
			return element.Label
		}
	}

	// Check text elements
	for _, textElement := range i.TextElements {
		if textElement.ID == elementID {
			return string(textElement.Role)
		}
	}

	return "unknown"
}

// _safeBoundingBoxFromSlice 安全地从接口切片中提取边界框数据
// 返回 [x, y, width, height] 格式的边界框和是否成功的标志
func _safeBoundingBoxFromSlice(data interface{}) ([4]float64, bool) {
	var result [4]float64

	if data == nil {
		return result, false
	}

	switch v := data.(type) {
	case []interface{}:
		if len(v) < 4 {
			return result, false
		}
		for i := 0; i < 4; i++ {
			result[i] = utils.InterfaceToFloat64(v[i])
		}
		return result, true

	case []float64:
		if len(v) < 4 {
			return result, false
		}
		copy(result[:], v[:4])
		return result, true

	case []float32:
		if len(v) < 4 {
			return result, false
		}
		for i := 0; i < 4; i++ {
			result[i] = float64(v[i])
		}
		return result, true

	case [4]float64:
		return v, true

	case [4]float32:
		for i := 0; i < 4; i++ {
			result[i] = float64(v[i])
		}
		return result, true

	default:
		return result, false
	}
}

// _safeGetStringSlice 安全地从InvokeParams中获取字符串切片
// 支持将单个字符串、字符串数组或接口数组转换为字符串切片
func _safeGetStringSlice(params map[string]interface{}, key string) []string {
	if params == nil {
		return nil
	}

	rawValue, exists := params[key]
	if !exists || rawValue == nil {
		return nil
	}

	switch v := rawValue.(type) {
	case []string:
		return v

	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str := utils.InterfaceToString(item); str != "" {
				result = append(result, str)
			}
		}
		return result

	case string:
		return []string{v}

	default:
		return nil
	}
}

// _safeGetFloat64 安全地获取float64值，带有默认值
// 当输入为nil或无法转换时返回默认值
func _safeGetFloat64(value interface{}, defaultValue float64) float64 {
	if value == nil {
		return defaultValue
	}
	return utils.InterfaceToFloat64(value)
}

// _safeGetString 安全地获取字符串值，带有默认值
// 当输入为nil或无法转换时返回默认值
func _safeGetString(value interface{}, defaultValue string) string {
	if value == nil {
		return defaultValue
	}
	return utils.InterfaceToString(value)
}

// Validate 验证ImageAnalysisResult的数据完整性和一致性
func (i *ImageAnalysisResult) Validate() []string {
	var issues []string

	if i == nil {
		return []string{"ImageAnalysisResult is nil"}
	}

	// 检查必需字段
	if i.CumulativeSummary == "" {
		issues = append(issues, "cumulative_summary is empty (required field)")
	}

	// 检查视觉元素的有效性
	for idx, visual := range i.VisualElements {
		if visual.ID == "" {
			issues = append(issues, fmt.Sprintf("visual element %d has empty ID", idx))
		}
		if visual.Label == "" {
			issues = append(issues, fmt.Sprintf("visual element %d (%s) has empty label", idx, visual.ID))
		}
		if visual.Confidence < 0 || visual.Confidence > 1 {
			issues = append(issues, fmt.Sprintf("visual element %d (%s) has invalid confidence: %f", idx, visual.ID, visual.Confidence))
		}
	}

	// 检查文本元素的有效性
	for idx, text := range i.TextElements {
		if text.ID == "" {
			issues = append(issues, fmt.Sprintf("text element %d has empty ID", idx))
		}
		if text.Text == "" {
			issues = append(issues, fmt.Sprintf("text element %d (%s) has empty text", idx, text.ID))
		}
		if text.Confidence < 0 || text.Confidence > 1 {
			issues = append(issues, fmt.Sprintf("text element %d (%s) has invalid confidence: %f", idx, text.ID, text.Confidence))
		}
	}

	// 检查关系的有效性
	for idx, rel := range i.Relationships {
		if rel.SubjectID == "" || rel.ObjectID == "" || rel.Predicate == "" {
			issues = append(issues, fmt.Sprintf("relationship %d has empty required fields (subject: %s, predicate: %s, object: %s)",
				idx, rel.SubjectID, rel.Predicate, rel.ObjectID))
		}

		// 验证引用的元素是否存在
		if !i.hasElement(rel.SubjectID) {
			issues = append(issues, fmt.Sprintf("relationship %d references non-existent subject: %s", idx, rel.SubjectID))
		}
		if !i.hasElement(rel.ObjectID) {
			issues = append(issues, fmt.Sprintf("relationship %d references non-existent object: %s", idx, rel.ObjectID))
		}
	}

	return issues
}

// hasElement 检查给定ID的元素是否存在
func (i *ImageAnalysisResult) hasElement(elementID string) bool {
	if i == nil || elementID == "" {
		return false
	}

	// 检查视觉元素
	for _, visual := range i.VisualElements {
		if visual.ID == elementID {
			return true
		}
	}

	// 检查文本元素
	for _, text := range i.TextElements {
		if text.ID == elementID {
			return true
		}
	}

	return false
}

// Stats 返回结果的统计信息
func (i *ImageAnalysisResult) Stats() map[string]interface{} {
	if i == nil {
		return map[string]interface{}{"error": "result is nil"}
	}

	stats := map[string]interface{}{
		"visual_elements_count":  len(i.VisualElements),
		"text_elements_count":    len(i.TextElements),
		"relationships_count":    len(i.Relationships),
		"has_cumulative_summary": i.CumulativeSummary != "",
		"has_scene_context":      i.SceneContext.LocationType != "" || i.SceneContext.TimeOfDay != "",
	}

	// 计算平均置信度
	if len(i.VisualElements) > 0 {
		var totalConfidence float64
		for _, visual := range i.VisualElements {
			totalConfidence += visual.Confidence
		}
		stats["avg_visual_confidence"] = totalConfidence / float64(len(i.VisualElements))
	}

	if len(i.TextElements) > 0 {
		var totalConfidence float64
		for _, text := range i.TextElements {
			totalConfidence += text.Confidence
		}
		stats["avg_text_confidence"] = totalConfidence / float64(len(i.TextElements))
	}

	return stats
}

func AnalyzeImageFile(image string, opts ...any) (*ImageAnalysisResult, error) {
	if !utils.FileExists(image) {
		return nil, fmt.Errorf("image file not found: %s", image)
	}

	raw, err := os.ReadFile(image)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file %s: %w", image, err)
	}
	return AnalyzeImage(raw, opts...)
}

func AnalyzeImage(image any, opts ...any) (*ImageAnalysisResult, error) {
	var imgCfg = NewAnalysisConfig(opts...)
	imgCfg.fallbackOptions = append(imgCfg.fallbackOptions, _withImageCompress(image), _withForceImage(true))
	imgCfg.fallbackOptions = append(imgCfg.fallbackOptions, WithOutputJSONSchema(IMAGE_OUTPUT_SCHEMA))
	// 构建详细的分析提示
	prompt := `Your primary task is to perform a comprehensive, multi-modal analysis of the provided inputs. You will receive an **image** and, optionally, **supplementary text** (such as a title, user description, or speech-to-text transcript). Your analysis must holistically synthesize information from both sources to generate a detailed JSON output.

**Core Mandate: Synthesize Information**

Do not treat the image and the text as separate items. You must **integrate** the supplementary information to **enrich, confirm, and disambiguate** your visual analysis.

*   Use the text to clarify ambiguous objects, locations, or relationships.
*   Allow the text to guide your interpretation of the scene's context, mood, and purpose.
*   Your final summary must explicitly weave together insights from both the visual evidence and the provided text.

**Inputs**

1.  **Image:** The primary visual content for analysis.
2.  **Supplementary Information:** (Optional, may be "null") A string of text that provides additional context about the image.


1. **Visual Elements**: Identify and describe all objects, people, animals, or items visible in the image
2. **Text Elements**: Extract all text content using OCR (Optical Character Recognition) 
3. **Relationships**: Describe how elements relate to each other spatially and contextually
4. **Scene Context**: Determine the location type, time of day, overall mood, and inferred purpose

**Important Instructions:**
- Provide unique IDs for each element (v_1, v_2, etc. for visual elements; t_1, t_2, etc. for text elements)
- Include confidence scores for all detections
- Use descriptive labels and detailed descriptions
- Extract ALL visible text, even if partially obscured
- Establish relationships between identified elements
- Ensure the cumulative_summary synthesizes all findings into a coherent narrative

**Output Requirements:**
- Must include "@action": "object" field
- All required fields must be populated
- Follow the provided JSON schema exactly
- Return valid JSON without additional commentary
- Ensure cumulative_summary is comprehensive and descriptive

` + imgCfg.ExtraPrompt

	forgeResult, err := _executeLiteForgeTemp(prompt, imgCfg.ForgeExecOption(IMAGE_OUTPUT_SCHEMA)...)
	if err != nil {
		return nil, err
	}
	if forgeResult == nil || forgeResult.Action == nil {
		return nil, fmt.Errorf("invalid forge result")
	}

	// 添加调试信息
	log.Debugf("ForgeResult Action Name: %s", forgeResult.Action.Name())
	log.Debugf("ForgeResult Action Type: %s", forgeResult.Action.ActionType())
	// 添加调试信息 - 输出是否检测到图像相关内容
	if forgeResult.GetString("cumulative_summary") == "" {
		log.Warnf("No cumulative_summary found in action result")
	}
	if len(forgeResult.GetInvokeParamsArray("visual_elements")) == 0 {
		log.Warnf("No visual_elements found in action result")
	}
	if len(forgeResult.GetInvokeParamsArray("text_elements")) == 0 {
		log.Warnf("No text_elements found in action result")
	}

	// 检查具体的字段内容用于调试
	log.Debugf("Raw cumulative_summary: %q", forgeResult.GetString("cumulative_summary"))
	log.Debugf("Visual elements count: %d", len(forgeResult.GetInvokeParamsArray("visual_elements")))
	log.Debugf("Text elements count: %d", len(forgeResult.GetInvokeParamsArray("text_elements")))

	result := &ImageAnalysisResult{}
	/*
		handle forgeResult.Action -> *ImageAnalysisResult
	*/
	// 修复累积摘要提取问题 - 直接从ForgeResult中提取
	result.CumulativeSummary = forgeResult.GetString("cumulative_summary")
	log.Debugf("Extracted cumulative_summary: %q", result.CumulativeSummary)
	sceneContext := forgeResult.GetInvokeParams("scene_context")
	if sceneContext != nil {
		// 安全处理场景上下文
		locationType := sceneContext.GetString("location_type")
		timeOfDay := sceneContext.GetString("time_of_day")

		// 验证TimeOfDay的有效性
		validTimeOfDay := TimeOfDay(timeOfDay)
		switch validTimeOfDay {
		case TimeDaylight, TimeDusk, TimeNight, TimeDawn, TimeUnknown:
			// 有效的时间值
		default:
			validTimeOfDay = TimeUnknown // 默认为未知
		}

		result.SceneContext = SceneContext{
			LocationType:   locationType,
			TimeOfDay:      validTimeOfDay,
			OverallEmotion: sceneContext.GetString("overall_emotion", ""),
			InferredIntent: sceneContext.GetString("inferred_intent", ""),
		}
	}
	for _, relationship := range forgeResult.GetInvokeParamsArray("relationships") {
		if relationship == nil {
			continue // 跳过空的relationship元素
		}

		subjectID := relationship.GetString("subject_id")
		predicate := relationship.GetString("predicate")
		objectID := relationship.GetString("object_id")

		// 只添加有效的关系（必须有主语、谓语、宾语）
		if subjectID != "" && predicate != "" && objectID != "" {
			rel := Relationship{
				SubjectID:         subjectID,
				Predicate:         predicate,
				ObjectID:          objectID,
				SpatialQualifier:  relationship.GetString("spatial_qualifier"),
				TemporalQualifier: TemporalQualifier(relationship.GetString("temporal_qualifier")),
				Confidence:        relationship.GetFloat("confidence"),
			}
			result.Relationships = append(result.Relationships, rel)
		}
	}
	for idx, visual := range forgeResult.GetInvokeParamsArray("visual_elements") {
		if visual == nil {
			continue // 跳过空的visual元素
		}

		log.Debugf("Processing visual element %d: id=%q, label=%q", idx, visual.GetString("id"), visual.GetString("label"))

		element := VisualElement{
			ID:          visual.GetString("id"),
			Label:       visual.GetString("label"),
			Description: visual.GetString("description"),
			Confidence:  visual.GetFloat("confidence"),
			Role:        ElementRole(visual.GetString("role")),
		}

		// 安全处理边界框
		if boundingBox, ok := _safeBoundingBoxFromSlice(visual["bounding_box"]); ok {
			element.BoundingBox = boundingBox
		}

		// 安全处理属性
		if attrs := visual.GetObject("attributes"); len(attrs) > 0 {
			element.Attributes = make(map[string]interface{})
			for k, v := range attrs {
				if k != "" && v != nil { // 只添加有效的键值对
					element.Attributes[k] = v
				}
			}
		}

		// 安全处理状态
		if states := _safeGetStringSlice(visual, "state"); len(states) > 0 {
			element.State = states
		}

		// 安全处理分割掩码
		element.SegmentationMask = visual.GetString("segmentation_mask", "")

		result.VisualElements = append(result.VisualElements, element)
	}

	// 处理文本元素
	for idx, text := range forgeResult.GetInvokeParamsArray("text_elements") {
		if text == nil {
			continue // 跳过空的text元素
		}

		log.Debugf("Processing text element %d: id=%q, text=%q", idx, text.GetString("id"), text.GetString("text"))

		textElement := TextElement{
			ID:         text.GetString("id"),
			Text:       text.GetString("text"),
			Role:       TextRole(text.GetString("role")),
			Confidence: text.GetFloat("confidence"),
		}

		// 安全处理边界框
		if boundingBox, ok := _safeBoundingBoxFromSlice(text["bounding_box"]); ok {
			textElement.BoundingBox = boundingBox
		}

		// 只添加有文本内容的元素
		if textElement.Text != "" {
			result.TextElements = append(result.TextElements, textElement)
		}
	}

	// 验证结果的完整性
	if validationIssues := result.Validate(); len(validationIssues) > 0 {
		log.Warnf("Image analysis result has validation issues: %v", validationIssues)
		// 不阻止返回，只是记录警告
	}

	log.Infof("Image analysis completed successfully: %v", result.Stats())
	return result, nil
}

func AnalyzeSingleMedia(mediaPath string, opts ...any) (<-chan AnalysisResult, error) {
	if !utils.FileExists(mediaPath) {
		return nil, fmt.Errorf("media file not found: %s", mediaPath)
	}

	analyzeConfig := NewAnalysisConfig(opts...)
	analyzeConfig.AnalyzeStatusCard("Analysis", "extracting images from media")
	chunkOption := []chunkmaker.Option{chunkmaker.WithCtx(analyzeConfig.Ctx)}
	chunkOption = append(chunkOption, analyzeConfig.chunkOption...)

	cm, err := chunkmaker.NewChunkMakerFromFile(mediaPath, chunkmaker.WithCtx(analyzeConfig.Ctx))
	if err != nil {
		return nil, err
	}

	indexedChannel := chanx.NewUnlimitedChan[chunkmaker.Chunk](analyzeConfig.Ctx, 100)
	count := 0
	ar, err := aireducer.NewReducerEx(cm,
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
			analyzeConfig.AnalyzeLog("chunk index[%d] size:%v Analyzing media type [%s]", count, utils.ByteSize(uint64(chunk.BytesSize())), chunk.MIMEType().String())
			indexedChannel.SafeFeed(chunk)
			count++
			analyzeConfig.AnalyzeStatusCard("多模态切片(Multimodels-Chunk)", count)
			return nil
		}),
		aireducer.WithContext(analyzeConfig.Ctx),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to analyze media image: %w", err)
	}

	go func() {
		defer indexedChannel.Close()
		err = ar.Run()
		if err != nil {
			log.Errorf("failed to run analyze media image: %v", err)
		}
	}()

	processedCount := 0

	return utils.OrderedParallelProcessSkipError[chunkmaker.Chunk, AnalysisResult](analyzeConfig.Ctx, indexedChannel.OutputChannel(), func(chunk chunkmaker.Chunk) (AnalysisResult, error) {
		defer func() {
			processedCount++
			analyzeConfig.AnalyzeStatusCard("知识实体分析(Entity Analyzing)", processedCount)
		}()
		if chunk.MIMEType().IsImage() {
			return AnalyzeImage(chunk.Data(), opts...)
		} else {
			return &TextAnalysisResult{Text: string(chunk.Data())}, nil
		}
	},
		utils.WithParallelProcessConcurrency(analyzeConfig.AnalyzeConcurrency),
		utils.WithParallelProcessStartCallback(func() {
			analyzeConfig.AnalyzeStatusCard("Analysis", "processing media chunk")
		}),
		utils.WithParallelProcessFinishCallback(func() {
			analyzeConfig.AnalyzeStatusCard("Analysis", "finished preliminary analysis")
		})), nil
}
