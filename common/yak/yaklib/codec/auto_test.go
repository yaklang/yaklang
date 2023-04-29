package codec

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

const test = `

/*------------Modifier: < surizhou > 11/28/2021, 1:51:07 PM------------*/
var apub_5df6e3b3 = {"title":"\u70ed\u95e8\u8d44\u8baf\u5c4f\u853d\u5173\u952e\u8bcd\u914d\u7f6e","list":[{"keyword":"\u5b55\u5987\u5bb6\u5c5e\u66b4\u6253\u5973\u533b\u751f"},{"keyword":"\u6731\u8d24\u5065"}],"delID":[{"keyword":"15391756"},{"keyword":"19275116"},{"keyword":"18772778"},{"keyword":"18677255"},{"keyword":"15391756"},{"keyword":"5549728"},{"keyword":"18770637"},{"keyword":"5428085"},{"keyword":"11179498"},{"keyword":"5707182"},{"keyword":"8467146"},{"keyword":"10973052"},{"keyword":"10487446"},{"keyword":"5933788"},{"keyword":"10864490"}],"time":"2021-10-22 11:19:29","schemaId":"5df6e33498cff7722d3e6624","btype":"ch"}
`

func TestAutoDecode(t *testing.T) {
	var result = AutoDecode(`UVdFYXNkMTIzXzE%3D`)
	spew.Dump(result)
}

const test1 = `{"success":true,"message":"操作成功！","code":0,"result":"data:image/jpg;base64,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","timestamp":1654587991489}`

func TestAutoDecode2(t *testing.T) {
	var result = AutoDecode(test1)
	spew.Dump(result)
}

const test2 = `<script src="//mat1.gtimg.com/qqcdn/qqindex2021/qqdc/js/index.js"></script>
<script defer async type="text/javascript" src="https://mat1.gtimg.com/qqcdn/qqindex2021/libs/barrier/aria.js?appid=9327b8b06379d9d1728bbfbe2025ef9c" charset="utf-8"></script>
<script type="text/javascript" charset="utf-8">
$(".nav-list").append('<a href="javascript:void(0)" onclick="aria.start()" style="float: left;padding: 14px 0 16px;height: 30px;line-height: 30px;font-size: 14px;color: #333;" class="" dt-imp-once="true" dt-eid="em_func_btn" dt-params="func_btn_id=barrier_free&dt_element_path=[' + 'em_func_btn'+ ']">\u65e0\u969c\u788d\u6d4f\u89c8</a>');
</script>
  </body>`

func TestAutoDecode3(t *testing.T) {
	var result = AutoDecode(test2)
	spew.Dump(result)
}
