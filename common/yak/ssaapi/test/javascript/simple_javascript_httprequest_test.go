package javascript

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"strings"
	"testing"
)

func Test_JS_XMLHttpRequest(t *testing.T) {
	t.Run("simple get request", func(t *testing.T) {
		code := `
	let xhr1 =new XMLHttpRequest()

	xhr1.open('GET', 'http://example.com')
	xhr1.send()
    xhr1.send("123")
    xhr1.addEventListener('load', function () {
      console.log(this.response)
    })
   `
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			t.Fatal("prog parse error", err)
		}
		prog.Show()
		results, err := prog.SyntaxFlowWithError("XMLHttpRequest().open()")

		for _, result := range results {
			args := result.GetCallArgs()
			method := args[0]
			fmt.Printf("method: %v\n", method.String())
			require.Equal(t, method.String(), "\"GET\"")
			path := args[1]
			fmt.Printf("path: %v\n", path.String())
			require.Equal(t, path.String(), "\"http://example.com\"")
		}
	})

	t.Run("simple post request", func(t *testing.T) {
		code := `
	const data1 = {
       name: 'job',
       age: '12',
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
       age: '12',
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
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			t.Fatal("prog parse error", err)
		}
		prog.Show()
		requests, err := prog.SyntaxFlowWithError("XMLHttpRequest().open()")
		if err != nil {
			t.Fatal(err)
		}
		// TODO: 获取post的data 并且data要与url、method关联

		for _, result := range requests {
			args := result.GetCallArgs()
			if len(args) == 2 {
				fmt.Println("=====================================================")
				method := args[0]
				fmt.Printf("method: %v\n", method.String())
				require.Equal(t, method.String(), "\"POST\"")
				path := args[1]
				fmt.Printf("path: %v\n", path.String())
				require.Contains(t, path.String(), "http://example")
			}
		}

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
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			panic(err)
		}
		results, err := prog.SyntaxFlowWithError("$.ajax()")
		for _, result := range results {
			if err != nil {
				panic(err)
			}
			params, err := result.GetCallActualParams()
			if err != nil {
				panic(err)
			}
			members, err := params.GetMembers()
			if err != nil {
				panic(err)
			}

			match, operator, err := members.ExactMatch("url")
			if err != nil {
				panic(err)
			}
			assert.Equal(t, match, true, "match is false, except true")
			assert.Equal(t, len(operator.GetNames()), 4, "valueOperator number not match")
		}
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
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			panic(err)
		}
		results, err := prog.SyntaxFlowWithError("$.post()")
		for _, result := range results {
			if err != nil {
				panic(err)
			}
			params, err := result.GetCallActualParams()
			if err != nil {
				panic(err)
			}
			members, err := params.GetMembers()
			if err != nil {
				panic(err)
			}

			match, operator, err := members.ExactMatch("url")
			if err != nil {
				panic(err)
			}
			assert.Equal(t, match, true, "match is false, except true")
			assert.Equal(t, len(operator.GetNames()), 4, "valueOperator number not match")
		}
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
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
	if err != nil {
		panic(err)
	}
	results, err := prog.SyntaxFlowWithError("fetch()")
	for _, result := range results {
		args := result.GetCallArgs()
		if len(args) == 1 {
			fmt.Println("===========================")
			url := args[0]
			fmt.Println("Method: GET")
			fmt.Printf("URL:%v\n", url.String())
			require.Equal(t, "\"http://example.com\"", url.String())
			continue
		} else if len(args) == 2 {
			fmt.Println("===========================")
			url := args[0]
			fmt.Printf("URL :%v\n", url.String())
			require.Equal(t, "\"https://example.com/api/resource\"", url.String())
			extArg := args[1]
			if extArg.IsMake() {
				method := getDataFromMake(extArg.GetAllMember(), "\"method\"")
				body := getDataFromMake(extArg.GetAllMember(), "\"body\"")
				require.Equal(t, "\"POST\"", method)
				require.Equal(t, "\"key1\" : \"value1\"", body)
			}
		}
	}

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
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			panic(err)
		}
		results, err := prog.SyntaxFlowWithError("axios.get()")
		for _, result := range results {
			args := result.GetCallArgs()
			if len(args) == 1 {
				fmt.Println("===========================")
				url := args[0]
				fmt.Println("Method: GET")
				fmt.Printf("URL:%v\n", url.String())
				require.Equal(t, "\"http://example.com\"", url.String())
				continue
			}
		}
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
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			panic(err)
		}
		results, err := prog.SyntaxFlowWithError("axios.post()")
		for _, result := range results {
			args := result.GetCallArgs()
			require.Equal(t, 2, len(args))

			fmt.Println("===========================")
			url := args[0]
			fmt.Printf("URL :%v\n", url.String())
			require.Equal(t, "\"/user\"", url.String())
			extArg := args[1]
			if extArg.IsMake() {
				firstName := getDataFromMake(extArg.GetAllMember(), "\"firstName\"")
				require.Equal(t, "\"a\"", firstName)
				lastName := getDataFromMake(extArg.GetAllMember(), "\"lastName\"")
				require.Equal(t, "\"b\"", lastName)
			}

		}
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

		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			panic(err)
		}
		results, err := prog.SyntaxFlowWithError("axios()")
		for _, result := range results {
			args := result.GetCallArgs()
			require.Equal(t, 1, len(args))

			if args[0].IsMake() {
				method := getDataFromMake(args[0].GetAllMember(), "\"method\"")
				path := getDataFromMake(args[0].GetAllMember(), "\"url\"")
				data := getDataFromMake(args[0].GetAllMember(), "\"data\"")

				require.Equal(t, "\"post\"", method)
				require.Equal(t, "\"/user/12345\"", path)
				require.Equal(t, "\"key\":\"value\"", data)
			}
		}
	})

}

func getDataFromMake(members ssaapi.Values, expected string) string {
	var returnResult []string
	for _, member := range members {
		key := member.GetKey()
		if key.String() == expected {
			switch {
			case member.IsCall():
				args := member.GetCallArgs()
				for _, arg := range args {
					if arg.IsConstInst() {
						returnResult = append(returnResult, arg.String())
					}
					if arg.IsMake() {
						topdefs := arg.GetTopDefs()
						for i, topdef := range topdefs {
							if i == 0 {
								continue
							}
							if topdef.GetKey() != nil {
								returnResult = append(returnResult, fmt.Sprintf("%v : %v", topdef.GetKey().String(), topdef.String()))
							} else {
								returnResult = append(returnResult, topdefs.String())
							}
						}
					}
				}
			case member.IsMake():
				datas := member.GetAllMember()
				for _, data := range datas {
					if data.GetKey() != nil {
						returnResult = append(returnResult, fmt.Sprintf("%v:%v", data.GetKey().String(), data.String()))
					} else {
						returnResult = append(returnResult, data.String())
					}
				}
			default:
				returnResult = append(returnResult, member.String())
			}

		}
	}
	return strings.Join(returnResult, ",")
}
