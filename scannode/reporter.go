package scannode

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm/dialects/postgres"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
)

type ScannerAgentReporter struct {
	TaskId    string
	SubTaskId string
	RuntimeId string

	agent *ScanNode
}

var streamDebugCount int32
var chunkInspectCount int32
var chunkAssembleCount int32

type ssaChunkBuffer struct {
	total    int
	chunks   map[int]string
	received int
	updated  time.Time
}

var (
	ssaChunkMu      sync.Mutex
	ssaChunkPool    = make(map[string]*ssaChunkBuffer)
	chunkDebugCount int32
)

// convertRawToString 将原始数据转换为字符串，处理 JSON 反序列化后的各种数据格式
func convertRawToString(raw interface{}) string {
	if raw == nil {
		return ""
	}
	switch data := raw.(type) {
	case []byte:
		return string(data)
	case []interface{}:
		// JSON 反序列化后的数组，每个元素是 float64
		bytes := make([]byte, len(data))
		for i, v := range data {
			if f, ok := v.(float64); ok {
				bytes[i] = byte(f)
			}
		}
		return string(bytes)
	case string:
		return data
	default:
		return utils.InterfaceToString(raw)
	}
}

func parseChunkFields(m map[string]interface{}) (key string, index int, total int, data string, ok bool) {
	if m == nil {
		return "", 0, 0, "", false
	}
	key = utils.InterfaceToString(utils.MapGetFirstRaw(m, "chunk_key", "chunkKey", "ChunkKey"))
	if key == "" {
		return "", 0, 0, "", false
	}
	index = int(utils.InterfaceToFloat64(utils.MapGetFirstRaw(m, "chunk_index", "chunkIndex", "ChunkIndex")))
	total = int(utils.InterfaceToFloat64(utils.MapGetFirstRaw(m, "chunk_total", "chunkTotal", "ChunkTotal")))
	data = utils.InterfaceToString(utils.MapGetFirstRaw(m, "chunk_data", "chunkData", "ChunkData"))
	if total <= 0 || data == "" {
		return key, index, total, data, false
	}
	return key, index, total, data, true
}

func emitSSAChunk(taskId, runtimeId, subTaskId string, emitter *StreamEmitter, chunkKey string, chunkIndex, chunkTotal int, chunkData string) bool {
	if emitter == nil || !emitter.Enabled() {
		return false
	}
	fullKey := taskId + ":" + chunkKey
	ssaChunkMu.Lock()
	buf := ssaChunkPool[fullKey]
	if buf == nil {
		buf = &ssaChunkBuffer{
			total:  chunkTotal,
			chunks: make(map[int]string),
		}
		ssaChunkPool[fullKey] = buf
	}
	if _, ok := buf.chunks[chunkIndex]; !ok {
		buf.chunks[chunkIndex] = chunkData
		buf.received++
	}
	buf.updated = time.Now()
	complete := buf.received >= buf.total
	ssaChunkMu.Unlock()

	if complete {
		ssaChunkMu.Lock()
		buf = ssaChunkPool[fullKey]
		delete(ssaChunkPool, fullKey)
		ssaChunkMu.Unlock()
		if buf != nil {
			var sb strings.Builder
			for i := 0; i < buf.total; i++ {
				sb.WriteString(buf.chunks[i])
			}
			reportJSON := sb.String()
			if reportJSON != "" {
				if atomic.AddInt32(&chunkAssembleCount, 1) <= 5 {
					log.Infof("stream ssa chunk assembled task=%s len=%d chunks=%d", taskId, len(reportJSON), buf.total)
				}
				if atomic.AddInt32(&streamDebugCount, 1) <= 3 {
					log.Infof("stream ssa report begin task=%s len=%d", taskId, len(reportJSON))
				}
				streamErr := emitter.EmitSSAReportJSON(taskId, runtimeId, subTaskId, reportJSON)
				if streamErr == nil {
					if atomic.AddInt32(&streamDebugCount, 1) <= 3 {
						log.Infof("stream ssa report sent task=%s", taskId)
					}
					return true
				}
				log.Errorf("stream ssa report failed: %v", streamErr)
			}
		}
		return true
	}
	return true
}

func NewScannerAgentReporter(taskId string, subTaskId string, runtimeId string, agent *ScanNode) *ScannerAgentReporter {
	return &ScannerAgentReporter{
		TaskId:    taskId,
		SubTaskId: subTaskId,
		RuntimeId: runtimeId,
		agent:     agent,
	}
}

func (r *ScannerAgentReporter) Report(record *schema.Report) error {
	if r.agent != nil {
		raw, err := json.Marshal(record)
		if err != nil {
			return utils.Errorf("marshal report failed: %s", err)
		}
		r.agent.feedback(&spec.ScanResult{
			Type:      spec.ScanResult_Report,
			Content:   raw,
			TaskId:    r.TaskId,
			RuntimeId: r.RuntimeId,
			SubTaskId: r.SubTaskId,
		})
	}
	return nil
}

