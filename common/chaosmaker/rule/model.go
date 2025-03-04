package rule

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Storage struct {
	gorm.Model

	RawTrafficBeyondIPPacketBase64  string `json:"raw_traffic_beyond_ip_packet_base64"`
	RawTrafficBeyondLinkLayerBase64 string `json:"raw_traffic_beyond_link_layer_base64"`
	RawTrafficBeyondHTTPBase64      string `json:"raw_traffic_beyond_http_base64"`

	// suricata / http-request
	RuleType string `json:"rule_type"`

	SuricataRaw string `json:"raw"`
	Protocol    string `json:"protocol"`
	Action      string `json:"action"`
	Name        string `json:"name"`
	NameZh      string `json:"name_zh"`
	ClassType   string `json:"class_type"`
	ClassTypeZh string `json:"class_type_zh"`
	Group       string `json:"group"`
	Hash        string `json:"hash" gorm:"unique_index"`

	Keywords      string `json:"keywords"`
	KeywordsZh    string `json:"keywords_zh"`
	Description   string `json:"description"`
	DescriptionZh string `json:"description_zh"`

	RuleUpdatedAt      string `json:"origin_updated_at"`
	RuleCreatedAt      string `json:"origin_created_at"`
	Deployment         string `json:"deployment"`
	SignatureSeverity  string `json:"signature_severity"`
	AttackTarget       string `json:"attack_target"`
	FormerCategory     string `json:"former_category"`
	AffectedProduct    string `json:"affected_product"`
	Tag                string `json:"tag"`
	PerformanceImpact  string `json:"performance_impact"`
	MalwareFamily      string `json:"malware_family"`
	MitreTechniqueID   string `json:"mitre_technique_id"`
	MitreTacticID      string `json:"mitre_tactic_id"`
	MitreTechniqueName string `json:"mitre_technique_name"`
	MitreTacticName    string `json:"mitre_tactic_name"`
	Confidence         string `json:"confidence"`
	ReviewedAt         string `json:"reviewed_at"`
	CVE                string `json:"cve"`
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_PROFILE_DATABASE, &Storage{})
}

func (Storage) TableName() string {
	return "chaos_maker_rules"
}

