package javaclassparser

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"github.com/yaklang/yaklang/common/log"
)

type Instruction struct {
	Name        string       `json:"name"`
	OpCode      int          `json:"opcode"`
	Length      int          `json:"data_length"`
	StackPopped []*StackType `json:"stack_types"`
	StackPushed []*StackType `json:"stack_pushed"`
	RawJavaType *RawJavaType `json:"raw_java_type"`
	HandleName  string       `json:"handler"`
	NoThrow     bool         `json:"no_throw"`
}
type RawJavaType struct {
	Name             string     `json:"name"`
	SuggestedVarName string     `json:"suggestedVarName"`
	StackType        *StackType `json:"stackType"`
	UsableType       bool       `json:"usableType"`
	BoxedName        string     `json:"boxedName"`
	IsNumber         bool       `json:"isNumber"`
	IsObject         bool       `json:"isObject"`
	IntMin           int        `json:"intMin"`
	IntMax           int        `json:"intMax"`
}
type StackType struct {
	ComputationCategory int    `json:"computationCategory"`
	Closed              bool   `json:"closed"`
	Name                string `json:"name"`
}

var InstrInfos = map[int]*Instruction{}

func init() {
	content, err := classes.FS.ReadFile("instr_infos.json")
	if err != nil {
		log.Errorf("initialize instruction info failed")
		return
	}
	instrInfos := map[string]*Instruction{}
	err = json.Unmarshal(content, &instrInfos)
	if err != nil {
		log.Errorf("invalid json, parse instruction failed: %v", err)
		return
	}
	for k, instruction := range instrInfos {
		instruction.Name = k
		InstrInfos[instruction.OpCode] = instruction
	}
}
