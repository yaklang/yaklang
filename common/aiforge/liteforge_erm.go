package aiforge

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag/entitybase"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type TemporaryRelationship struct {
	SourceTemporaryName  string
	TargetTemporaryName  string
	RelationshipType     string
	DecorationAttributes string
	DecisionRationale    string
}

type ERMAnalysisResult struct {
	Entities      []*schema.ERModelEntity
	Relationships []*TemporaryRelationship
	OriginalData  []byte
}

func (e *ERMAnalysisResult) GetCumulativeSummary() string {
	return ""
}

func (e *ERMAnalysisResult) Dump() string {
	var sb strings.Builder
	sb.WriteString("Entities:\n")
	for name, entity := range e.Entities {
		sb.WriteString(fmt.Sprintf("- ID: %d\n", name))
		sb.WriteString(fmt.Sprintf("  QualifiedName: %s\n", entity.EntityName))
		sb.WriteString(fmt.Sprintf("  EntityType: %s\n", entity.EntityType))
		if entity.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", utils.ShrinkString(entity.Description, 100)))
		}
		if entity.Rationale != "" {
			sb.WriteString(fmt.Sprintf("  Rationale: %s\n", utils.ShrinkString(entity.Rationale, 100)))
		}
		if len(entity.Attributes) > 0 {
			sb.WriteString("  Attributes:\n")
			for key, value := range entity.Attributes {
				sb.WriteString(fmt.Sprintf("    - %s: %s\n", key, utils.ShrinkString(value, 100)))
			}
		}
	}
	sb.WriteString("Relationships:\n")
	for _, Relationship := range e.Relationships {
		sb.WriteString(fmt.Sprintf("- Source: %s\n", Relationship.SourceTemporaryName))
		sb.WriteString(fmt.Sprintf("  Target: %s\n", Relationship.TargetTemporaryName))
		sb.WriteString(fmt.Sprintf("  Type: %s\n", Relationship.RelationshipType))
		if Relationship.DecisionRationale != "" {
			sb.WriteString(fmt.Sprintf("  Rationale: %s\n", utils.ShrinkString(Relationship.DecisionRationale, 100)))
		}
	}
	return sb.String()
}

func (e *ERMAnalysisResult) GenerateDotGraph() *dot.Graph {
	G := dot.New()
	G.MakeDirected()
	for _, entity := range e.Entities {
		n := G.AddNode(entity.EntityName)
		for key, value := range entity.Attributes {
			G.NodeAttribute(n, key, utils.InterfaceToString(value))
		}
	}

	for _, Relationship := range e.Relationships {
		G.AddEdgeByLabel(Relationship.SourceTemporaryName, Relationship.TargetTemporaryName, Relationship.RelationshipType)
	}

	return G
}

func (e *ERMAnalysisResult) ShowDotGraph() {
	art, err := dot.DotGraphToAsciiArt(e.GenerateDotGraph().GenerateDOTString())
	if err != nil {
		return
	}
	fmt.Println(art)
}

var DetectPrompt = `# **你的角色与目标**
你是一个智能文本领域分类器。你的任务是精准地分析输入的一小段文本（<|INPUT|>），并从以下四个预定义领域中，选择**一个最主要、最贴切**的领域。这个分类至关重要，因为它将决定后续使用哪种专业的分析引擎来深度解析整个文档。你的选择必须果断且有依据。
# **领域定义与分类标准**
请严格按照以下定义来判断：
### 1. "code" (代码领域)
*   **分类意义**: 此领域代表由程序员编写、用于计算机执行的指令。内容遵循严格的语法和结构。
*   **识别目标**: 识别出编程语言、脚本语言、查询语言或标记/配置文件。
*   **关键特征**: 出现编程关键字("func", "class")、大量特殊符号("{}", "()", ";")、注释("//", "#")。
*   **示例**: Go, Python, JavaScript, SQL, Dockerfile, Nginx配置。
### 2. "rules" (规则领域)
*   **分类意义**: 此领域代表用于定义行为、约束、权利或义务的结构化文本。
*   **识别目标**: 识别出法律条文、公司政策、合同协议或高度结构化的配置（如CI/CD）。
*   **关键特征**: 使用契约性语言(“甲方应...”)、条款章节形式、YAML/TOML等配置语法。
*   **示例**: GDPR法律条文、用户服务协议(TOS)、".gitlab-ci.yml"。
### 3. "log" (日志领域)
*   **分类意义**: 此领域代表由机器或系统自动生成的、按时间顺序记录的事件数据。
*   **识别目标**: 识别出来自应用程序、服务器或网络设备的运行时记录。
*   **关键特征**: 以**时间戳**开始、包含日志级别("INFO", "ERROR")、包含源标识(IP地址, PID)。
*   **示例**: Nginx访问日志、应用错误堆栈、系统syslog。
### 4. "other" (其他领域)
*   **分类意义**: 这是包罗万象的默认类别，适用于所有不属于上述三个专业领域的、以**自然语言**为主的文本。
*   **识别目标**: 识别出人类日常交流、写作和阅读的内容。
*   **关键特征**: 连贯的散文、段落或对话。
*   **示例**: 小说、新闻文章、个人简历、产品说明书、聊天记录。
# **你的任务**
现在，请分析下面的文本，并输出它所属的**唯一**领域。`

