package crawler

import (
	"testing"
)

func TestGetInformationInPage_1(t *testing.T) {
	var data []*JavaScriptContent
	err := PageInformationWalker("html", `<body>

<a href="#">this is a ssa ir js test spa</a>

<script src='1.js'></script>	

<div id='app'></div>

<script>

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


</script>

<script src='defer.js' defer></script>
<script src='3.js'></script>

</body>`, WithFetcher_JavaScript(func(content *JavaScriptContent) {
		data = append(data, content)
	}))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if !(!data[0].IsCodeText && data[0].UrlPath == "1.js") {
		t.Error("1.js not found")
		t.FailNow()
	}

	if !(!data[2].IsCodeText && data[2].UrlPath == "3.js") {
		t.Error("3.js not found")
		t.FailNow()
	}

	if !(!data[3].IsCodeText && data[3].UrlPath == "defer.js") {
		t.Error("defer.js not found")
		t.FailNow()
	}

	if !(data[1].IsCodeText && len(data[1].Code) > 0) {
		t.Error("js code not found")
		t.FailNow()
	}
}

func TestExtractorPath(t *testing.T) {

}
