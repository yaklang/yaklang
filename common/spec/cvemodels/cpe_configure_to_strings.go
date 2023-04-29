package cvemodels

import (
	"fmt"
	"strings"
)

func (c *Configurations) ToHumanReadableString() string {
	return c.ShowShorter()
}

func (c *Configurations) ShowShorter() string {
	var r []string
	for _, n := range c.Nodes {
		r = append(r, n.ToHumanReadableString())
	}
	return strings.Join(r, "\r\n")
}

type cpeCluster struct {
	c        *CpeStruct
	versions []string
}

func (c *cpeCluster) CompactVersions() []string {
	tree := NewVersionTree("", c.versions...)
	return tree.Strings()
}

func (n *Nodes) ToHumanReadableString() string {
	switch n.Operator {
	case "AND", "and", "And":
		var ss []string
		for _, c := range n.Children {
			s := c.ToHumanReadableString()
			ss = append(ss, fmt.Sprintf("(%v)", s))
		}
		return strings.Join(ss, " and ")
	case "OR", "Or", "or":

		var table = make(map[string]*cpeCluster)
		for _, m := range n.CpeMatch {
			if !m.Vulnerable {
				continue
			}

			ins, err := ParseCPEStringToStruct(m.Cpe23URI)
			if err != nil {
				continue
			}

			cluster, ok := table[ins.ProductCPE23()]
			if !ok {
				cluster = &cpeCluster{
					c: &CpeStruct{
						Part:    ins.Part,
						Vendor:  ins.Vendor,
						Product: ins.Product,
					},
				}
				table[ins.ProductCPE23()] = cluster
			}

			cluster.versions = append(cluster.versions, ins.Version)
		}

		var s []string
		for _, clusterCPE := range table {
			raw := fmt.Sprintf("[%v]%v: {{%v}}",
				clusterCPE.c.Vendor, strings.Title(strings.ReplaceAll(clusterCPE.c.Product, "_", " ")),
				strings.Join(clusterCPE.CompactVersions(), ", "))
			s = append(s, raw)
		}

		var ret []string
		if len(s) > 1 {
			ret = append(ret, fmt.Sprintf("(%v)", s))
		} else {
			ret = s
		}
		return strings.Join(ret, " or ")
	}
	return "unknown"
}
