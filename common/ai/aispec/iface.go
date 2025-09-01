package aispec

import (
	"io"

	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type GeneralChatter func(string, ...AIConfigOption) (string, error)

type Chatter interface {
	Chat(string, ...any) (string, error)
	ChatStream(string) (io.Reader, error)
}

type FunctionCaller interface {
	ExtractData(data string, desc string, fields map[string]any) (map[string]any, error)
}

type Configurable interface {
	LoadOption(opt ...AIConfigOption)
	BuildHTTPOptions() ([]poc.PocConfigOption, error)
	CheckValid() error
}

type StructuredData struct {
	Id             string
	Event          string
	DataSourceType string
	DataRaw        []byte

	IsParsed   bool
	IsResponse bool
	// parsed from node
	OutputNodeName     string
	OutputNodeStatus   string
	OutputNodeId       string
	OutputNodeType     string
	OutputNodeExecTime string
	OutputText         string
	OutputReason       string

	HaveUsage  bool
	ModelUsage []UsageStatsInfo
}

func (s *StructuredData) Copy() *StructuredData {
	return &StructuredData{
		Id:             s.Id,
		Event:          s.Event,
		DataSourceType: s.DataSourceType,
		DataRaw:        s.DataRaw,
		HaveUsage:      s.HaveUsage,
		ModelUsage:     s.ModelUsage,
	}
}

type UsageStatsInfo struct {
	Model       string
	InputToken  int
	OutputToken int
}

type StructuredStreamer interface {
	SupportedStructuredStream() bool
	StructuredStream(string, ...any) (chan *StructuredData, error)
}

type ModelMeta struct {
	Id      string `json:"id"`
	Object  string `json:"object,omitempty"`
	Created int64  `json:"created,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type ModelListCaller interface {
	GetModelList() ([]*ModelMeta, error)
}

type AIClient interface {
	Chatter
	FunctionCaller
	Configurable
	StructuredStreamer
	ModelListCaller
}

type EmbeddingCaller interface {
	Embedding(string) ([]float32, error)
}
