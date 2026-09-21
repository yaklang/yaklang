// Derived from github.com/fxsjy/RF.go, commit 46700521f302.
package rf

import (
	"math"
	"math/rand"
)

const CAT = "cat"
const NUMERIC = "numeric"

type TreeNode struct {
	ColumnNo int //column number
	Value    interface{}
	Left     *TreeNode
	Right    *TreeNode
	Labels   map[string]int
}

type Tree struct {
	Root *TreeNode
}

func getRandomRange(N int, M int) []int {
	tmp := make([]int, N)
	for i := 0; i < N; i++ {
		tmp[i] = i
	}
	for i := 0; i < M; i++ {
		j := i + int(rand.Float64()*float64(N-i))
		tmp[i], tmp[j] = tmp[j], tmp[i]
	}

	return tmp[:M]
}

func getSamples(ary [][]interface{}, index []int) [][]interface{} {
	result := make([][]interface{}, len(index))
	for i := 0; i < len(index); i++ {
		result[i] = ary[index[i]]
	}
	return result
}

func getLabels(ary []string, index []int) []string {
	result := make([]string, len(index))
	for i := 0; i < len(index); i++ {
		result[i] = ary[index[i]]
	}
	return result
}

func getEntropy(ep_map map[string]float64, total int) float64 {

	for k, _ := range ep_map {
		ep_map[k] = ep_map[k] / float64(total) //normalize
	}

	entropy := 0.0
	for _, v := range ep_map {
		entropy += v * math.Log(1.0/v)
	}

	return entropy
}

func getBestGain(samples [][]interface{}, c int, samples_labels []string, column_type string, current_entropy float64) (float64, interface{}, int, int) {
	var best_value interface{}
	best_gain := 0.0
	best_total_r := 0
	best_total_l := 0

	uniq_values := make(map[interface{}]int)
	for i := 0; i < len(samples); i++ {
		uniq_values[samples[i][c]] = 1
	}

	for value, _ := range uniq_values {
		map_l := make(map[string]float64)
		map_r := make(map[string]float64)
		total_l := 0
		total_r := 0
		if column_type == CAT {
			for j := 0; j < len(samples); j++ {
				if samples[j][c] == value {
					total_l += 1
					map_l[samples_labels[j]] += 1.0
				} else {
					total_r += 1
					map_r[samples_labels[j]] += 1.0
				}
			}
		}
		if column_type == NUMERIC {
			for j := 0; j < len(samples); j++ {
				if samples[j][c].(float64) <= value.(float64) {
					total_l += 1
					map_l[samples_labels[j]] += 1.0
				} else {
					total_r += 1
					map_r[samples_labels[j]] += 1.0
				}
			}
		}

		p1 := float64(total_r) / float64(len(samples))
		p2 := float64(total_l) / float64(len(samples))

		new_entropy := p1*getEntropy(map_r, total_r) + p2*getEntropy(map_l, total_l)
		entropy_gain := current_entropy - new_entropy

		if entropy_gain >= best_gain {
			best_gain = entropy_gain
			best_value = value
			best_total_l = total_l
			best_total_r = total_r
		}
	}

	return best_gain, best_value, best_total_l, best_total_r
}

func splitSamples(samples [][]interface{}, column_type string, c int, value interface{}, part_l *[]int, part_r *[]int) {
	if column_type == CAT {
		for j := 0; j < len(samples); j++ {
			if samples[j][c] == value {
				*part_l = append(*part_l, j)
			} else {
				*part_r = append(*part_r, j)
			}
		}
	}
	if column_type == NUMERIC {
		for j := 0; j < len(samples); j++ {
			if samples[j][c].(float64) <= value.(float64) {
				*part_l = append(*part_l, j)
			} else {
				*part_r = append(*part_r, j)
			}
		}
	}
}

func buildTree(samples [][]interface{}, samples_labels []string, selected_feature_count int) *TreeNode {
	column_count := len(samples[0])
	split_count := selected_feature_count
	columns_choosen := getRandomRange(column_count, split_count)

	best_gain := 0.0
	var best_part_l []int = make([]int, 0, len(samples))
	var best_part_r []int = make([]int, 0, len(samples))
	var best_total_l int = 0
	var best_total_r int = 0
	var best_value interface{}
	var best_column int
	var best_column_type string

	current_entropy_map := make(map[string]float64)
	for i := 0; i < len(samples_labels); i++ {
		current_entropy_map[samples_labels[i]] += 1
	}

	current_entropy := getEntropy(current_entropy_map, len(samples_labels))

	for _, c := range columns_choosen {
		column_type := CAT
		if _, ok := samples[0][c].(float64); ok {
			column_type = NUMERIC
		}

		gain, value, total_l, total_r := getBestGain(samples, c, samples_labels, column_type, current_entropy)
		if gain >= best_gain {
			best_gain = gain
			best_value = value
			best_column = c
			best_column_type = column_type
			best_total_l = total_l
			best_total_r = total_r
		}
	}

	if best_gain > 0 && best_total_l > 0 && best_total_r > 0 {
		node := &TreeNode{}
		node.Value = best_value
		node.ColumnNo = best_column
		splitSamples(samples, best_column_type, best_column, best_value, &best_part_l, &best_part_r)
		node.Left = buildTree(getSamples(samples, best_part_l), getLabels(samples_labels, best_part_l), selected_feature_count)
		node.Right = buildTree(getSamples(samples, best_part_r), getLabels(samples_labels, best_part_r), selected_feature_count)
		return node
	}

	return genLeafNode(samples_labels)

}

func genLeafNode(labels []string) *TreeNode {
	counter := make(map[string]int)
	for _, v := range labels {
		counter[v] += 1
	}

	node := &TreeNode{}
	node.Labels = counter
	return node
}

func predicate(node *TreeNode, input []interface{}) map[string]int {
	if node.Labels != nil { //leaf node
		return node.Labels
	}

	c := node.ColumnNo
	value := input[c]

	switch value.(type) {
	case float64:
		if value.(float64) <= node.Value.(float64) && node.Left != nil {
			return predicate(node.Left, input)
		} else if node.Right != nil {
			return predicate(node.Right, input)
		}
	case string:
		if value == node.Value && node.Left != nil {
			return predicate(node.Left, input)
		} else if node.Right != nil {
			return predicate(node.Right, input)
		}
	}

	return nil
}

func BuildTree(inputs [][]interface{}, labels []string, samples_count, selected_feature_count int) *Tree {

	samples := make([][]interface{}, samples_count)
	samples_labels := make([]string, samples_count)
	for i := 0; i < samples_count; i++ {
		j := int(rand.Float64() * float64(len(inputs)))
		samples[i] = inputs[j]
		samples_labels[i] = labels[j]
	}

	tree := &Tree{}
	tree.Root = buildTree(samples, samples_labels, selected_feature_count)

	return tree
}

func PredicateTree(tree *Tree, input []interface{}) map[string]int {
	return predicate(tree.Root, input)
}
