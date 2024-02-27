package crawler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMUSTPASS_JSHandle(t *testing.T) {

	check := func(t *testing.T, code string, want *requestNewTarget) {
		test := assert.New(t)

		handleJS(code, func(got *requestNewTarget) {
			test.Equal(want.Method, got.Method)
			test.Equal(want.Path, got.Path)
			test.Equal(len(want.Header), len(got.Header))
			test.Equal(want.Header, got.Header)
		})
	}

	type checkData struct {
		name string
		code string
		want *requestNewTarget
	}

	checkDatas := []checkData{
		{
			name: "featch post",
			code: `
console.log('1.js'); var deepUrl = 'deep.js';;
console.log('2.js'); fetch(deepUrl, {
	method: 'POST',
	headers: { 'HackedJS': "AAA"},
});
			`,
			want: &requestNewTarget{
				Method: "POST",
				Path:   "deep.js",
				Header: []*ypb.KVPair{
					{Key: "HackedJS", Value: "AAA"},
				},
			},
		},

		{
			name: "fetch get",
			code: `
fetch('/misc/response/fetch/basic.action')
.then(response => {
  if (!response.ok) {
	throw new Error('Network response was not ok ' + response.statusText);
  }
  return response.text();
})
.then(data => {
  console.log(data); // 这里是你的页面内容
})
.catch(error => {
  console.error('There has been a problem with your fetch operation:', error);
});
			`,
			want: &requestNewTarget{
				Method: "GET",
				Path:   "/misc/response/fetch/basic.action",
			},
		},
		{
			name: "xhr post",
			code: `
	// 创建一个新的 XMLHttpRequest 对象
var xhr = new XMLHttpRequest;

// 配置请求类型为 POST，以及目标 URL
xhr.open('POST', 'deep.js', true);

// 设置所需的 HTTP 请求头
xhr.setRequestHeader('HackedJS', 'AAA');

// 设置请求完成后的回调函数
xhr.onreadystatechange = function() {
  // 检查请求是否完成
  if (xhr.readyState === XMLHttpRequest.DONE) {
    // 检查请求是否成功
    if (xhr.status === 200) {
      // 请求成功，处理响应数据
      console.log(xhr.responseText);
    } else {
      // 请求失败，打印状态码
      console.error('Request failed with status:', xhr.status);
    }
  }
};

// 发送请求，可以在此处发送任何需要的数据
xhr.send();;
			`,
			want: &requestNewTarget{
				Method: "POST",
				Path:   "deep.js",
				Header: []*ypb.KVPair{
					{Key: "HackedJS", Value: "AAA"},
				},
			},
		},
	}

	for _, data := range checkDatas {
		t.Run(data.name, func(t *testing.T) {
			check(t, data.code, data.want)
		})
	}
}
