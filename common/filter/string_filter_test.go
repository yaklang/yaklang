package filter

import (
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
	"testing"
)

func TestRemoveDuplicatePorts(t *testing.T) {
	type args struct {
		ports1 string
		ports2 string
	}
	tests := []struct {
		name string
		args args
		want []int
	}{
		{
			name: "test1",
			args: args{
				ports1: "80,81,82,83,84,85,86,87,88,89",
				ports2: "80,81,82,83,84,85,86,87,88,89,90",
			},
			want: utils.ParseStringToPorts("80-90"),
		},
		{
			name: "test2",
			args: args{
				ports1: "1000-1005,2000-2005",
				ports2: "1003-1007,2003-2007,3000-3005",
			},
			want: utils.ParseStringToPorts("1000-1007,2000-2007,3000-3005"),
		},
		{
			name: "test3",
			args: args{
				ports1: "5000-5005",
				ports2: "6000-6005",
			},
			want: utils.ParseStringToPorts("5000-5005,6000-6005"),
		},
		{
			name: "test4",
			args: args{
				ports1: "7000-7005,8000-8005",
				ports2: "8000-8005,9000-9005",
			},
			want: utils.ParseStringToPorts("7000-7005,8000-8005,9000-9005"),
		},
		{
			name: "test5",
			args: args{
				ports1: "80,81,82,83,84,85,86,87,88,89",
				ports2: "",
			},
			want: utils.ParseStringToPorts("80-89"),
		},
		{
			name: "test6",
			args: args{
				ports1: "",
				ports2: "80,81,82,83,84,85,86,87,88,89,90",
			},
			want: utils.ParseStringToPorts("80-90"),
		},
		{
			name: "test7",
			args: args{
				ports1: "",
				ports2: "",
			},
			want: utils.ParseStringToPorts(""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveDuplicatePorts(tt.args.ports1, tt.args.ports2); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RemoveDuplicatePorts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterPorts(t *testing.T) {
	type args struct {
		sourcePorts  string
		excludePorts string
	}
	tests := []struct {
		name string
		args args
		want []int
	}{
		{
			name: "Test 1: No common ports",
			args: args{
				sourcePorts:  "1-5",
				excludePorts: "6-10",
			},
			want: []int{1, 2, 3, 4, 5},
		},
		{
			name: "Test 2: All ports are common",
			args: args{
				sourcePorts:  "1-5",
				excludePorts: "1-5",
			},
			want: []int{},
		},
		{
			name: "Test 3: Some common ports",
			args: args{
				sourcePorts:  "1-10",
				excludePorts: "5-15",
			},
			want: []int{1, 2, 3, 4},
		},
		{
			name: "Test 4: Exclude ports are subset of source ports",
			args: args{
				sourcePorts:  "1-10",
				excludePorts: "3-7",
			},
			want: []int{1, 2, 8, 9, 10},
		},
		{
			name: "Test 5: Source ports are subset of exclude ports",
			args: args{
				sourcePorts:  "3-7",
				excludePorts: "1-10",
			},
			want: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FilterPorts(tt.args.sourcePorts, tt.args.excludePorts); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterPorts() = %v, want %v", got, tt.want)
			}
		})
	}
}
