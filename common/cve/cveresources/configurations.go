package cveresources

import "encoding/json"

func ParseCPEConfigurations(raw []byte) (Configurations, error) {
	var config Configurations
	if err := json.Unmarshal(raw, &config); err == nil && len(config.Nodes) > 0 {
		return config, nil
	}

	var cve2Configs []CVE2Configuration
	if err := json.Unmarshal(raw, &cve2Configs); err != nil {
		return Configurations{}, err
	}

	config.Nodes = convertCVE2Configurations(cve2Configs)
	return config, nil
}

func convertCVE2Configurations(configs []CVE2Configuration) []Nodes {
	nodes := make([]Nodes, 0)
	for _, config := range configs {
		for _, node := range config.Nodes {
			nodes = append(nodes, convertCVE2Node(node))
		}
	}
	return nodes
}

func convertCVE2Node(node CVE2Node) Nodes {
	converted := Nodes{
		Operator: node.Operator,
		CpeMatch: make([]CpeMatch, 0, len(node.CpeMatch)),
		Children: make([]Nodes, 0, len(node.Children)),
	}
	for _, match := range node.CpeMatch {
		converted.CpeMatch = append(converted.CpeMatch, CpeMatch{
			Vulnerable:            match.Vulnerable,
			Cpe23URI:              match.Criteria,
			VersionStartExcluding: match.VersionStartExcluding,
			VersionEndExcluding:   match.VersionEndExcluding,
			VersionStartIncluding: match.VersionStartIncluding,
			VersionEndIncluding:   match.VersionEndIncluding,
		})
	}
	for _, child := range node.Children {
		converted.Children = append(converted.Children, convertCVE2Node(child))
	}
	return converted
}
