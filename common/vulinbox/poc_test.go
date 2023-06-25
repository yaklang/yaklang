package vulinbox

import (
	"context"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	netURL "net/url"
	"strings"
	"testing"
)

type vulBoxTester struct {
	serverAdderss   string
	t               *testing.T
	pluginTestCases []*pluginTestCase
}

// 插件的测试用例
type pluginTestCase struct {
	name        string
	riskChecker riskChecker
}
type riskChecker func(risk []*yakit.Risk) error

func getRouteFromUrl(u string) string {
	urlIns, err := netURL.Parse(u)
	if err != nil {
		return ""
	}
	return urlIns.Path
}

// 根据risk的url和title检查, 不允许存在误报和漏报
func newStrictRiskChecker(riskinfos [][2]string, allowPositiveRisks func(route, title string) bool) riskChecker {
	return func(risk []*yakit.Risk) error {
		falsePositiveRisks := make([]*yakit.Risk, 0) // 误报
		checkResult := make(map[*yakit.Risk]bool)
		for _, r := range risk {
			for index, riskinfo := range riskinfos {
				route, riskTitle := riskinfo[0], riskinfo[1]
				riskRoute := getRouteFromUrl(r.Url)
				if riskRoute == route && r.Title == riskTitle {
					checkResult[r] = true
					riskinfos = append(riskinfos[:index], riskinfos[index+1:]...)
					break
				}
			}
			if checkResult[r] == false && allowPositiveRisks(r.Url, r.Title) == false {
				falsePositiveRisks = append(falsePositiveRisks, r)
			}
		}
		res := ""
		if len(falsePositiveRisks) > 0 {
			res += "存在误报: \n"
			for _, r := range falsePositiveRisks {
				res += fmt.Sprintf("url: %s, title: %s\n", r.Url, r.Title)
			}
		}
		if len(riskinfos) > 0 {
			res += "存在漏报: \n"
			for _, r := range riskinfos {
				res += fmt.Sprintf("url: %s, title: %s\n", r[0], r[1])
			}
		}
		if res != "" {
			return fmt.Errorf(res)
		}
		return nil
	}
}
func newPluginTestCase(name string, riskChecker riskChecker) *pluginTestCase {
	return &pluginTestCase{
		name:        name,
		riskChecker: riskChecker,
	}
}

func newVulBoxTester(t *testing.T) *vulBoxTester {
	return &vulBoxTester{
		t: t,
	}
}

func (v *vulBoxTester) addPluginTestCase(testCase *pluginTestCase) {
	v.pluginTestCases = append(v.pluginTestCases, testCase)
}

func (v *vulBoxTester) run() {
	serverAdderss, err := NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}
	v.serverAdderss = serverAdderss
	risks := make(map[string][]*yakit.Risk)
	yaklib.RiskExports["NewRisk"] = func(target string, opts ...yakit.RiskParamsOpt) {
		r := &yakit.Risk{
			Hash: uuid.NewV4().String(),
		}
		for _, opt := range opts {
			opt(r)
		}
		r.Url = target
		risks[r.FromYakScript] = append(risks[r.FromYakScript], r)
	}
	yaklang.Import("risk", yaklib.RiskExports)
	manager, err := yak.NewMixPluginCaller()
	if err != nil {
		log.Error(err)
	}
	manager.SetFeedback(func(i *ypb.ExecResult) error {
		return nil
	})
	plugins := make(map[string]*pluginTestCase)
	for _, testCase := range v.pluginTestCases {
		fileName := testCase.name
		var pluginName = strings.TrimSuffix(fileName, ".yak")
		var scriptBytes = coreplugin.GetCorePluginData(pluginName)
		var name string
		name, err = yakit.CreateTemporaryYakScript("yak", string(scriptBytes))
		if err != nil {
			panic(fmt.Sprintf("create temporary script failed: %s", err))
		}
		plugins[name] = testCase
		var err = manager.LoadPlugin(name)
		if err != nil {
			panic(fmt.Sprintf("load plugin %v failed: %s", pluginName, err))
		}
	}

	manager.SetDividedContext(true)
	manager.SetConcurrent(20)

	ch := make(chan *crawler.Req)

	crawler, err := crawler.NewCrawler(v.serverAdderss, crawler.WithOnRequest(func(req *crawler.Req) {
		ch <- req
	}), crawler.WithMaxDepth(1))
	if err != nil {
		panic(err)
	}
	go func() {
		defer close(ch)
		err := crawler.Run()
		if err != nil {
			log.Error(err)
		}
	}()
	urlPathMap := make(map[string]bool)
	for req := range ch {
		if _, ok := urlPathMap[req.Url()]; ok {
			continue
		} else {
			manager.MirrorHTTPFlowEx(false, req.IsHttps(), req.Url(), req.RequestRaw(), req.ResponseRaw(), req.ResponseBody())
			urlPathMap[req.Url()] = true
		}

	}

	manager.Wait()
	for name, testCase := range plugins {
		yakit.DeleteYakScriptByName(consts.GetGormProjectDatabase(), name)
		err := testCase.riskChecker(risks[name])
		if err != nil {
			v.t.Fatal(utils.Errorf("plugin `%s` failed: %v", testCase.name, err))
		}
	}
}
func TestSSRF(t *testing.T) {
	tester := newVulBoxTester(t)
	tester.addPluginTestCase(newPluginTestCase("启发式SQL注入检测.yak", newStrictRiskChecker([][2]string{
		{"/user/name", "Maybe SQL Injection: [param - type:str value:admin single-quote]"},
		{"/user/id", "Maybe SQL Injection: [param - type:str value:1 single-quote]"},
		{"/user/id", "Union-Based SQL Injection: [id:[1]]"},
		{"/user/id-json", "Maybe SQL Injection: [param - type:str value:1 single-quote]"},
		{"/user/id-json", "Union-Based SQL Injection: [id:[1]]"},
		{"/user/id-b64-json", "Maybe SQL Injection: [param - type:str value:1 single-quote]"},
		{"/user/id-b64-json", "Union-Based SQL Injection: [id:[1]]"},
		{"/user/name", "Union-Based SQL Injection: [name:[admin]]"},
	}, func(url, title string) bool {
		route := getRouteFromUrl(url)
		if strings.Contains(route, "xss") && strings.Contains(title, "Maybe SQL Injection") {
			return true
		}
		return false
	})))
	tester.run()
}
