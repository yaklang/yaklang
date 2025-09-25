package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockRRFData 实现 RRFScoredData 接口
type mockRRFData struct {
	uuid        string
	score       float64
	scoreMethod string
}

func (m mockRRFData) GetScoreMethod() string { return m.scoreMethod }
func (m mockRRFData) GetScore() float64      { return m.score }
func (m mockRRFData) GetUUID() string        { return m.uuid }

func TestRRFRank_Basic(t *testing.T) {
	data := []mockRRFData{
		{uuid: "a", score: 0.9, scoreMethod: "m1"},
		{uuid: "b", score: 0.8, scoreMethod: "m1"},
		{uuid: "c", score: 0.7, scoreMethod: "m1"},
		{uuid: "a", score: 0.95, scoreMethod: "m2"}, // a在m2中分更高 ,是更多“评委”选择的
		{uuid: "d", score: 0.6, scoreMethod: "m2"},
	}

	// k=60, 实际分数差异不大，但顺序可测
	result := RRFRank(data, 60)
	assert.Equal(t, 4, len(result))
	assert.Equal(t, "a", result[0].GetUUID())
}

func TestRRFRankWithDefaultK(t *testing.T) {
	data := []mockRRFData{
		{uuid: "x", score: 1.0, scoreMethod: "m1"},
		{uuid: "y", score: 0.5, scoreMethod: "m1"},
		{uuid: "z", score: 0.2, scoreMethod: "m2"},
	}
	result := RRFRankWithDefaultK(data)
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "x", result[0].GetUUID())
}

func TestRRFRank_Deduplication(t *testing.T) {
	data := []mockRRFData{
		{uuid: "a", score: 0.5, scoreMethod: "m1"},
		{uuid: "a", score: 0.8, scoreMethod: "m1"}, // 更高分应覆盖
	}
	result := RRFRank(data, 60)
	assert.Equal(t, 1, len(result))
	assert.Equal(t, 0.8, result[0].GetScore())
}

func TestRRFRank_FlattenDifference(t *testing.T) {
	uuidA := "A"
	uuidB := "B"
	uuidC := "C"
	uuidD := "D"
	data := []mockRRFData{
		// uuidA: methodA极高分，methodB极低分
		{uuid: uuidA, score: 0.99, scoreMethod: "methodA"},
		{uuid: uuidA, score: 0.01, scoreMethod: "methodB"},
		// uuidB: methodA极低分，methodB极高分
		{uuid: uuidB, score: 0.01, scoreMethod: "methodA"},
		{uuid: uuidB, score: 0.99, scoreMethod: "methodB"},
		// uuidC: 两个方法都是中等分
		{uuid: uuidC, score: 0.5, scoreMethod: "methodA"},
		{uuid: uuidC, score: 0.5, scoreMethod: "methodB"},
		// uuidD: 两个方法都是低等分
		{uuid: uuidD, score: 0.4, scoreMethod: "methodA"},
		{uuid: uuidD, score: 0.4, scoreMethod: "methodB"},
	}

	result := RRFRank(data, 60)
	assert.Equal(t, 4, len(result))

	// 理论上C > A ~ B > D ，但A和B的RRF分数应接近且都高于D
	pos := map[string]int{}
	for i, k := range result {
		pos[k.GetUUID()] = i
	}
	assert.Equal(t, 0, pos[uuidC], "uuidC should be ranked 1st")
	assert.True(t, pos[uuidA] == 1 || pos[uuidA] == 2, "uuidA should be ranked 2nd or 3rd")
	assert.True(t, pos[uuidB] == 1 || pos[uuidB] == 2, "uuidB should be ranked 2nd or 3rd")
	assert.Equal(t, 3, pos[uuidD], "uuidD should be ranked last")
}