var detectDomainSchema = aitool.NewObjectSchemaWithAction(
	aitool.WithStringParam(
		"domain_type",
		aitool.WithParam_Description("判断输入内容限定的领域是哪些？"),
		aitool.WithParam_Enum("code", "rule", "log", "other"),
	),
)

//go:embed liteforge_prompt/entity_analyze_code.txt
var ermCodePrompt string

//go:embed liteforge_prompt/entity_analyze_rule.txt
var ermRulesPrompt string

//go:embed liteforge_prompt/entity_analyze_log.txt
var ermLogPrompt string

//go:embed liteforge_prompt/entity_analyze_other.txt
var ermOtherPrompt string

func DetectERMPrompt(input string, options ...any) (string, error) {
	analyzeConfig := NewAnalysisConfig(options...)
	options = append(options, _withOutputJSONSchema(detectDomainSchema))
	detectResut, err := _executeLiteForgeTemp(quickQueryBuild(DetectPrompt, input), options...)
	if err != nil {
		return "", err
	}
	analyzeConfig.AnalyzeLog("chunk [%s] detected domain type: %s", utils.ShrinkString(detectResut, 800), detectResut.GetString("domain_type"))

	switch detectResut.GetString("domain_type") {
	case "code":
		return ermCodePrompt, nil
	case "rule":
		return ermRulesPrompt, nil
	case "log":
		return ermLogPrompt, nil
	case "other":
		fallthrough
	default:
		return ermOtherPrompt, nil
	}
}