func (r *ScannerAgentReporter) ReportWeakPassword(result interface{}) error {
	var s = r.agent
	switch ret := result.(type) {
	case *bruteutils.BruteItemResult:
		host, port, err := utils.ParseStringToHostPort(ret.Target)
		if err != nil {
			return err
		}

		if !utils.IsIPv4(host) {
			return r.ReportRisk(
				fmt.Sprintf(
					"%v WeakPassword for %v: %v/%v",
					strings.ToUpper(ret.Type),
					ret.Target, ret.Username, ret.Password,
				),
				ret.Target, utils.Jsonify(ret),
			)
		}

		var targetType = VulnTargetType_Service
		if utils.IsHttpOrHttpsUrl(ret.Target) {
			targetType = VulnTargetType_Url
		}

		vul := &Vuln{
			IPAddr:       host,
			Host:         host,
			Port:         port,
			IsPrivateNet: utils.IsPrivateIP(net.ParseIP(host)),
			Target:       ret.Target,
			TargetRaw: postgres.Jsonb{RawMessage: utils.Jsonify(map[string]interface{}{
				"target": ret.Target, "host": host, "port": port,
			})},
			TargetType: targetType,
			Plugin: strings.Join([]string{
				"weakpassword", strings.ToLower(ret.Type),
			}, "/"),
			Detail: postgres.Jsonb{RawMessage: utils.Jsonify(ret)},
		}
		res, err := NewVulnResult(vul)
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		s.feedback(res)
		return nil
	default:
		return utils.Errorf("unsupported: %v", spew.Sdump(ret))
	}
}

