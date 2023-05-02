package antlr4nasl

import (
	"encoding/json"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
)

type NaslScriptInfo struct {
	Kbs        map[string]interface{}
	naslScript *yakit.NaslScript
	Hash       string
	OID        string

	CVE             []string
	ScriptName      string
	Script          string
	Tags            map[string]interface{}
	Version         string
	Category        string
	Family          string
	Copyright       string
	Dependencies    []string
	RequirePorts    []string
	RequireKeys     []string
	ExcludeKeys     []string
	Xrefs           map[string]string
	Preferences     map[string]interface{}
	RequireUdpPorts []string
	BugtraqId       []int
	MandatoryKeys   []string
	Timeout         int
}

func NewNaslScriptObject() *NaslScriptInfo {
	return &NaslScriptInfo{
		naslScript:  yakit.NewEmptyNaslScript(),
		Tags:        make(map[string]interface{}),
		Xrefs:       make(map[string]string),
		Preferences: make(map[string]interface{}),
		Kbs:         make(map[string]interface{}),
	}
}
func GetNaslScriptObjectBy() *NaslScriptInfo {
	return nil
}
func (n *NaslScriptInfo) Save() error {
	tagMarshal, err := json.Marshal(n.Tags)
	if err != nil {
		return err
	}
	dependenciesMarshal, err := json.Marshal(n.Dependencies)
	if err != nil {
		return err
	}
	portsMarshal, err := json.Marshal(n.RequirePorts)
	if err != nil {
		return err
	}
	excludeKeysMarshal, err := json.Marshal(n.ExcludeKeys)
	if err != nil {
		return err
	}
	xrefsMarshal, err := json.Marshal(n.Xrefs)
	if err != nil {
		return err
	}
	preferencesMarshal, err := json.Marshal(n.Preferences)
	if err != nil {
		return err
	}
	RequireUdpPorts, err := json.Marshal(n.RequireUdpPorts)
	if err != nil {
		return err
	}
	MandatoryKeys, err := json.Marshal(n.MandatoryKeys)
	if err != nil {
		return err
	}
	BugtraqId, err := json.Marshal(n.BugtraqId)
	if err != nil {
		return err
	}
	cves, err := json.Marshal(n.CVE)
	if err != nil {
		return err
	}
	RequireKeys, err := json.Marshal(n.RequireKeys)
	if err != nil {
		return err
	}
	n.naslScript.Script = n.Script
	n.naslScript.ScriptName = n.ScriptName
	n.naslScript.Tags = string(tagMarshal)
	n.naslScript.Version = n.Version
	n.naslScript.Category = n.Category
	n.naslScript.Family = n.Family
	n.naslScript.Copyright = n.Copyright
	n.naslScript.Dependencies = string(dependenciesMarshal)
	n.naslScript.RequirePorts = string(portsMarshal)
	n.naslScript.ExcludeKeys = string(excludeKeysMarshal)
	n.naslScript.Hash = n.Hash
	n.naslScript.OID = n.OID
	n.naslScript.CVE = string(cves)
	n.naslScript.Xref = string(xrefsMarshal)
	n.naslScript.Preferences = string(preferencesMarshal)
	n.naslScript.RequireUdpPorts = string(RequireUdpPorts)
	n.naslScript.BugtraqId = string(BugtraqId)
	n.naslScript.MandatoryKeys = string(MandatoryKeys)
	n.naslScript.Timeout = n.Timeout
	n.naslScript.RequireKeys = string(RequireKeys)
	return n.naslScript.CreateOrUpdateNaslScript(consts.GetGormProfileDatabase())
}
