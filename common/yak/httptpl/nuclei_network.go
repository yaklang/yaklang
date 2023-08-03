package httptpl

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func parseNetworkInputs(data map[string]any) []*YakTcpInput {
	var inputs []*YakTcpInput
	for _, inputItemRaw := range utils.InterfaceToSliceInterface(utils.MapGetFirstRaw(data, "inputs", "input")) {
		var inputItem = utils.InterfaceToMapInterface(inputItemRaw)
		var (
			// data / read(int) / type: hex
			dataRaw = utils.InterfaceToString(utils.MapGetFirstRaw(inputItem, "data"))
			readInt = utils.MapGetInt(inputItem, "read")
			typeRaw = utils.MapGetString(inputItem, "type")
		)
		if readInt <= 0 {
			readInt = 2048
		}
		inputs = append(inputs, &YakTcpInput{
			Data: dataRaw,
			Read: readInt,
			Type: typeRaw,
		})
	}
	return inputs
}

func buildNetworkRequests(inputs []*YakTcpInput, hosts []string) *YakNetworkBulkConfig {
	return &YakNetworkBulkConfig{
		Inputs:   inputs,
		Hosts:    hosts,
		ReadSize: 2048,
	}
}

func parseNetworkBulk(ret []any, tagsToPlaceHolderMap map[string]string) ([]*YakNetworkBulkConfig, error) {
	var confs []*YakNetworkBulkConfig
	for _, i := range utils.InterfaceToSliceInterface(ret) {
		data := utils.InterfaceToGeneralMap(i)
		inputs := parseNetworkInputs(data)
		var hosts []string
		for _, h := range utils.InterfaceToStringSlice(utils.MapGetFirstRaw(data, "host", "hosts")) {
			hosts = append(hosts, nucleiFormatToFuzzTagMode(h))
		}
		network := buildNetworkRequests(inputs, hosts)
		_ = network
		readSize := utils.MapGetIntEx(data, "read-size", "read_size", "readSize", "readsize")
		network.ReadSize = readSize
		matcher, err := generateYakMatcher(data)
		if err != nil {
			log.Warnf("build matcher failed: %s", err)
			continue
		}
		network.Matcher = matcher
		extractors, err := generateYakExtractors(data)
		if err != nil {
			log.Warnf("build extractor failed: %s", err)
		}
		network.Extractor = extractors
		if len(network.Extractor) <= 0 && network.Matcher == nil {
			log.Warn("no matcher and extractor found")
			continue
		}
		confs = append(confs, network)
	}
	if len(confs) <= 0 {
		return nil, utils.Error("empty network bulk config")
	}
	return confs, nil
}
