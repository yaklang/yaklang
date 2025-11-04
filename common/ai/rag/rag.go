package rag

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
)

// type DocumentOption = vectorstore.DocumentOption

// Vector store related functions and types
// var ImportRAGFromFile = vectorstore.ImportRAGFromFile
var DeleteCollection = vectorstore.DeleteCollection
var GetCollection = vectorstore.GetCollection

func BuildVectorIndexForKnowledgeBaseEntry(db *gorm.DB, knowledgeBaseId int64, id string, opts ...RAGSystemConfigOption) (*vectorstore.SQLiteVectorStoreHNSW, error) {
	colOpts := NewRAGSystemConfig(opts...).ConvertToVectorStoreOptions()
	return vectorstore.BuildVectorIndexForKnowledgeBaseEntry(db, knowledgeBaseId, id, colOpts...)
}

func BuildVectorIndexForKnowledgeBase(db *gorm.DB, id int64, opts ...RAGSystemConfigOption) (*vectorstore.SQLiteVectorStoreHNSW, error) {
	colOpts := NewRAGSystemConfig(opts...).ConvertToVectorStoreOptions()
	return vectorstore.BuildVectorIndexForKnowledgeBase(db, id, colOpts...)
}

// func ImportRAGFromReader(reader io.Reader, optFuncs ...RAGSystemConfigOption) error {
// 	config := NewRAGSystemConfig(optFuncs...)

// 	knowledgebase.ImportKnowledgeBase(context.Background(), config.db, reader, &knowledgebase.ImportKnowledgeBaseOptions{
// 		OverwriteExisting:    true,
// 		NewKnowledgeBaseName: config.Name,
// 	})
// 	importOpts := NewRAGSystemConfig(optFuncs...).ConvertToExportOptions()
// 	return vectorstore.ImportRAGFromReader(reader, importOpts...)
// }

// func LoadRAGFromReader(reader io.Reader) (*vectorstore.RAGBinaryData, error) {
// 	return vectorstore.LoadRAGFromBinary(reader)
// }

// func ImportRAGFromBinary(binary []byte, optFuncs ...RAGSystemConfigOption) error {
// 	return ImportRAGFromReader(bytes.NewReader(binary), optFuncs...)
// }

// func ImportRAGFromFile(inputPath string, optFuncs ...RAGSystemConfigOption) error {
// 	importOpts := NewRAGSystemConfig(optFuncs...).ConvertToExportOptions()
// 	return vectorstore.ImportRAGFromFile(inputPath, importOpts...)
// }

// func ExportRAGToBinary(collectionName string, opts ...RAGSystemConfigOption) (io.Reader, error) {
// 	exportOpts := NewRAGSystemConfig(opts...).ConvertToExportOptions()
// 	return vectorstore.ExportRAGToBinary(collectionName, exportOpts...)
// }

// func ExportRAGToFile(collectionName string, fileName string, opts ...RAGSystemConfigOption) error {
// 	exportOpts := NewRAGSystemConfig(opts...).ConvertToExportOptions()
// 	return vectorstore.ExportRAGToFile(collectionName, fileName, exportOpts...)
// }
