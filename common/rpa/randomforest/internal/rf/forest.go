// Derived from github.com/fxsjy/RF.go, commit 46700521f302.
package rf

import (
	"math/rand"
	"sync"
	"time"
)

type Forest struct {
	Trees []*Tree
}

func BuildForest(inputs [][]interface{}, labels []string, treesAmount, samplesAmount, selectedFeatureAmount int) *Forest {
	rand.Seed(time.Now().UnixNano())
	forest := &Forest{}
	forest.Trees = make([]*Tree, treesAmount)
	var workers sync.WaitGroup
	workers.Add(treesAmount)
	for i := range forest.Trees {
		go func(index int) {
			defer workers.Done()
			forest.Trees[index] = BuildTree(inputs, labels, samplesAmount, selectedFeatureAmount)
		}(i)
	}
	workers.Wait()

	return forest
}

func (self *Forest) Predicate(input []interface{}) string {
	counter := make(map[string]float64)
	for i := 0; i < len(self.Trees); i++ {
		tree_counter := PredicateTree(self.Trees[i], input)
		total := 0.0
		for _, v := range tree_counter {
			total += float64(v)
		}
		for k, v := range tree_counter {
			counter[k] += float64(v) / total
		}
	}

	max_c := 0.0
	max_label := ""
	for k, v := range counter {
		if v >= max_c {
			max_c = v
			max_label = k
		}
	}
	return max_label
}