func QueryRule(db *gorm.DB, req *ypb.QueryChaosMakerRuleRequest) (*bizhelper.Paginator, []*Storage, error) {
	db = db.Model(&Storage{})

	params := req.GetPagination()

	db = bizhelper.ExactQueryString(db, "rule_type", req.GetRuleType())

	if req.GetFromId() > 0 {
		db = db.Where("id > ?", req.GetFromId())
	}

	if req.GetUntilId() > 0 {
		db = db.Where("id < ?", req.GetUntilId())
	}

	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"name", "name_zh", "class_type", "class_type_zh", "group", "keywords", "keywords_zh", "description", "description_zh",
	}, req.GetKeywords(), false)

	db = bizhelper.QueryOrder(db, params.OrderBy, params.Order)

	var ret []*Storage
	paging, db := bizhelper.Paging(db, int(params.Page), int(params.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func (c *Storage) ToGPRCModel() *ypb.ChaosMakerRule {
	return &ypb.ChaosMakerRule{
		Id:                              int64(c.ID),
		RawTrafficBeyondIpPacketBase64:  c.RawTrafficBeyondIPPacketBase64,
		RawTrafficBeyondLinkLayerBase64: c.RawTrafficBeyondLinkLayerBase64,
		RawTrafficBeyondHttpBase64:      c.RawTrafficBeyondHTTPBase64,
		RuleType:                        c.RuleType,
		SuricataRaw:                     c.SuricataRaw,
		Protocol:                        c.Protocol,
		Action:                          c.Action,
		Name:                            strings.Trim(c.Name, `"`),
		NameZh:                          strings.Trim(c.NameZh, `"`),
		ClassType:                       c.ClassType,
		ClassTypeZh:                     c.ClassTypeZh,
		Group:                           c.Group,
		Keywords:                        c.Keywords,
		KeywordsZh:                      c.KeywordsZh,
		Description:                     c.Description,
		DescriptionZh:                   c.DescriptionZh,
		CVE:                             utils.RemoveRepeatStringSlice(utils.PrettifyListFromStringSplitEx(c.CVE, ",", "|")),
	}
}

func (origin *Storage) DecoratedByOpenAI(t string, opts ...aispec.AIConfigOption) {
	ruleIns := origin
	if ruleIns.ID <= 0 {
		existed, err := GetSuricataChaosMakerRuleByHash(consts.GetGormProfileDatabase(), ruleIns.CalcHash())
		if err == nil {
			ruleIns = existed
		}
	}

	// et
	var ok bool
	ruleIns.Name, ok = strings.CutPrefix(ruleIns.Name, "ET ")
	if ok {
		log.Debugf("cut 'et ' prefix failed: %v", ruleIns.Name)
	}
	ruleIns.Name, ok = strings.CutPrefix(ruleIns.Name, "ETPRO ")
	if ok {
		log.Debugf("cut 'et ' prefix failed: %v", ruleIns.Name)
	}

	/*
		这是一个提炼攻击流量特征的任务，请提炼 %v 中的特征关键字（去除引用）？提取成 json array，以方便系统打标签和筛选，提供中文和英文的版本，放在 json 中，以 keywords 和 keywords_zh 作为字段，再描述一下这个特征（中文50字以内，去除‘检测’等字段意图），作为 description_zh 字段，同时补充他的 description（英文）
	*/
	var clearData string
	switch ruleIns.RuleType {
	case "suricata":
		clearData = strconv.Quote(ruleIns.SuricataRaw)
	case "http-request":
		raw, _ := codec.DecodeBase64(ruleIns.RawTrafficBeyondHTTPBase64)
		if raw != nil {
			clearData = strconv.Quote(string(raw))
		}
		return
	case "tcp":
		raw, _ := codec.DecodeBase64(ruleIns.RawTrafficBeyondIPPacketBase64)
		if raw != nil {
			clearData = strconv.Quote(string(raw))
		}
		return
	default:
		log.Errorf("unknown rule type: %v", ruleIns.RuleType)
		return
	}

	if ruleIns.CVE == "" && ruleIns.SuricataRaw != "" {
		records := regexp.MustCompile(`(?i)CVE-\d+-\d+`).FindAllString(ruleIns.SuricataRaw, -1)
		records = utils.RemoveRepeatStringSlice(strings.Split(strings.ToUpper(strings.Join(records, ",")), ","))
		if len(records) > 0 {
			ruleIns.CVE = strings.Join(records, ",")
		}
	}

	if ruleIns.Keywords == "" || ruleIns.KeywordsZh == "" || ruleIns.NameZh == "" {
		agent := ai.GetAI(t, opts...)
		if agent == nil {
			log.Errorf("cannot get ai type: %s", t)
			return
		}
		raw, err := agent.ExtractData(clearData, "从规则内的流量特征中提取关键信息", map[string]any{
			"keywords":       "所有规则中的特征关键字，数量大概5个，关键字长度不要超过4个词，注意要去除引用，以','分隔", //  gpt4不需要这个提示 `一定要重点注意不要提取suricata语法的关键字，如alert, content, sid等（英文）`
			"keywords_zh":    "keywords的中文翻译",
			"description":    "描述这个规则的作用，必须要说清楚此规则描述了什么样的规则，尽量详细（英文）",
			"description_zh": "description的中文翻译",
			"name_zh":        "msg的中文翻译",
		})
		if err != nil {
			log.Errorf("openai extract data by ai failed: %s with:\n    %v\n\n", err, clearData)
			return
		}
		log.Infof("find raw answer: %v", raw)
		if ruleIns.NameZh == "" {
			ruleIns.NameZh = utils.MapGetString(raw, "name_zh")
		}
		ruleIns.Keywords = strings.Join(utils.InterfaceToStringSlice(strings.Split(utils.MapGetString(raw, "keywords"), ",")), "|")
		ruleIns.KeywordsZh = strings.Join(utils.InterfaceToStringSlice(strings.Split(utils.MapGetString(raw, "keywords_zh"), "，")), "|")
		ruleIns.Description = utils.MapGetString(raw, "description")
		ruleIns.DescriptionZh = utils.MapGetString(raw, "description_zh")
	}
	//err := UpsertRule(db, c.Hash, c)
	//if err != nil {
	//	log.Warn(err)
	//}
}

func DecorateRules(t string, concurrent int, proxy string) {
	db := consts.GetGormProfileDatabase()
	swg := utils.NewSizedWaitGroup(concurrent)
	db = db.Where("name_zh = '' or name_zh is null")
	for r := range YieldRules(db, context.Background()) {
		swg.Add()
		r := r
		go func() {
			defer swg.Done()
			if r.RuleType != "suricata" {
				return
			}
			raw, err := strconv.Unquote(r.SuricataRaw)
			if err != nil {
				raw = string(r.SuricataRaw)
			}
			suricataRule, err := rule.Parse(raw)
			if err != nil || len(suricataRule) > 0 {
				target := suricataRule[0]
				if target.Action == "" || target.Protocol == "" || target.Message == "" || target.DestinationPort == nil || target.SourceAddress == nil {
					log.Errorf("parse suricata rule failed: %s, remove bad rule", err, raw)
					DeleteSuricataRuleByID(consts.GetGormProfileDatabase(), int64(r.ID))
					return
				}
			} else if len(suricataRule) == 0 {
				DeleteSuricataRuleByID(consts.GetGormProfileDatabase(), int64(r.ID))
				return
			}
			r.DecoratedByOpenAI(t, aispec.WithProxy(proxy))
			err = UpsertRule(db, r.Hash, r)
			if err != nil {
				log.Errorf("upsert rule failed: %s", err)
			}
		}()
	}
	swg.Wait()
}

func (c *Storage) CalcHash() string {
	c.Hash = utils.CalcSha1(
		c.RawTrafficBeyondIPPacketBase64,
		c.RawTrafficBeyondLinkLayerBase64,
		c.RawTrafficBeyondHTTPBase64,
		c.SuricataRaw,
		c.Group,
	)
	return c.Hash
}

func (c *Storage) BeforeSave() error {
	if c.Hash == "" {
		c.CalcHash()
	}
	return nil
}

func NewRuleFromSuricata(s *rule.Rule) *Storage {
	return &Storage{
		SuricataRaw:        s.Raw,
		Protocol:           s.Protocol,
		RuleType:           "suricata",
		Action:             s.Action,
		Name:               s.Message,
		ClassType:          s.ClassType,
		NameZh:             s.MessageChinese,
		RuleUpdatedAt:      s.RuleUpdatedAt,
		RuleCreatedAt:      s.RuleCreatedAt,
		Deployment:         s.Deployment,
		SignatureSeverity:  s.SignatureSeverity,
		AttackTarget:       s.AttackTarget,
		FormerCategory:     s.FormerCategory,
		AffectedProduct:    s.AffectedProduct,
		Tag:                s.Tag,
		PerformanceImpact:  s.PerformanceImpact,
		MalwareFamily:      s.MalwareFamily,
		MitreTechniqueID:   s.MitreTechniqueID,
		MitreTacticID:      s.MitreTacticID,
		MitreTechniqueName: s.MitreTechniqueName,
		MitreTacticName:    s.MitreTacticName,
		Confidence:         s.Confidence,
		ReviewedAt:         s.ReviewedAt,
		CVE:                strings.Join(utils.RemoveRepeatStringSlice(utils.PrettifyListFromStringSplitEx(s.CVE, ",", "|")), ","),
	}
}

func SaveSuricata(db *gorm.DB, s *rule.Rule) error {
	r := NewRuleFromSuricata(s)
	return UpsertRule(db, r.CalcHash(), r)
}

func NewHTTPRequestRule(name string, raw []byte) *Storage {
	return &Storage{
		Model:                      gorm.Model{},
		RawTrafficBeyondHTTPBase64: codec.EncodeBase64(raw),
		RuleType:                   "http-request",
		Protocol:                   "http",
		Action:                     "alert",
		Name:                       name,
	}
}

func SaveHTTPRequest(db *gorm.DB, name string, raw []byte) error {
	r := NewHTTPRequestRule(name, raw)
	return UpsertRule(db, r.CalcHash(), r)
}

func SaveTCPTraffic(db *gorm.DB, name string, raw []byte) error {
	r := &Storage{
		Model:                          gorm.Model{},
		RawTrafficBeyondIPPacketBase64: codec.EncodeBase64(raw),
		RuleType:                       "tcp",
		Protocol:                       "tcp",
		Action:                         "alert",
		Name:                           name,
	}
	return UpsertRule(db, r.CalcHash(), r)
}

func SaveICMPTraffic(db *gorm.DB, name string, raw []byte) error {
	r := &Storage{
		Model:                          gorm.Model{},
		RawTrafficBeyondIPPacketBase64: codec.EncodeBase64(raw),
		RuleType:                       "icmp",
		Protocol:                       "icmp",
		Action:                         "alert",
		Name:                           name,
	}
	return UpsertRule(db, r.CalcHash(), r)
}

var saveChaosMaker sync.Mutex

func SaveToDB(rule *Storage) error {
	return UpsertRule(consts.GetGormProfileDatabase(), rule.CalcHash(), rule)
}

func UpsertRule(db *gorm.DB, hash string, i interface{}) error {
	saveChaosMaker.Lock()
	defer saveChaosMaker.Unlock()

	db = db.Model(&Storage{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&Storage{}); db.Error != nil {
		return utils.Errorf("create/update Storage failed: %s", db.Error)
	}

	return nil
}

func GetSuricataChaosMakerRule(db *gorm.DB, id int64) (*Storage, error) {
	var req Storage
	if db := db.Model(&Storage{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Storage failed: %s", db.Error)
	}

	return &req, nil
}

func GetSuricataChaosMakerRuleByHash(db *gorm.DB, hash string) (*Storage, error) {
	var req Storage
	if db := db.Model(&Storage{}).Where("hash = ?", hash).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Storage failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteSuricataRuleByID(db *gorm.DB, id int64) error {
	if db := db.Model(&Storage{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&Storage{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func YieldRules(db *gorm.DB, ctx context.Context) chan *Storage {
	return bizhelper.YieldModel[*Storage](ctx, db, bizhelper.WithYieldModel_PageSize(1000))
}

func ExportRulesToFile(db *gorm.DB, fileName string) error {
	fp, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		return err
	}
	defer fp.Close()
	for result := range YieldRules(db.Model(&Storage{}), context.Background()) {
		result.ID = 0
		raw, err := json.Marshal(result)
		if err != nil {
			log.Errorf("marshal rules failed: %s", err)
			continue
		}
		rawMap := make(map[string]any)
		if err := json.Unmarshal(raw, &rawMap); err != nil {
			log.Errorf("unmarshal rules failed: %s", err)
			continue
		}
		delete(rawMap, "ID")
		delete(rawMap, "CreatedAt")
		delete(rawMap, "DeletedAt")
		delete(rawMap, "UpdatedAt")
		raw, _ = json.Marshal(rawMap)
		fp.Write(raw)
		fp.Write([]byte("\n"))
	}
	return nil
}

func ImportRulesFromFile(db *gorm.DB, fileName string) error {
	fp, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer fp.Close()

	raw, _ := io.ReadAll(fp)
	for result := range utils.ParseLines(string(raw)) {
		var rule Storage
		if err := json.Unmarshal([]byte(result), &rule); err != nil {
			log.Errorf("unmarshal rules failed: %s", err)
			continue
		}
		rule.ID = 0
		rule.DeletedAt = nil
		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()
		if err := UpsertRule(db, rule.Hash, rule); err != nil {
			log.Errorf("create/update rules failed: %s", err)
			continue
		}
	}
	return nil
}
