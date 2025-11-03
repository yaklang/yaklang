package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func getListCollectionCommand() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "列出所有知识库列表",
		Action: listCollection,
	}
}

func listCollection(c *cli.Context) error {
	db := consts.GetGormProfileDatabase()
	collections, err := yakit.GetAllRAGCollectionInfos(db)
	if err != nil {
		return utils.Errorf("获取知识库列表失败: %v", err)
	}
	if len(collections) == 0 {
		fmt.Println("暂无知识库")
		return nil
	}

	fmt.Println("知识库列表:")
	fmt.Println(strings.Repeat("=", 80))

	for i, collection := range collections {
		info, err := vectorstore.GetCollectionInfo(db, collection.Name)
		if err != nil {
			fmt.Printf("获取知识库 %s 信息失败: %v\n", collection.Name, err)
			continue
		}

		if i > 0 {
			fmt.Println(strings.Repeat("-", 80))
		}

		fmt.Printf("知识库名称: %s\n", info.Name)
		fmt.Printf("描    述: %s\n", info.Description)
		fmt.Printf("模型名称: %s\n", info.ModelName)
		fmt.Printf("向量维度: %d\n", info.Dimension)
		fmt.Printf("距离函数: %s\n", info.DistanceFuncType)
		fmt.Printf("\nHNSW参数:\n")
		fmt.Printf("  M (最大连接数): %d\n", info.M)
		fmt.Printf("  Ml (层生成因子): %.2f\n", info.Ml)
		fmt.Printf("  EfSearch: %d\n", info.EfSearch)
		fmt.Printf("  EfConstruct: %d\n", info.EfConstruct)
		fmt.Printf("\n图结构统计:\n")
		// fmt.Printf("  层数: %d\n", info.LayerCount)
		// fmt.Printf("  总节点数: %d\n", info.NodeCount)
		// fmt.Printf("  最大邻居数: %d\n", info.MaxNeighbors)
		// fmt.Printf("  最小邻居数: %d\n", info.MinNeighbors)
		// fmt.Printf("  总连接数: %d\n", info.ConnectionCount)

		// if len(info.LayerNodeCountMap) > 0 {
		// 	fmt.Printf("\n各层节点分布:\n")
		// 	for layer := 0; layer < info.LayerCount; layer++ {
		// 		if count, exists := info.LayerNodeCountMap[layer]; exists {
		// 			fmt.Printf("  第%d层: %d个节点\n", layer, count)
		// 		}
		// 	}
		// }
	}

	fmt.Println(strings.Repeat("=", 80))
	return nil
}
