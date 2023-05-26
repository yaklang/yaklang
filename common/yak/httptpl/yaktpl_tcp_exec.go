package httptpl

import (
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strings"
	"time"
)

func (y *YakNetworkBulkConfig) handleConn(
	conn net.Conn, lowhttpConfig *lowhttp.LowhttpExecConfig,
	vars map[string]any, callback func(rsp []byte, matched bool, extractorResults map[string]any),
) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Error(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	if lowhttpConfig.Ctx != nil {
		conn = ctxio.NewConn(lowhttpConfig.Ctx, conn)
	}

	var (
		extractorResults  = make(map[string]any)
		availableResponse []string
	)

	var err error
	if len(y.Inputs) > 0 {
	REQ:
		for _, inputElement := range y.Inputs {
			var raw []byte
			switch strings.ToLower(strings.TrimSpace(inputElement.Type)) {
			case "hex":
				raw, err = codec.DecodeHex(inputElement.Data)
				if err != nil {
					log.Errorf("network.inputs codec.DecodeHex failed: %s", err)
					break REQ
				}
			case "base64":
				raw, err = codec.DecodeBase64(inputElement.Data)
				if err != nil {
					log.Errorf("network.inputs codec.DecodeBase64 failed: %s", err)
					break REQ
				}
			default:
				raw = []byte(inputElement.Data)
			}

			if len(raw) > 0 {
				conn.Write(raw)
			}
			bufferSize := inputElement.Read
			if bufferSize <= 0 {
				bufferSize = y.ReadSize
			}
			response := utils.StableReaderEx(conn, 5*time.Second, bufferSize)
			for _, extractor := range y.Extractor {
				extractorVars, err := extractor.Execute(response)
				if err != nil {
					log.Warnf("YakNetworkBulkConfig extractor.Execute failed: %s", err)
				}
				vars = utils.MergeGeneralMap(vars, extractorVars)
				for k, v := range extractorVars {
					extractorResults[k] = v
				}
			}
			if len(response) > 0 {
				availableResponse = append(availableResponse, string(response))
			}
		}
	} else {
		response := utils.StableReaderEx(conn, 5*time.Second, y.ReadSize)
		for _, extractor := range y.Extractor {
			extractorVars, err := extractor.Execute(response)
			if err != nil {
				log.Warnf("YakNetworkBulkConfig extractor.Execute failed: %s", err)
			}
			vars = utils.MergeGeneralMap(vars, extractorVars)
			for k, v := range extractorVars {
				extractorResults[k] = v
			}
		}
		if len(response) > 0 {
			availableResponse = append(availableResponse, string(response))
		}
	}

	if len(availableResponse) == 1 {
		vars["raw"] = availableResponse[0]
	}

	var haveResponse = len(availableResponse) > 0
	for _, response := range availableResponse {
		if y.Matcher != nil {
			matched, err := y.Matcher.ExecuteRaw([]byte(response), vars)
			if err != nil {
				log.Errorf("YakNetworkBulkConfig matcher.ExecuteRaw failed: %s", err)
			}
			callback([]byte(response+"-"+conn.RemoteAddr().String()), matched, extractorResults)
		}
	}

	if !haveResponse {
		callback(nil, false, extractorResults)
	}
	return err
}

func (y *YakNetworkBulkConfig) Execute(
	vars map[string]interface{}, lowhttpConfig *lowhttp.LowhttpExecConfig,
	callback func(rsp []byte, matched bool, extractorResults map[string]any),
) error {
	if len(y.Hosts) == 0 {
		return utils.Error("YakNetworkBulkConfig hosts is empty")
	}

	for _, host := range y.Hosts {
		hosts, err := mutate.FuzzTagExec(host, mutate.Fuzz_WithParams(vars))
		if err != nil {
			log.Errorf("YakNetworkBulkConfig mutate.FuzzTagExec(host) failed: %s", err)
			continue
		}
		if len(hosts) <= 0 {
			continue
		}
		host := hosts[0]
		host = utils.ExtractHostPort(host)

		defaultHost, defaultPort, _ := utils.ParseStringToHostPort(host)
		actualHost, actualPort := lowhttpConfig.Host, lowhttpConfig.Port
		if actualHost == "" {
			actualHost = defaultHost
		}
		if actualPort == 0 {
			actualPort = defaultPort
		}
		if actualHost == "" {
			log.Error("YakNetworkBulkConfig actualHost is empty")
			continue
		}

		if actualPort <= 0 {
			log.Errorf("YakNetworkBulkConfig actualPort is invalid: %d, use default 80", actualPort)
			actualPort = 80
		}
		target := utils.HostPort(actualHost, actualPort)
		conn, err := utils.GetAutoProxyConnEx(target, lowhttpConfig.Proxy, lowhttpConfig.Timeout)
		if err != nil {
			log.Errorf("get conn[%v] failed: %s", target, err)
			continue
		}
		err = y.handleConn(conn, lowhttpConfig, vars, callback)
		if conn != nil {
			conn.Close()
		}
		if err != nil {
			log.Errorf(`YakNetworkBulkConfig.handleConn failed: %s`, err)
		}
	}
	return nil
}
