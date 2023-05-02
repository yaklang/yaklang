package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"yaklang/common/consts"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib"
	"yaklang/common/yakgrpc/yakit"

	"github.com/hpcloud/tail"
)

type PocInvoker struct {
	xrayBinary   string
	xrayPocDir   string
	nucleiBinary string
	nucleiPocDir string

	isXrayReady   *utils.AtomicBool
	isNucleiReady *utils.AtomicBool
	xrayPocList   []*utils.FileInfo
	nucleiPocList []*utils.FileInfo
}

func NewPocInvoker() (*PocInvoker, error) {
	invoker := &PocInvoker{
		xrayBinary:    "",
		xrayPocDir:    "",
		nucleiBinary:  "",
		nucleiPocDir:  "",
		isXrayReady:   utils.NewBool(false),
		isNucleiReady: utils.NewBool(false),
	}

	xrayLocations := BinaryLocations(fmt.Sprintf("xray_%s_%s", runtime.GOOS, runtime.GOARCH), "xray")
	nucleiLocations := BinaryLocations(fmt.Sprintf("nuclei_%s_%s", runtime.GOOS, runtime.GOARCH), "nuclei")

	var err error
	_ = err
	invoker.xrayBinary = utils.GetFirstExistedPath(xrayLocations...)
	invoker.nucleiBinary = utils.GetFirstExistedPath(nucleiLocations...)
	invoker.nucleiPocDir = utils.GetFirstExistedPath(BinaryLocations(
		"nuclei-pocs", "nuclei-poc", "nuclei_pocs", "nuclei_poc")...,
	)
	invoker.xrayPocDir = utils.GetFirstExistedPath(BinaryLocations(
		"xray-pocs", "xray-poc", "xray_pocs", "xray_poc")...,
	)

	/*
		校验 xray 是否安装完毕
	*/
	if invoker.xrayBinary != "" {
		xrayCmd := exec.CommandContext(utils.TimeoutContext(3*time.Second), invoker.xrayBinary, "version")
		raw, err := xrayCmd.CombinedOutput()
		if err != nil {
			invoker.isXrayReady.UnSet()
		}
		utils.Debug(func() {
			log.Infof("xray version banner: \n%s", string(raw))
		})

		r := yaklib.Grok(string(raw), `Version: *%{COMMONVERSION:ver}`)
		xrayVersion := r.Get("ver")
		log.Infof("xray version: %s", xrayVersion)
		infos, _ := utils.ReadDirsRecursively(invoker.xrayPocDir)
		for _, info := range infos {
			if info.IsDir {
				continue
			}

			if strings.Contains(info.Path, ".yaml") || strings.Contains(info.Path, ".yml") {
				invoker.xrayPocList = append(invoker.xrayPocList, info)
			}
		}

		if xrayVersion != "" && len(invoker.xrayPocList) >= 0 {
			invoker.isXrayReady.Set()
		}
	} else {
		log.Warnf("cannot find xray binary, put it in %#v", xrayLocations)
	}

	/*
		校验 nuclei 安装完毕
	*/
	if invoker.nucleiBinary != "" {
		nucleiCmd := exec.CommandContext(utils.TimeoutContext(3*time.Second), invoker.nucleiBinary, "-version")
		raw, _ := nucleiCmd.CombinedOutput()
		r := yaklib.Grok(string(raw), "Current Version: %{COMMONVERSION:ver}")
		nucleiVersion := r.Get("ver")
		log.Infof("nuclei version: %s", nucleiVersion)
		infos, _ := utils.ReadDirsRecursively(invoker.nucleiPocDir)
		for _, info := range infos {
			if info.IsDir {
				continue
			}

			if strings.Contains(info.Path, ".yaml") || strings.Contains(info.Path, ".yml") {
				invoker.nucleiPocList = append(invoker.nucleiPocList, info)
			}
		}

		if nucleiVersion != "" && len(invoker.nucleiPocList) > 5 {
			invoker.isNucleiReady.Set()
		}
	} else {
		log.Warnf("cannot find nuclei binary, put it in %#v", nucleiLocations)
	}

	// 检查执行条件
	if !invoker.isXrayReady.IsSet() && !invoker.isNucleiReady.IsSet() {
		return nil, utils.Errorf("xray and nuclei missed...")
	}

	return invoker, nil
}

