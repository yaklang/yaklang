package httptpl

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

func parseNetworkInputs(root *yaml.Node) []*YakTcpInput {
	var inputs []*YakTcpInput

	// must be slice
	inputNode := nodeGetFirstRaw(root, "inputs", "input")
	sequenceNodeForEach(inputNode, func(subNode *yaml.Node) error {
		var (
			// data / read(int) / type: hex
			dataRaw = nodeGetString(subNode, "data")
			readInt = int(nodeGetInt64(subNode, "read"))
			typeRaw = nodeGetString(subNode, "type")
		)
		if readInt <= 0 {
			readInt = 2048
		}
		inputs = append(inputs, &YakTcpInput{
			Data: dataRaw,
			Read: readInt,
			Type: typeRaw,
		})
		return nil
	})

	return inputs
}

func parseNetworkBulk(ret []*yaml.Node, ReverseConnectionNeed bool) ([]*YakNetworkBulkConfig, error) {
	var confs []*YakNetworkBulkConfig
	for _, node := range ret {
		inputs := parseNetworkInputs(node)
		hosts := nodeToStringSlice(nodeGetFirstRaw(node, "host", "hosts"))
		network := &YakNetworkBulkConfig{
			Inputs:                inputs,
			Hosts:                 hosts,
			ReadSize:              2048,
			ReverseConnectionNeed: ReverseConnectionNeed,
		}
		readSizeNode := nodeGetFirstRaw(node, "read-size", "read_size", "readSize", "readsize")
		readSize := nodeToInt64(readSizeNode)
		network.ReadSize = int(readSize)
		matcher, err := generateYakMatcher(node)
		if err != nil {
			log.Warnf("build matcher failed: %s", err)
			continue
		}
		network.Matcher = matcher
		extractors, err := generateYakExtractors(node)
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
