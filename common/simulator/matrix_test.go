// Package simulator
// @Author bcy2007  2023/8/17 16:21
package simulator

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestDataMatrix(t *testing.T) {
	type args struct {
		tags  []string
		items []string
		data  [][]float64
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "zero similarity test",
			args: args{
				tags:  []string{"username", "password", "test", "testZero"},
				items: []string{"selector1", "selector2", "selector3", "selector4", "selector5"},
				data: [][]float64{
					{1, 1.2, 1.4, 0},
					{0.5, 0.7, 0.9, 0},
					{0.2, 1.3, 2.4, 0},
					{0.83, 0.82, 0.84, 0},
					{0.1, 0.09, 0.08, 0},
				},
			},
			want: map[string]string{
				"username": "selector4",
				"test":     "selector3",
				"password": "selector1",
				"testZero": "",
			},
		},
		{
			name: "short items",
			args: args{
				tags:  []string{"username", "password", "test", "testZero"},
				items: []string{"selector1", "selector2"},
				data: [][]float64{
					{1, 1.2, 1.4, 0},
					{0.5, 0.7, 0.9, 0},
				},
			},
			want: map[string]string{
				"test":     "selector1",
				"password": "selector2",
			},
		},
		{
			name: "short tags",
			args: args{
				tags:  []string{"username", "password", "test", "testZero"},
				items: []string{"selector1", "selector2", "selector3", "selector4", "selector5"},
				data: [][]float64{
					{1, 1.2, 1.4, 0},
					{0.5, 0.7, 0.9, 0},
					{0, 0, 0, 0},
					{0.83, 0.82, 0.84, 0},
					{0.1, 0.09, 0.08, 0},
				},
			},
			want: map[string]string{
				"test":     "selector1",
				"username": "selector4",
				"password": "selector2",
				"testZero": "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matrix := DataMatrix[string]{
				ItemList: tt.args.items,
				TagList:  tt.args.tags,
				Data:     tt.args.data,
			}
			err := matrix.ValidCheck()
			if err != nil {
				t.Error(err)
			}
			result, err := matrix.GetResult()
			if err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(result, tt.want) {
				t.Errorf(`matrix result = %v, want = %v`, result, tt.want)
			}
		})
	}
}

func TestMatrix(t *testing.T) {
	test := assert.New(t)
	tags := []string{"username", "password", "test", "testZero"}
	selectors := []string{"selector1", "selector2", "selector3", "selector4", "selector5"}
	data := [][]float64{
		{1, 1.2, 1.4, 0},
		{0.5, 0.7, 0.9, 0},
		{0.2, 1.3, 2.4, 0},
		{0.83, 0.82, 0.84, 0},
		{0.1, 0.09, 0.08, 0},
	}
	matrix := DataMatrix[string]{
		ItemList: selectors,
		TagList:  tags,
		Data:     data,
	}
	err := matrix.ValidCheck()
	if err != nil {
		t.Error(err)
	}
	result, err := matrix.GetResult()
	if err != nil {
		t.Error(err)
		return
	}
	expect := map[string]string{
		"username": "selector4",
		"test":     "selector3",
		"password": "selector1",
		"testZero": "",
	}
	test.Equal(expect, result)
}
