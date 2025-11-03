package rag

import "github.com/yaklang/yaklang/common/ai/rag/vectorstore"

// type DocumentOption = vectorstore.DocumentOption

// Vector store related functions and types
var ImportRAGFromFile = vectorstore.ImportRAGFromFile
var ExportRAGToFile = vectorstore.ExportRAGToFile
var DeleteCollection = vectorstore.DeleteCollection
var GetCollection = vectorstore.GetCollection
var BuildVectorIndexForKnowledgeBase = vectorstore.BuildVectorIndexForKnowledgeBase
var BuildVectorIndexForKnowledgeBaseEntry = vectorstore.BuildVectorIndexForKnowledgeBaseEntry

type CollectionConfigFunc = vectorstore.CollectionConfigFunc
type DocumentOption = vectorstore.DocumentOption