var entitySchema = []aitool.ToolOption{
	aitool.WithStringParam(
		"identifier",
		aitool.WithParam_Description(`一个机器可读的、全局唯一的、可预测的实体ID。格式必须严格遵循 "类型:唯一限定名" 的规范。
**格式示例 (Good Case):**
- **代码:** "func:huff0.Decoder.Decompress4X", "type:huff0.Decoder", "package:huff0"
- **规则:** "rule:gdpr.article_17.section_1", "term:gdpr.personal_data", "obligation:processor.ensure_security"
- **日志:** "event:2023-10-27T10:00:01Z_login_failure", "ip:192.168.1.101", "user_id:john.doe"
- **通用:** "person:elon_musk", "concept:artificial_intelligence", "location:paris"`),
		aitool.WithParam_Required(true),
	),
	aitool.WithStringParam(
		"qualified_name",
		aitool.WithParam_Description(`人类可读的、在领域内完整的实体名称，应包含足够的上下文。
**格式示例 (Good Case):**
- **代码:** "func (d *Decoder) Decompress4X(dst, src []byte, dt *huffmanTable) (n int, err error)"
- **规则:** "Article 17(1): The data subject shall have the right to obtain from the controller the erasure of personal data."
- **日志:** "Login failed for user 'john.doe' from 192.168.1.101"
- **通用:** "Elon Musk, CEO of SpaceX and Tesla"`),
		aitool.WithParam_Required(true),
	),
	aitool.WithStringParam(
		"decision_rationale",
		aitool.WithParam_Description("Explain the reasoning behind the values assigned to the other fields in this JSON object. Justify the choices made."),
	),
	aitool.WithStructArrayParam(
		"attributes",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("You're seeking to extract attributes from this entity. Your objective is to identify fundamental attributes for well-defined entities. For instance, in the case of a \"person\" entity, you would extract their name. Similarly, for a \"log\" entity, the occurrence time would be a crucial attribute."),
		}, nil,
		aitool.WithStringParam(
			"attribute_name",
			aitool.WithParam_Description("The name of the attribute"),
		),
		aitool.WithStringParam(
			"attribute_value",
			aitool.WithParam_Description("The value of the attribute"),
		),
		aitool.WithBoolParam(
			"unique_identifier",
			aitool.WithParam_Description("Determine whether this attribute serves as a latent primary key for the entity. That is, ascertain if the attribute could function as a unique identifier, such as a UUID, email address, or fully qualified name."),
		),
	),
	aitool.WithStringParam(
		"description",
		aitool.WithParam_Description("A brief description of the entity providing additional context or details."),
	),
	aitool.WithStringParam(
		"entity_type",
		aitool.WithParam_Description(`The type of the entity, from the universal EntityType enum.`),
		aitool.WithParam_Enum(
			"PERSON", "ORGANIZATION", "LOCATION", "EVENT", "DOCUMENT", "SECTION", "CONCEPT",
			"THEORY", "CLAIM", "QUOTE", "NUMERIC_DATA", "CODE_MODULE", "CODE_TYPE", "CODE_FUNCTION",
			"CODE_VARIABLE", "DESIGN_PATTERN", "CONCURRENCY_PRIMITIVE", "ERROR_PATTERN", "DEPENDENCY",
			"CONFIGURATION_ENTRY", "DATA_SOURCE", "LEGAL_RULE", "DEFINITION", "LEGAL_SUBJECT", "POLICY",
			"OBLIGATION", "RIGHT", "PROHIBITION", "CONDITION", "CONSEQUENCE", "SCOPE_OF_APPLICABILITY",
			"LOG_ENTRY", "SERVICE_COMPONENT", "TRACE_ID", "SESSION_ID", "USER_ID", "IP_ADDRESS",
			"HOSTNAME", "STATUS_CODE", "PERFORMANCE_METRIC", "SECURITY_EVENT",
		),
	),
}