func (p *PocInvoker) Exec(urls ...string) ([]*PocVul, error) {
	if len(urls) <= 0 {
		utils.Debug(func() {
			log.Info("empty targets")
		})
		return nil, nil
	}

	rL, err := contentToTmpFileStr(strings.Join(urls, "\n"))
	if err != nil {
		return nil, err
	}

	ctx := utils.TimeoutContext(10 * time.Minute)

	wg := new(sync.WaitGroup)
	wg.Add(2)

	var allVuls []*PocVul
	vulLock := new(sync.Mutex)
	go func() {
		defer wg.Done()
		vuls, err := p.xrayExec(ctx, rL)
		if err != nil {
			log.Errorf("xray exec poc failed: %s", err)
		}
		vulLock.Lock()
		defer vulLock.Unlock()

		allVuls = append(allVuls, vuls...)
	}()

	go func() {
		defer wg.Done()
		vuls, err := p.execNuclei(ctx, rL)
		if err != nil {
			log.Errorf("nuclei ")
		}
		vulLock.Lock()
		defer vulLock.Unlock()
		allVuls = append(allVuls, vuls...)
	}()

	wg.Wait()
	return allVuls, nil
}

func (p *PocInvoker) execNuclei(ctx context.Context, urlFile string) ([]*PocVul, error) {
	f, err := consts.TempFile("nuclei-result-*.jsons")
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	_ = os.RemoveAll(f.Name())

	/**

	./nuclei

	-t exposures -t cves -t vulnerabilities -t misconfiguration -t miscellaneous

	-severity medium,high,critical --retries 0 -l ../targets-local.txt -json -o nuclei-json.txt
	*/
	nucleiOptions := []string{
		"-t", "exposures", "-t", "cves",
		"-t", "vulnerabilities", "-t", "misconfiguration",
		"-t", "miscellaneous",

		"-severity", "medium,high,critical",
		"-retries", "0",

		"-l", urlFile,

		"-json", "-o", f.Name(),
	}

	utils.Debug(func() {
		// 输出调试内容
		nucleiOptions = append(nucleiOptions, "-v")
	})
	ins := exec.CommandContext(ctx, p.nucleiBinary, nucleiOptions...)
	utils.Debug(func() {
		ins.Stdout = os.Stdout
		ins.Stderr = os.Stderr
		log.Infof("nuclei options: %v", nucleiOptions)
		raw, err := ioutil.ReadFile(urlFile)
		if err != nil {
			return
		}
		log.Infof("targets: \n%s", string(raw))
	})

	err = ins.Run()
	if err != nil {
		log.Warnf("nuclei process finished with: %s", err)
	}

	jsonsResult, err := ioutil.ReadFile(f.Name())
	if err != nil {
		log.Warnf("nuclei process json result failed: %s", err)
	}

	utils.Debug(func() {
		log.Infof("nuclei output: %s", jsonsResult)
	})
	return HandleNucleiResult(jsonsResult), nil
}

func (p *PocInvoker) xrayExec(ctx context.Context, urlFile string) ([]*PocVul, error) {
	f, err := consts.TempFile("xray-result-*.json")
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	_ = os.RemoveAll(f.Name())

	xrayOptions := []string{
		"webscan",

		// 锁定目标
		"--url-file", urlFile,

		// 制定 poc 模式
		"--plugin", "phantasm,dirscan",

		// 输出
		"--json-output", f.Name(),
	}
	ins := exec.CommandContext(ctx, p.xrayBinary, xrayOptions...)
	utils.Debug(func() {
		ins.Stdout = os.Stdout
		ins.Stderr = os.Stderr
		log.Infof("xray options: %v", xrayOptions)
		raw, err := ioutil.ReadFile(urlFile)
		if err != nil {
			return
		}
		log.Infof("targets: \n%s", string(raw))
	})

	err = ins.Run()
	if err != nil {
		log.Warnf("xray process finished with: %s", err)
	}

	fileRaw, err := ioutil.ReadFile(f.Name())
	if err != nil {
		log.Warnf("xray process json result failed with: %s", err)
	}

	utils.Debug(func() {
		log.Infof("xray output: %s", fileRaw)
	})
	return HandleXrayResult(fileRaw), nil
}