func (r *ScannerAgentReporter) ReportRisk(
	title string, target string, details interface{},
	tags ...string,
) error {
	var s = r.agent
	//details, err := json.Marshal(details)
	//if err != nil {
	//	return err
	//}

	vul := &Vuln{
		Target: target,
		TargetRaw: postgres.Jsonb{RawMessage: utils.Jsonify(map[string]interface{}{
			"target": target, "title": title,
		})},
		TargetType: VulnTargetType_Risk,
		Plugin:     strings.Join(append([]string{"risk"}, tags...), "/"),
	}
	var detailsMap map[string]interface{}
	switch t := details.(type) {
	case map[string]interface{}:
		detailsMap = t
	case string:
		rawStr := strings.TrimSpace(t)
		if rawStr != "" {
			if strings.HasPrefix(rawStr, "\"") && strings.HasSuffix(rawStr, "\"") {
				if unq, err := strconv.Unquote(rawStr); err == nil {
					rawStr = unq
				}
			}
			_ = json.Unmarshal([]byte(rawStr), &detailsMap)
		}
	case []byte:
		rawStr := strings.TrimSpace(string(t))
		if rawStr != "" {
			_ = json.Unmarshal([]byte(rawStr), &detailsMap)
		}
	case json.RawMessage:
		rawStr := strings.TrimSpace(string(t))
		if rawStr != "" {
			_ = json.Unmarshal([]byte(rawStr), &detailsMap)
		}
	}

	if v := detailsMap; v != nil {
		riskType := utils.InterfaceToString(utils.MapGetFirstRaw(
			v,
			"RiskType", "risk_type", "riskType", "type", "Type",
		))
		if chunkKey, chunkIndex, chunkTotal, chunkData, ok := parseChunkFields(v); chunkKey != "" && r.agent != nil && r.agent.streamer != nil && r.agent.streamer.Enabled() {
			if !ok {
				log.Warnf("stream ssa chunk skipped: missing fields task=%s key=%s idx=%d total=%d", r.TaskId, chunkKey, chunkIndex, chunkTotal)
				return nil
			}
			if atomic.AddInt32(&chunkDebugCount, 1) <= 3 {
				log.Infof("stream ssa chunk received task=%s key=%s idx=%d/%d len=%d", r.TaskId, chunkKey, chunkIndex, chunkTotal, len(chunkData))
			}
			_ = emitSSAChunk(r.TaskId, r.RuntimeId, r.SubTaskId, r.agent.streamer, chunkKey, chunkIndex, chunkTotal, chunkData)
			return nil
		}

		detailRaw := utils.MapGetFirstRaw(v, "Details", "Detail", "details", "detail")
		if atomic.AddInt32(&chunkInspectCount, 1) <= 3 {
			rawStr := utils.InterfaceToString(detailRaw)
			log.Infof("risk_details_debug task=%s riskType=%s detailType=%T detailLen=%d has_chunk_key=%v",
				r.TaskId, riskType, detailRaw, len(rawStr), strings.Contains(rawStr, "chunk_key"))
		}
		var lib map[string]interface{}
		switch t := detailRaw.(type) {
		case map[string]interface{}:
			lib = t
		case string:
			if t != "" {
				rawStr := t
				if strings.HasPrefix(rawStr, "\"") && strings.HasSuffix(rawStr, "\"") {
					if unq, err := strconv.Unquote(rawStr); err == nil {
						rawStr = unq
					}
				}
				_ = json.Unmarshal([]byte(rawStr), &lib)
			}
		}
		if lib == nil {
			lib = make(map[string]interface{})
		}
		if chunkKey, chunkIndex, chunkTotal, chunkData, ok := parseChunkFields(lib); chunkKey != "" && r.agent != nil && r.agent.streamer != nil && r.agent.streamer.Enabled() {
			if !ok {
				log.Warnf("stream ssa chunk skipped: missing fields task=%s key=%s idx=%d total=%d", r.TaskId, chunkKey, chunkIndex, chunkTotal)
				return nil
			}
			if atomic.AddInt32(&chunkDebugCount, 1) <= 3 {
				log.Infof("stream ssa chunk received task=%s key=%s idx=%d/%d len=%d", r.TaskId, chunkKey, chunkIndex, chunkTotal, len(chunkData))
			}
			_ = emitSSAChunk(r.TaskId, r.RuntimeId, r.SubTaskId, r.agent.streamer, chunkKey, chunkIndex, chunkTotal, chunkData)
			return nil
		}

		if riskType == "ssa-risk" && r.agent != nil && r.agent.streamer != nil && r.agent.streamer.Enabled() {
			if data := utils.MapGetFirstRaw(lib, "data", "Data"); data != nil {
				reportJSON := utils.InterfaceToString(data)
				if reportJSON == "" {
					if enc := utils.MapGetString(lib, "data_gzip_b64"); enc != "" {
						if raw, err := codec.DecodeBase64(enc); err == nil {
							if ungzip, err := utils.GzipDeCompress(raw); err == nil {
								reportJSON = string(ungzip)
							} else {
								log.Errorf("stream ssa report gzip decode failed: %v", err)
							}
						} else {
							log.Errorf("stream ssa report base64 decode failed: %v", err)
						}
					}
				}
				if reportJSON != "" {
					if atomic.AddInt32(&streamDebugCount, 1) <= 3 {
						log.Infof("stream ssa report begin task=%s len=%d", r.TaskId, len(reportJSON))
					}
					if streamErr := r.agent.streamer.EmitSSAReportJSON(r.TaskId, r.RuntimeId, r.SubTaskId, reportJSON); streamErr == nil {
						if atomic.AddInt32(&streamDebugCount, 1) <= 3 {
							log.Infof("stream ssa report sent task=%s", r.TaskId)
						}
						return nil
					} else {
						log.Errorf("stream ssa report failed: %v", streamErr)
					}
				}
			}
			if len(lib) == 0 {
				keys := make([]string, 0, len(v))
				for k := range v {
					keys = append(keys, k)
				}
				log.Warnf("stream ssa report skipped: empty details riskType=%s keys=%v", riskType, keys)
			}
		}

		if ip := net.ParseIP(utils.FixForParseIP(utils.MapGetString(v, "IP"))); ip != nil {
			vul.IPAddr = ip.String()
			if i, err := utils.IPv4ToUint32(ip.To4()); err == nil {
				vul.IPv4Int = uint32(i)
			}
			vul.IsPrivateNet = utils.IsPrivateIP(ip)
		}
		vul.Host = utils.MapGetString(v, "Host")
		vul.Port = int(utils.MapGetFloat64(v, "Port"))
		vul.Hash = utils.MapGetString(v, "Hash")
		vul.FromThreatAnalysisRuntimeId = utils.MapGetString(v, "RuntimeId")
		vul.FromThreatAnalysisTaskId = r.TaskId
		vul.Detail = postgres.Jsonb{RawMessage: utils.Jsonify(lib)}
		vul.Payload = utils.MapGetString(v, "Payload")
		vul.RiskTypeVerbose = utils.MapGetString(v, "RiskTypeVerbose")
		vul.RiskType = utils.MapGetString(v, "RiskType")
		vul.Severity = utils.MapGetString(v, "Severity")
		vul.FromYakScript = utils.MapGetString(v, "FromYakScript")
		vul.TitleVerbose = utils.MapGetString(v, "TitleVerbose")
		vul.Title = utils.MapGetString(v, "Title")
		vul.ReverseToken = utils.MapGetString(v, "ReverseToken")
		vul.Url = utils.MapGetString(v, "Url")
		vul.Description = utils.MapGetString(v, "Description")
		vul.Solution = utils.MapGetString(v, "Solution")
		vul.Request = convertRawToString(utils.MapGetRaw(v, "Request"))
		vul.Response = convertRawToString(utils.MapGetRaw(v, "Response"))
		vul.Parameter = utils.MapGetString(v, "Parameter")
		vul.IsPotential = utils.MapGetBool(v, "IsPotential")
		vul.CVE = utils.MapGetString(v, "CVE")
		vul.CveAccessVector = utils.MapGetString(v, "CveAccessVector")
		vul.CveAccessComplexity = utils.MapGetString(v, "CveAccessComplexity")
	}
	res, err := NewVulnResult(vul)
	if err != nil {
		return err
	}
	res.RuntimeId = r.RuntimeId
	res.TaskId = r.TaskId
	res.SubTaskId = r.SubTaskId
	s.feedback(res)
	return nil
}

