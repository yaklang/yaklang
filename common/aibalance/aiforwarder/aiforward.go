package aiforwarder

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"os"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/tcpreverse"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"gopkg.in/yaml.v3"
)

type Rule struct {
	SNI       string `json:"sni" yaml:"sni"`
	EnableTLS bool   `json:"enable_tls" yaml:"enable_tls"`
	Target    string `json:"target" yaml:"target"`
}

type AIForward struct {
	root  string
	rules []*Rule
}

func (a *AIForward) Run() error {
	if a.root == "" {
		return utils.Error("no root path, cannot register to aibalance")
	}

	if len(a.rules) <= 0 {
		return utils.Errorf("no rules, cannot run forwarder")
	}

	rsp, _, err := poc.DoPOST(a.root, poc.WithJSON(a.rules))
	if err != nil {
		return utils.Errorf("failed to register forwarder: %s", err)
	}

	type registerResponse struct {
		CA  string `json:"ca,omitempty"`
		Crt string `json:"crt,omitempty"`
		Key string `json:"key,omitempty"`
	}
	var rspBody registerResponse
	var body = rsp.GetBody()
	err = json.Unmarshal(body, &rspBody)
	if err != nil {

		return utils.Errorf("failed to parse register response: %s, with: \n%v", err, utils.ShrinkString(spew.Sdump(body), 200))
	}

	tconfig, err := tlsutils.GetX509ServerTlsConfig([]byte(rspBody.CA), []byte(rspBody.Crt), []byte(rspBody.Key))
	if err != nil {
		return utils.Errorf("failed to get tls config: %s", err)
	}

	reverse := tcpreverse.NewTCPReverseWithTLSConfig(443, tconfig)

	for _, rule := range a.rules {
		reverse.RegisterSNIForward(rule.SNI, &tcpreverse.TCPReverseTarget{
			ForceTLS: rule.EnableTLS,
			Address:  rule.Target,
		})
	}

	return reverse.Run()
}

func NewAIForwarder(root string) *AIForward {
	return &AIForward{
		root: root,
	}
}

func (a *AIForward) AddRule(sni string, enableTLS bool, target string) {
	a.rules = append(a.rules, &Rule{
		SNI:       sni,
		EnableTLS: enableTLS,
		Target:    target,
	})
}

func (a *AIForward) LoadFromYaml(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Errorf("failed to read yaml file: %s, err: %v", path, err)
		return utils.Errorf("failed to read yaml file: %s", err)
	}
	var rules []*Rule
	err = yaml.Unmarshal(raw, &rules)
	if err != nil {
		log.Errorf("failed to unmarshal yaml file: %s, err: %v", path, err)
		return utils.Errorf("failed to unmarshal yaml file: %s", err)
	}

	a.rules = append(a.rules, rules...)
	log.Infof("Loaded %d rules from yaml file: %s, total rules: %d", len(rules), path, len(a.rules))
	return nil
}
