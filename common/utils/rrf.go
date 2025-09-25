package utils

import (
	"github.com/samber/lo"
	"sort"
)

type RRFScoredData interface {
	GetScoreMethod() string
	GetScore() float64
	GetUUID() string
}

type RankScoredData struct {
	ID           string
	RRFRankScore float64
}

func RRFRankWithDefaultK[T RRFScoredData](scoredDataList []T) []T {
	return RRFRank(scoredDataList, 60)
}

func RRFRank[T RRFScoredData](scoredDataList []T, k int) []T {
	scoredDataMethodMap := make(map[string]map[string]T)
	for _, scoredData := range scoredDataList {
		dataUUID := scoredData.GetUUID()
		scoreMethod := scoredData.GetScoreMethod()
		if subMap, ok := scoredDataMethodMap[scoreMethod]; ok {
			if oldData, exist := subMap[dataUUID]; !exist || oldData.GetScore() < scoredData.GetScore() {
				subMap[dataUUID] = scoredData
			}
			scoredDataMethodMap[scoreMethod] = subMap
		} else {
			scoredDataMethodMap[scoreMethod] = map[string]T{
				scoredData.GetUUID(): scoredData,
			}
		}
	}

	rrfScores := make(map[string]float64)
	scoredDataMap := make(map[string]T)
	for _, subMap := range scoredDataMethodMap {
		var subSlice []T
		for _, datum := range subMap {
			subSlice = append(subSlice, datum)
		}
		sort.Slice(subSlice, func(i, j int) bool {
			return subSlice[i].GetScore() > subSlice[j].GetScore()
		})
		for rank, scoredData := range subSlice {
			actualRank := rank + 1
			scoredDataMap[scoredData.GetUUID()] = scoredData
			rrfScores[scoredData.GetUUID()] += 1.0 / (float64(k) + float64(actualRank))
		}
	}

	var rrfData []RankScoredData
	for dataID, score := range rrfScores {
		rrfData = append(rrfData, RankScoredData{ID: dataID, RRFRankScore: score})
	}
	sort.Slice(rrfData, func(i, j int) bool {
		return rrfData[i].RRFRankScore > rrfData[j].RRFRankScore
	})

	return lo.Map(rrfData, func(item RankScoredData, _ int) T {
		return scoredDataMap[item.ID]
	})
}