var ermOutputSchema = aitool.NewObjectSchemaWithAction(
	// 定义 entity_list，一个对象数组
	aitool.WithStructArrayParam(
		"entity_list",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("Represents a node in the knowledge graph, such as a person, a function, a concept, a veriable, an object/instance, a pure function, a definition"),
		}, nil,
		// 定义数组中每个对象的字段
		entitySchema...,
	),
	// 定义 Relationship_list，另一个对象数组
	aitool.WithStructArrayParam(
		"relationship_list",
		[]aitool.PropertyOption{
			aitool.WithParam_Description(`Represents a directed edge between two entities in the knowledge graph.`),
		},
		nil,
		// 定义数组中每个对象的字段
		aitool.WithStringParam(
			"source",
			aitool.WithParam_Description("the entity(identifier) where the Relationship originates."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"target",
			aitool.WithParam_Description("The entity(identifier) where the Relationship terminates"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"relationship_type",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description(`描述源实体与目标实体之间关系的“动词”。建议使用标准化的动词。
**推荐值:** IMPORTS, DEFINES, CALLS, INSTANTIATES, ACCESSES_FIELD, HAS_PARAMETER, IMPLEMENTS, RETURNS_ERROR_FROM`),
		),
		aitool.WithStringParam(
			"decoration_attributes",
			aitool.WithParam_Description("Additional attributes that provide more context about the Relationship."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"decision_rationale",
			aitool.WithParam_Description("Explain the reasoning behind the values assigned to the other fields in this JSON object. Justify the choices made."),
		),
	),
)

func normalizeEntityName(inputStr string) string {
	if strings.TrimSpace(inputStr) == "" {
		return ""
	}
	seps := []string{",", ";", "|", " "}
	processedStr := strings.ToLower(inputStr)
	for _, sep := range seps {
		processedStr = strings.ReplaceAll(processedStr, sep, "_")
	}
	return processedStr
}

//func invokeParams2ERMAttribute(params aitool.InvokeParams) *schema.ERModelAttribute {
//	return &schema.ERModelAttribute{
//		AttributeName:    params.GetString("attribute_name"),
//		AttributeValue:   params.GetString("attribute_value"),
//		UniqueIdentifier: params.GetBool("unique_identifier"),
//	}
//}

func invokeParams2ERMEntity(entityParams aitool.InvokeParams) *schema.ERModelEntity {
	entity := &schema.ERModelEntity{
		EntityName:  normalizeEntityName(entityParams.GetString("identifier")),
		EntityType:  entityParams.GetString("entity_type"),
		Description: entityParams.GetString("description"),
		Rationale:   entityParams.GetString("decision_rationale"),
		Attributes:  map[string]interface{}{},
	}
	for _, attrs := range entityParams.GetObjectArray("attributes") {
		key, value, unique := attrs.GetString("attribute_name"), attrs.GetString("attribute_value"), attrs.GetBool("unique_identifier")
		_ = unique
		entity.Attributes[key] = value
	}
	return entity
}

func Result2ERMAnalysisResult(ermResult *ForgeResult) *ERMAnalysisResult {
	result := &ERMAnalysisResult{
		Entities:      make([]*schema.ERModelEntity, 0),
		Relationships: make([]*TemporaryRelationship, 0),
	}
	for _, entityParams := range ermResult.GetInvokeParamsArray("entity_list") {
		result.Entities = append(result.Entities, invokeParams2ERMEntity(entityParams))
	}

	for _, RelationshipParams := range ermResult.GetInvokeParamsArray("relationship_list") {
		r := &TemporaryRelationship{
			SourceTemporaryName:  RelationshipParams.GetString("source"),
			TargetTemporaryName:  RelationshipParams.GetString("target"),
			RelationshipType:     RelationshipParams.GetString("relationship_type"),
			DecorationAttributes: RelationshipParams.GetString("decoration_attributes"),
			DecisionRationale:    RelationshipParams.GetString("decision_rationale"),
		}
		result.Relationships = append(result.Relationships, r)
	}
	return result
}

func AnalyzeERMFromAnalysisResult(input <-chan AnalysisResult, options ...any) (<-chan *ERMAnalysisResult, error) {
	analyzeConfig := NewAnalysisConfig(options...)
	cm, err := chunkmaker.NewSimpleChunkMaker[AnalysisResult](
		input,
		func(result AnalysisResult) chunkmaker.Chunk {
			return chunkmaker.NewBufferChunk([]byte(result.Dump()))
		},
		chunkmaker.WithCtx(analyzeConfig.Ctx))
	if err != nil {
		return nil, err
	}

	return AnalyzeERMChunkMaker(cm, options...)
}

func AnalyzeERMChunkMaker(cm chunkmaker.ChunkMaker, options ...any) (<-chan *ERMAnalysisResult, error) {
	analyzeConfig := NewAnalysisConfig(options...)
	var domainPrompt string
	var err error
	var detectERMPromptOnce = new(sync.Once)
	var firstMutex = new(sync.Mutex)
	return utils.OrderedParallelProcessSkipError(analyzeConfig.Ctx, cm.OutputChannel(),
		func(i chunkmaker.Chunk) (*ERMAnalysisResult, error) {
			firstMutex.Lock()
			unlockOnce := new(sync.Once)
			detectERMPromptOnce.Do(func() {
				defer func() {
					unlockOnce.Do(func() {
						firstMutex.Unlock()
					})
				}()
				log.Infof("start to detect erm prompt for the first chunk: %s", utils.ShrinkString(string(i.Data()), 800))
				firstChunk := i
				count := 0
				for firstChunk.HaveLastChunk() {
					count++
					firstChunk = firstChunk.LastChunk()
					if count > 10 {
						break
					}
				}
				domainPrompt, err = DetectERMPrompt(string(firstChunk.Data()), options...)
				if err != nil {
					log.Errorf("[detect ERM Prompt] error in analyzing ERM: %v ", err)
				}
			})
			unlockOnce.Do(func() {
				firstMutex.Unlock()
			})
			return AnalyzeERMChunk(domainPrompt, i, options...)
		},
		utils.WithParallelProcessConcurrency(analyzeConfig.AnalyzeConcurrency),
		utils.WithParallelProcessStartCallback(func() {
			analyzeConfig.AnalyzeStatusCard("ERM_ANALYSIS", "start analyzing ERM")
			analyzeConfig.AnalyzeLog("start analyzing ERM concurrency")
		}),
		utils.WithParallelProcessFinishCallback(func() {
			analyzeConfig.AnalyzeStatusCard("ERM_ANALYSIS", "finish analyzing ERM")
			analyzeConfig.AnalyzeLog("finish analyzing ERM concurrency")
		}),
	), nil
}

func AnalyzeERMChunk(domainPrompt string, c chunkmaker.Chunk, options ...any) (*ERMAnalysisResult, error) {
	analyzeConfig := NewAnalysisConfig(options...)
	if domainPrompt == "" {
		prompt, err := DetectERMPrompt(string(c.Data()), options...)
		if err != nil {
			analyzeConfig.AnalyzeLog("[detect ERM Prompt] error in analyzing ERM: %v ", err)
			return nil, err
		}
		domainPrompt = prompt
	}

	query, err := LiteForgeQueryFromChunk(domainPrompt+"\n"+analyzeConfig.ExtraPrompt, c, 200)
	if err != nil {
		analyzeConfig.AnalyzeLog("[build forge query] error in analyzing ERM: %v", err)
		return nil, err
	}

	ermResult, err := _executeLiteForgeTemp(query, append(analyzeConfig.fallbackOptions, _withOutputJSONSchema(ermOutputSchema))...)
	if err != nil {
		analyzeConfig.AnalyzeLog("[analyze erm] error in analyzing ERM: %v", err)
		return nil, err
	}
	result := Result2ERMAnalysisResult(ermResult)
	result.OriginalData = c.Data()
	return result, nil
}

func SaveERMResult(eb *entitybase.EntityRepository, erm *ERMAnalysisResult, options ...any) error {
	analyzeConfig := NewAnalysisConfig(options...)
	swg := utils.NewSizedWaitGroup(analyzeConfig.AnalyzeConcurrency)
	currentEntities := omap.NewOrderedMap[string, *schema.ERModelEntity](map[string]*schema.ERModelEntity{})
	for _, tempEntity := range erm.Entities {
		log.Infof("start to handle raw entity: %v", tempEntity)
		swg.Add(1)
		go func() {
			defer swg.Done()
			tempName := tempEntity.EntityName
			matchedEntity, accurate, err := eb.MatchEntities(tempEntity)
			if err != nil {
				log.Errorf("failed to search entity [%s]: %v", tempEntity.EntityName, err)
				return
			}
			if matchedEntity == nil {
				log.Infof("start to create entity: %s", tempEntity.EntityName)
				err = eb.CreateEntity(tempEntity)
				if err != nil {
					analyzeConfig.AnalyzeLog("failed to create entity [%s]: %v", tempEntity.EntityName, err)
					return
				}
				analyzeConfig.AnalyzeLog("entity created: %s", tempEntity.EntityName)
				currentEntities.Set(tempName, tempEntity)
				return
			}

			var current = tempEntity
			if accurate { // if search is accurate, just use the matched entity
				for s, i := range tempEntity.Attributes {
					matchedEntity.Attributes[s] = i
				}
				current = matchedEntity
			} else {
				resolvedEntity, sameEntity, err := ResolveEntity(matchedEntity, tempEntity, options...)
				if err != nil {
					analyzeConfig.AnalyzeLog("failed to resolve entity [%s]: %v", tempEntity.EntityName, err)
					return
				}
				if sameEntity {
					current = resolvedEntity
				}
			}
			err = eb.SaveEntity(current) // create or update entity
			if err != nil {
				analyzeConfig.AnalyzeLog("failed to update entity [%s]: %v", current.EntityName, err)
				return
			}
			currentEntities.Set(tempName, current)
		}()
	}
	swg.Wait()

	for _, tempShip := range erm.Relationships {
		sourceEntity, ok := currentEntities.Get(tempShip.SourceTemporaryName)
		if !ok {
			analyzeConfig.AnalyzeLog("not found entity [%s], create it", tempShip.SourceTemporaryName)
			sourceEntity = &schema.ERModelEntity{
				EntityName: tempShip.SourceTemporaryName,
			}
			err := eb.CreateEntity(sourceEntity)
			if err != nil {
				analyzeConfig.AnalyzeLog("failed to create entity [%s]: %v", tempShip.SourceTemporaryName, err)
				return err
			}

		}
		targetEntity, ok := currentEntities.Get(tempShip.TargetTemporaryName)
		if !ok {
			analyzeConfig.AnalyzeLog("not found entity [%s], create it", tempShip.SourceTemporaryName)
			targetEntity = &schema.ERModelEntity{
				EntityName: tempShip.SourceTemporaryName,
			}
			err := eb.CreateEntity(targetEntity)
			if err != nil {
				analyzeConfig.AnalyzeLog("failed to create entity [%s]: %v", tempShip.SourceTemporaryName, err)
				return err
			}
		}
		err := eb.AddRelationship(sourceEntity.ID, targetEntity.ID, tempShip.RelationshipType, tempShip.DecisionRationale, map[string]any{
			"decoration_attr": tempShip.DecorationAttributes,
		})
		if err != nil {
			analyzeConfig.AnalyzeLog("failed to create Relationship [%s -> %s]: %v", sourceEntity.EntityName, targetEntity.EntityName, err)
			continue
		}
		analyzeConfig.AnalyzeLog("Relationship created: %s -> %s", sourceEntity.EntityName, targetEntity.EntityName)
	}
	return nil
}

func AnalyzeERM(path string, option ...any) (*entitybase.EntityRepository, error) {
	analyzeConfig := NewRefineConfig(option...)

	analyzeConfig.AnalyzeLog("analyze erm: %s", path)
	analyzeResult, err := AnalyzeFile(path, option...)
	if err != nil {
		return nil, utils.Errorf("failed to start analyze video/doc/txt: %v", err)
	}

	ermResult, err := AnalyzeERMFromAnalysisResult(analyzeResult, option...)
	if err != nil {
		return nil, utils.Errorf("failed to analyze erm from analysis result: %v", err)
	}
	db := consts.GetGormProfileDatabase()
	eb, err := entitybase.NewEntityBase(db, analyzeConfig.KnowledgeBaseName, analyzeConfig.KnowledgeBaseDesc)
	if err != nil {
		return nil, err
	}
	analyzeConfig.AnalyzeStatusCard("ERM_ENTITY_BASE", analyzeConfig.KnowledgeBaseName)

	for erm := range ermResult {
		err := SaveERMResult(eb, erm, option...)
		if err != nil {
			analyzeConfig.AnalyzeLog("failed to save ERM_ENTITY_BASE: %v", err)
			continue
		}
	}
	return eb, nil
}

var resolveEntitySchema = aitool.NewObjectSchemaWithAction(
	aitool.WithBoolParam(
		"same_entity",
		aitool.WithParam_Description("是否是同一个实体"),
	),
	aitool.WithStructParam(
		"entity",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("当实体是同一个实体时，重新综合返回实体的完整信息"),
		},
		entitySchema...,
	),
)

//go:embed liteforge_prompt/resolve_same_entity.txt
var resolveEntityPrompt string

func ResolveEntity(oldEntity *schema.ERModelEntity, newEntity *schema.ERModelEntity, options ...any) (*schema.ERModelEntity, bool, error) {
	analyzeConfig := NewAnalysisConfig(options...)
	analyzeConfig.AnalyzeLog("start resolving entity: old:[%s] | new:[%s]", oldEntity.String(), newEntity.String())

	options = append(options, _withOutputJSONSchema(resolveEntitySchema))
	resolveResult, err := _executeLiteForgeTemp(quickQueryBuild(resolveEntityPrompt, oldEntity.Dump(), newEntity.Dump()), options...)
	if err != nil {
		return nil, false, err
	}

	if resolveResult.GetBool("same_entity") {
		return invokeParams2ERMEntity(resolveResult.GetInvokeParams("entity")), true, nil
	} else {
		return newEntity, false, nil
	}
}
