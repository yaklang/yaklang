// Package tools
// @Author bcy2007  2025/9/25 14:05
package tools

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCalculateResult(t *testing.T) {
	type args struct {
		formula string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			"乘法测试1", args{"1×2=?"}, "2", assert.NoError,
		},
		{
			"乘法测试2", args{"3x4"}, "12", assert.NoError,
		},
		{
			"乘法测试3", args{"5X6=?"}, "30", assert.NoError,
		},
		{
			"加法测试", args{"7+8=?"}, "15", assert.NoError,
		},
		{
			"减法测试1", args{"10-3=?"}, "7", assert.NoError,
		},
		{
			"减法测试2", args{"15_5"}, "10", assert.NoError,
		},
		{
			"无等号测试", args{"2×3"}, "6", assert.NoError,
		},
		{
			"带空格测试", args{" 4 + 6 =? "}, "10", assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetCalculateResult(tt.args.formula)
			if !tt.wantErr(t, err, fmt.Sprintf("GetCalculateResult(%v)", tt.args.formula)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetCalculateResult(%v)", tt.args.formula)
		})
	}
}
