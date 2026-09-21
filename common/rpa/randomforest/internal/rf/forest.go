// Derived from github.com/fxsjy/RF.go, commit 46700521f302.
// Only the classification core used by RPA is retained.
package rf

import (
	"fmt"
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
	done_flag := make(chan bool)
	prog_counter := 0
	mutex := &sync.Mutex{}
	for i := 0; i < treesAmount; i++ {
		go func(x int) {
			fmt.Printf(">> %v buiding %vth tree...\n", time.Now(), x)
			forest.Trees[x] = BuildTree(inputs, labels, samplesAmount, selectedFeatureAmount)
			//fmt.Printf("<< %v the %vth tree is done.\n",time.Now(), x)
			mutex.Lock()
			prog_counter += 1
			fmt.Printf("%v tranning progress %.0f%%\n", time.Now(), float64(prog_counter)/float64(treesAmount)*100)
			mutex.Unlock()
			done_flag <- true
		}(i)
	}

	for i := 1; i <= treesAmount; i++ {
		<-done_flag
	}

	fmt.Println("all done.")
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