/*
解析 nuclei 和 xray 的输出结果 （JSON）
*/
func HandleXrayResult(raw []byte) []*PocVul {
	var vuls []*PocVul
	for r := range HandleXrayResultChan(bytes.NewBuffer(raw)) {
		vuls = append(vuls, r)
	}
	return vuls
}

func HandleXrayResultChan(r io.Reader) chan *PocVul {
	ch := make(chan *PocVul)
	go func() {
		defer close(ch)
		results := yaklib.JsonStreamToMapListWithDepth(r, 1)
		for _, res := range results {
			vulRaw, _ := json.Marshal(res)
			target := utils.MapGetMapRaw(res, "target")
			details := utils.MapGetMapRaw(res, "detail")
			p := &PocVul{
				Source:    "xray",
				PocName:   utils.MapGetString(details, "plugin"),
				Target:    utils.MapGetString(target, "url"),
				Timestamp: int64(utils.MapGetFloat64Or(res, "create_time", float64(time.Now().Unix()))),
				Severity:  "high",
				RawJson:   string(vulRaw),
			}
			p.IP, p.Port, _ = utils.ParseStringToHostPort(p.Target)
			p.PocName = utils.MapGetString(res, "plugin")

			ch <- p
		}
	}()
	return ch
}

func PocVulToRisk(p *PocVul) *yakit.Risk {
	var title = fmt.Sprintf("POC[%v] %v", p.Severity, p.PocName)
	var name = p.TitleName
	if name != "" {
		title = fmt.Sprintf("%v %v", name, title)
	}
	var matchedAt = utils.MapGetString(p.Details, "matched-at")
	if matchedAt != "" {
		title = fmt.Sprintf("%v at %v", title, matchedAt)
	}

	return yakit.CreateRisk(
		p.Target,
		yakit.WithRiskParam_Title(title),
		yakit.WithRiskParam_Payload(p.Payload),
		yakit.WithRiskParam_RiskType(fmt.Sprintf("nuclei-%v", p.Tags)),
		yakit.WithRiskParam_Severity(p.Severity),
		yakit.WithRiskParam_Details(p.Details),
	)
}

func HandleNucleiResultFromFile(ctx context.Context, fileName string) (chan *PocVul, error) {
	var vCh = make(chan *PocVul)
	go func() {
		defer close(vCh)

		t, err := tail.TailFile(fileName, tail.Config{Follow: true})
		if err != nil {
			log.Errorf("tail -f %v failed: %s", fileName, err)
		}

		// 清理资源
		defer func() {
			err := t.StopAtEOF()
			if err != nil {
				log.Debugf("stop/close %v failed: %s", fileName, err)
				return
			}
			log.Debugf("close tail -f for %v", fileName)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-t.Lines:
				if !ok {
					return
				}

				vulRawStr := line.Text
				vulRaw := []byte(vulRawStr)
				var res = make(map[string]interface{})
				err := json.Unmarshal(vulRaw, &res)
				if err != nil {
					log.Errorf("unmarshal %v failed: %s", line.Text, err)
					continue
				}

				p := &PocVul{
					Source:    "nuclei",
					PocName:   utils.MapGetString(res, "template-id"),
					Target:    utils.MapGetString(res, "host"),
					Payload:   utils.MapGetString(res, "curl-command"),
					Timestamp: time.Now().Unix(),
					RawJson:   string(vulRaw),
					Details:   res,
				}
				p.IP, p.Port, _ = utils.ParseStringToHostPort(p.Target)
				ts, err := time.Parse(time.RFC3339Nano, utils.MapGetString(res, "timestamp"))
				if err != nil {
					log.Errorf("parse ts string[%v] failed: %s", utils.MapGetString(res, "timestamp"), err)
				}
				p.Timestamp = ts.Unix()

				infoRaw, err := json.Marshal(utils.MapGetMapRaw(res, "info"))
				if err != nil {
					continue
				}

				var vulInfo = make(map[string]interface{})
				err2 := json.Unmarshal(infoRaw, &vulInfo)
				if err2 != nil {
					continue
				}
				p.Severity = utils.MapGetString(vulInfo, "severity")
				tags := utils.MapGetRaw(vulInfo, "tags")
				p.Tags = utils.InterfaceToString(tags)

				// 保存 tags
				risk := PocVulToRisk(p)
				err = yakit.SaveRisk(risk)
				if err != nil {
					log.Errorf("save risk failed: %s", err)
				}
				// 输出内容
				vCh <- p
			}
		}

	}()
	return vCh, nil
}

