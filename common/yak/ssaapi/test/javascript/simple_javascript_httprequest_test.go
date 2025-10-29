package javascript

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_JS_XMLHttpRequest(t *testing.T) {
	t.Run("simple get request", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("simple.js", `let xhr1 =new XMLHttpRequest()
		xhr1.open('GET', 'http://example.com')
		xhr1.send()
		xhr1.send("123")
		xhr1.addEventListener('load', function () {
		console.log(this.response)
		})`)

		ssatest.CheckSyntaxFlowWithFS(t, fs,
			`XMLHttpRequest().open(* as $method, * as $url)`,
			map[string][]string{
				"url":    {"\"http://example.com\""},
				"method": {"\"GET\""},
			}, false,
			ssaapi.WithLanguage(ssaconfig.JS),
		)
	})

	t.Run("simple post request", func(t *testing.T) {
		code := `
	const data1 = {
       name: 'job',
       age: '11',
    }
    let xhr1 = new XMLHttpRequest()
    xhr1.open('POST', 'http://example1.com')
    const usp = new URLSearchParams(data)
    const query = usp.toString()
    xhr1.setRequestHeader('Content-type', 'application/x-www-form-urlencoded')
    xhr1.send(query)
    xhr1.addEventListener('load', function () {
        console.log(this.response)
    })

const data2 = {
       name: 'job',
       age: '22',
    }
    let xhr2 = new XMLHttpRequest()
    xhr2.open('POST', 'http://example2.com')
    const usp = new URLSearchParams(data)
    const query = usp.toString()
    xhr2.setRequestHeader('Content-type', 'application/x-www-form-urlencoded')
    xhr2.send(query)
    xhr2.addEventListener('load', function () {
        console.log(this.response)
    })

   `
		// TODO: 获取post的data 并且data要与url、method关联
		ssatest.CheckSyntaxFlow(t, code,
			`XMLHttpRequest() as $xhr
			$xhr.open(* as $method, * as $url)
			$xhr.send(* as $data)
			`,
			map[string][]string{
				"url":    {"\"http://example1.com\"", "\"http://example2.com\""},
				"method": {"\"POST\"", "\"POST\""},
				// "data": {"\"name=job&age=12\"", "\"name=job&age=12\""},
			},
			ssaapi.WithLanguage(ssaconfig.JS),
		)
	})

}

func TestJs_JQuery(t *testing.T) {
	t.Run("test jQuery $.ajax", func(t *testing.T) {
		code := `$.ajax({ //统计访问量
    url:'/foot_action!getCount.action',
    type: 'POST',
    dataType: 'json',
    cache:false,
    data: {url:window.location.href},
    timeout: 5000,
    error: function(){
    },
    success: function(result){
     $("#fwls").html(result.count);
        $("#fwl").html(result.count1);
    }
	});`

		ssatest.CheckSyntaxFlow(t, code,
			`/\$/.ajax(* as $obj)
			$obj.type as $method 
			$obj.url as $url
			$obj.data as $data
			$obj.dataType as $dataType
			`,
			map[string][]string{
				"url":      {"\"/foot_action!getCount.action\""},
				"method":   {"\"POST\""},
				"dataType": {"\"json\""},
				// "data":     {"\"{url:window.location.href}\""},
			},
			ssaapi.WithLanguage(ssaconfig.JS),
		)
	})

	t.Run("test jQuery $.post", func(t *testing.T) {
		code := `$.post({
	  url: 'https://jsonplaceholder.typicode.com/posts',
	  contentType: 'application/json',
	  data: JSON.stringify(formData),
	  success: function(response) {
	    // ...
	  },
	  error: function(xhr, status, error) {
	    // ...
	  }
	});
		$.post({
	  url: 'https://tests.com',
	  contentType: 'application/json',
	  data: "aaa",
	  success: function(response) {
	    // ...
	  },
	  error: function(xhr, status, error) {
	    // ...
	  }
	});
	`
		ssatest.CheckSyntaxFlow(t, code,
			`/\$/.post(* as $obj)
			$obj.url as $url
			$obj.contentType as $contentType
			$obj.data as $data
			`,
			map[string][]string{
				"url":         {"\"https://jsonplaceholder.typicode.com/posts\"", "\"https://tests.com\""},
				"contentType": {"\"application/json\"", "\"application/json\""},
				"data":        {"Undefined-JSON.stringify(valid)(Undefined-formData)", "\"aaa\""},
			},
			ssaapi.WithLanguage(ssaconfig.JS))
	})
}