func (r *ScannerAgentReporter) ReportVul(i interface{}) error {
	var s = r.agent
	switch ret := i.(type) {
	case *tools.PocVul:
		var targetType = VulnTargetType_Service
		if strings.HasPrefix(strings.TrimSpace(ret.Target), "http://") ||
			strings.HasPrefix(strings.TrimSpace(ret.Target), "https://") {
			targetType = VulnTargetType_Url
		}

		raw, _ := json.Marshal(ret)
		targetRaw, _ := json.Marshal(map[string]interface{}{
			"ip":     ret.IP,
			"port":   ret.Port,
			"target": ret.Target,
		})

		res, err := NewVulnResult(&Vuln{
			IPAddr:       ret.IP,
			Host:         ret.IP,
			Port:         ret.Port,
			IsPrivateNet: utils.IsPrivateIP(net.ParseIP(ret.IP)),
			Target:       ret.Target,
			TargetRaw:    postgres.Jsonb{RawMessage: targetRaw},
			TargetType:   targetType,
			Plugin:       fmt.Sprintf("palm-poc-invoker/%v/%v", ret.Source, ret.PocName),
			Detail:       postgres.Jsonb{RawMessage: raw},
		})
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		s.feedback(res)
		return nil
	case *Vuln:
		res, err := NewVulnResult(ret)
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		s.feedback(res)
		return nil
	default:
		return utils.Errorf("unsupported: %s", spew.Sdump(i))
	}
}

func (r *ScannerAgentReporter) ReportFingerprint(i interface{}) error {
	var s = r.agent
	switch ret := i.(type) {
	case *fp.MatchResult:
		res, err := spec.NewScanFingerprintResult(ret)
		if err != nil {
			return err
		}
		res.RuntimeId = r.RuntimeId
		res.TaskId = r.TaskId
		res.SubTaskId = r.SubTaskId
		s.feedback(res)
		return nil
	default:
		return utils.Errorf("unsupported: %v", spew.Sdump(i))
	}
}

func (r *ScannerAgentReporter) ReportProcess(process float64) error {
	res, err := spec.NewScanProcessResult(process)
	if err != nil {
		return err
	}
	res.RuntimeId = r.RuntimeId
	res.TaskId = r.TaskId
	res.SubTaskId = r.SubTaskId
	r.agent.feedback(res)
	return nil
}

// ReportProcessWithSubTask reports a progress update under a different subtask id.
// This is useful when the script emits multiple progress streams via SetProgressEx(id, ...):
// we don't want non-"main" progress resets to shrink the overall progress in the UI.
func (r *ScannerAgentReporter) ReportProcessWithSubTask(subTaskId string, process float64) error {
	if strings.TrimSpace(subTaskId) == "" {
		return r.ReportProcess(process)
	}
	res, err := spec.NewScanProcessResult(process)
	if err != nil {
		return err
	}
	res.RuntimeId = r.RuntimeId
	res.TaskId = r.TaskId
	res.SubTaskId = subTaskId
	r.agent.feedback(res)
	return nil
}
func (r *ScannerAgentReporter) ReportTCPOpenPort(host interface{}, port interface{}) error {
	var s = r.agent

	hostRaw, portInt, err := utils.ParseStringToHostPort(utils.HostPort(fmt.Sprint(host), port))
	if err != nil {
		return err
	}

	ip := net.ParseIP(utils.FixForParseIP(hostRaw))
	if ip == nil {
		return utils.Errorf("IP parse failed: %s", spew.Sdump(host))
	}
	res, err := spec.NewScanTCPOpenPortResult(ip, portInt, spec.PortStateType_Open)
	if err != nil {
		return err
	}
	res.RuntimeId = r.RuntimeId
	res.TaskId = r.TaskId
	res.SubTaskId = r.SubTaskId
	s.feedback(res)
	return nil
}