func HandleNucleiResultFromReader(i io.Reader) chan *PocVul {
	var vuls = make(chan *PocVul)

	go func() {
		defer close(vuls)
		results := yaklib.JsonStreamToMapList(i)
		for _, res := range results {
			vulRaw, _ := json.Marshal(res)
			p := &PocVul{
				Source:    "nuclei",
				Payload:   utils.MapGetString(res, "curl-command"),
				PocName:   utils.MapGetString(res, "template-id"),
				Target:    utils.MapGetString(res, "host"),
				Timestamp: time.Now().Unix(),
				RawJson:   string(vulRaw),
			}
			p.IP, p.Port, _ = utils.ParseStringToHostPort(p.Target)
			ts, err := time.Parse(time.RFC3339Nano, utils.MapGetString(res, "timestamp"))
			if err != nil {
				log.Errorf("parse ts string[%v] failed: %s", utils.MapGetString(res, "timestamp"), err)
			}
			p.Timestamp = ts.Unix()

			infoRaw, err := json.Marshal(utils.MapGetMapRaw(res, "info"))
			if err != nil {
				continue
			}

			var vulInfo = make(map[string]interface{})
			err2 := json.Unmarshal(infoRaw, &vulInfo)
			if err2 != nil {
				continue
			}
			p.Severity = utils.MapGetString(vulInfo, "severity")

			var title = fmt.Sprintf("POC[%v] %v", p.Severity, p.PocName)
			var name = utils.MapGetString(vulInfo, "name")
			if name != "" {
				title = fmt.Sprintf("%v %v", name, title)
			}
			var matchedAt = utils.MapGetString(res, "matched-at")
			if matchedAt != "" {
				title = fmt.Sprintf("%v at %v", title, matchedAt)
			}

			_, _ = yakit.NewRisk(
				p.Target,
				yakit.WithRiskParam_Title(title),
				yakit.WithRiskParam_Payload(p.Payload),
				yakit.WithRiskParam_RiskType(fmt.Sprintf("nuclei-%v", utils.MapGetRaw(vulInfo, "tags"))),
				yakit.WithRiskParam_Severity(p.Severity),
				yakit.WithRiskParam_Details(res),
			)

			vuls <- p
		}
	}()

	return vuls
}

func HandleNucleiResult(raw []byte) []*PocVul {
	var vuls []*PocVul
	for v := range HandleNucleiResultFromReader(bytes.NewBuffer(raw)) {
		vuls = append(vuls, v)
	}
	return vuls
}

type PocVul struct {
	Source    string
	PocName   string
	MatchedAt string
	Target    string
	IP        string
	Port      int
	Timestamp int64
	Payload   string
	Severity  string
	RawJson   string
	Tags      string
	TitleName string
	Details   map[string]interface{}
}
