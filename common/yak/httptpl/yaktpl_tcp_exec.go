package httptpl

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/netx"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type NucleiTcpResponse struct {
	RawPacket  []byte
	RawRequest []byte
	RemoteAddr string
	RuntimeId  string
}

func (y *YakNetworkBulkConfig) handleConn(
	config *Config,
	conn net.Conn, lowhttpConfig *lowhttp.LowhttpExecConfig,
	vars map[string]any, template *YakTemplate,
	callback func(rsp []*NucleiTcpResponse, matched bool, extractorResults map[string]any),
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
		availableResponse []*NucleiTcpResponse
	)

	var err error
	if len(y.Inputs) > 0 {
	REQ:
		for _, inputElement := range y.Inputs {
			tcpResp := &NucleiTcpResponse{
				RemoteAddr: conn.RemoteAddr().String(),
				RuntimeId:  config.RuntimeId,
			}
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
				tcpResp.RawRequest = raw
				if config.Debug || config.DebugRequest {
					fmt.Println("---------------------TCP REQUEST---------------------")
					spew.Dump(string(raw))
					fmt.Println("------------------------------------------------------")
					fmt.Println(strconv.Quote(string(raw)))
				}
				conn.Write(raw)
			}
			bufferSize := inputElement.Read
			if bufferSize <= 0 {
				bufferSize = y.ReadSize
			}
			response := utils.StableReaderEx(conn, 5*time.Second, bufferSize)
			if y.ReverseConnectionNeed {
				if token, ok := vars["reverse_dnslog_token"].(string); ok {
					if config.OOBRequireCheckingTrigger == nil {
						template.InjectInteractshVar(token, config.RuntimeId, vars)
					}
				}
			}
			for _, extractor := range y.Extractor {
				extractorVars, err := extractor.Execute(response, vars)
				if err != nil {
					log.Warnf("YakNetworkBulkConfig extractor.Execute failed: %s", err)
				}
				vars = utils.MergeGeneralMap(vars, extractorVars)
				for k, v := range extractorVars {
					if v != nil {
						extractorResults[k] = v
					}
				}
			}
			if len(response) > 0 {
				tcpResp.RawPacket = response
				availableResponse = append(availableResponse, tcpResp)
			}
		}
	} else {
		tcpResp := &NucleiTcpResponse{
			RemoteAddr: conn.RemoteAddr().String(),
			RawRequest: nil,
			RuntimeId:  config.RuntimeId,
		}
		response := utils.StableReaderEx(conn, 5*time.Second, y.ReadSize)
		for _, extractor := range y.Extractor {
			extractorVars, err := extractor.Execute(response, vars)
			if err != nil {
				log.Warnf("YakNetworkBulkConfig extractor.Execute failed: %s", err)
			}
			vars = utils.MergeGeneralMap(vars, extractorVars)
			for k, v := range extractorVars {
				extractorResults[k] = v
			}
		}
		if len(response) > 0 {
			tcpResp.RawPacket = response
			availableResponse = append(availableResponse, tcpResp)
		}
	}

	if len(availableResponse) == 1 {
		vars["raw"] = availableResponse[0]
	}

	haveResponse := len(availableResponse) > 0
	for _, response := range availableResponse {
		if y.Matcher != nil {
			matched, err := y.Matcher.ExecuteRawWithConfig(config, response.RawPacket, vars)
			if err != nil {
				log.Errorf("YakNetworkBulkConfig matcher.ExecuteRaw failed: %s", err)
			}
			callback(availableResponse, matched, extractorResults)
		}
	}

	if !haveResponse {
		callback(nil, false, extractorResults)
	}
	return err
}

func (y *YakNetworkBulkConfig) Execute(
	config *Config,
	vars map[string]interface{}, params map[string]string, lowhttpConfig *lowhttp.LowhttpExecConfig, template *YakTemplate,
	callback func(rsp []*NucleiTcpResponse, matched bool, extractorResults map[string]any),
) error {
	if len(y.Hosts) == 0 {
		return utils.Error("YakNetworkBulkConfig hosts is empty")
	}

	var err error
	for _, host := range y.Hosts {
		host, err = QuickFuzzNucleiTag(host, utils.InterfaceToMapInterface(params))
		if err != nil {
			log.Error("YakNetworkBulkConfig render host error " + err.Error())
			continue
		}
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
		if config.Debug || config.DebugRequest {
			log.Infof("YakNetworkBulkConfig to target: %v", target)
		}
		conn, err := netx.DialTCPTimeout(lowhttpConfig.Timeout, target, lowhttpConfig.Proxy...)
		if err != nil {
			log.Errorf("get conn[%v] failed: %s", target, err)
			continue
		}
		err = y.handleConn(config, conn, lowhttpConfig, vars, template, callback)
		if conn != nil {
			conn.Close()
		}
		if err != nil {
			log.Errorf(`YakNetworkBulkConfig.handleConn failed: %s`, err)
		}
	}
	return nil
}