func Test_JS_Fetch(t *testing.T) {
	code := `
	fetch('http://example.com')
	  .then(response => {
	    if (!response.ok) {
	      throw new Error('Network response was not ok');
	    }
	    return response.json();
	  })
	  .then(data => {
	    console.log(data);
	  })
	  .catch(error => {
	    console.error('There has been a problem with your fetch operation:', error);
	  });

	const data = {
	  key1: 'value1',
	};

	fetch('https://example.com/api/resource', {
	  method: 'POST',
	  headers: {
	    'Content-Type': 'application/json'
	  },
	  body: JSON.stringify(data)
	})
	.then(response => response.json())
	.then(data => {
	  console.log('Success:', data);
	})
	.catch((error) => {
	  console.error('Error:', error);
	});
	`
	ssatest.CheckSyntaxFlow(t, code,
		`fetch(* as $url,* as $obj)
		$obj.method as $method
		$obj.body as $body
		$obj.headers as $headers
		`,
		// TODO: 的对齐 和处理make(object) make(any)从数据库读取以后没有类型了的问题
		map[string][]string{
			"url":    {"\"http://example.com\"", "\"https://example.com/api/resource\""},
			"method": {"\"POST\""},
			// "body":    {"Undefined-JSON.stringify(valid)(make(object{}))"},
			// "headers": {"make(object{})"},
		},
		ssaapi.WithLanguage(ssaconfig.JS))
}

func Test_JS_Axios(t *testing.T) {
	t.Run("test axios get", func(t *testing.T) {
		code := `axios.get('http://example.com')
	      .then(response => (this.info = response))
	      .catch(function (error) { // 请求失败处理
	        console.log(error);
	    });
			axios.post('/user', {
			firstName: 'Fred',
			lastName: 'Flintstone'
		  })
		  .then(function (response) {
			console.log(response);
		  })
		  .catch(function (error) {
			console.log(error);
		  });
	`
		// TODO: 处理post body
		ssatest.CheckSyntaxFlow(t, code,
			`axios.get(* as $getUrl)
			axios.post(* as $postUrl, *  as $data)
			`,
			map[string][]string{
				"getUrl":  {"\"http://example.com\""},
				"postUrl": {"\"/user\""},
			}, ssaapi.WithLanguage(ssaconfig.JS))
	})

	t.Run("test axios post", func(t *testing.T) {
		code := `axios.post('/user', {
	    firstName: 'a',
	    lastName: 'b'
	  })
	  .then(function (response) {
	    console.log(response);
	  })
	  .catch(function (error) {
	    console.log(error);
	  });`
		ssatest.CheckSyntaxFlow(t, code,
			//TODO: handler data
			`axios.post(* as $url, * as $data)`,
			map[string][]string{
				"url": {"\"/user\""},
			}, ssaapi.WithLanguage(ssaconfig.JS))
	})
	t.Run("test http request by config ", func(t *testing.T) {
		code := `
	axios({
	  method: 'post',
	  url: '/user/12345',
	  data: {
	    key : 'value'
	  }
	});`

		ssatest.CheckSyntaxFlow(t, code,
			`axios(* as $config)
			$config.method as $method
			$config.url as $url
			$config.data as $data
			`,
			map[string][]string{
				"method": {"\"post\""},
				"url":    {"\"/user/12345\""},
				// "data":   {"\"key\":\"value\""},
			}, ssaapi.WithLanguage(ssaconfig.JS))
	})

}
