package cartesian

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"math"
	"reflect"
	"testing"
)

func TestProductString(t *testing.T) {
	results, err := Product([][]string{
		{"a", "b", "c"},
		{"a", "b", "c"},
		{"a", "b", "c"},
		{"a", "b", "c"},
		{"a", "b", "c"},
		{"a", "b", "c"},
		{"a", "b", "c"},
	})
	if err != nil {
		panic(err)
	}
	if int64(math.Pow(3, 7)) != int64(len(results)) {
		panic("math.Pow(3, 7) != len(results)")
	}
	spew.Dump(results)
}

func TestCartesianProduct(t *testing.T) {
	tests := []struct {
		name string
		sets [][]interface{}
		want [][]interface{}
	}{
		{
			name: "Test 1",
			sets: [][]interface{}{
				{1, 2},
				{"a", "b"},
			},
			want: [][]interface{}{
				{1, "a"},
				{1, "b"},
				{2, "a"},
				{2, "b"},
			},
		},
		{
			name: "Test 2",
			sets: [][]interface{}{
				{1},
				{"a"},
			},
			want: [][]interface{}{
				{1, "a"},
			},
		},
		{
			name: "Test 3",
			sets: [][]interface{}{},
			want: [][]interface{}{},
		},
		{
			name: "Test 4",
			sets: [][]interface{}{
				{1, 2},
				{"a", "b"},
				{3.1, 3.2},
			},
			want: [][]interface{}{
				{1, "a", 3.1},
				{1, "a", 3.2},
				{1, "b", 3.1},
				{1, "b", 3.2},
				{2, "a", 3.1},
				{2, "a", 3.2},
				{2, "b", 3.1},
				{2, "b", 3.2},
			},
		},
		{
			name: "Test 5",
			sets: [][]interface{}{
				{1, 2},
				{"a"},
				{3.1},
				{"x", "y"},
			},
			want: [][]interface{}{
				{1, "a", 3.1, "x"},
				{1, "a", 3.1, "y"},
				{2, "a", 3.1, "x"},
				{2, "a", 3.1, "y"},
			},
		},
		{
			name: "Test 6",
			sets: [][]interface{}{
				{1, 2},
				{"a"},
				{3.1},
				{"x", "y"},
			},
			want: [][]interface{}{
				{1, "a", 3.1, "x"},
				{1, "a", 3.1, "y"},
				{2, "a", 3.1, "x"},
				{2, "a", 3.1, "y"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ch := make(chan []interface{})

			go cartesianProductRaw(ctx, tt.sets, ch)

			var got [][]interface{}
			for item := range ch {
				got = append(got, item)
			}

			if !reflect.DeepEqual(got, tt.want) {
				if len(got) == 0 && len(tt.want) == 0 {
					return
				}
				t.Errorf("CartesianProduct() = %v, want %v", got, tt.want)
			}
		})
	}
}
