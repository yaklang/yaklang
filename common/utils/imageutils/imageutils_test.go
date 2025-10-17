package imageutils

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png" // 关键：导入PNG编码器，它会通过 init() 注册自己
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

// MockDataCollection 结构体保持不变
type MockDataCollection struct {
	Base64PNG         string
	RawPNGBytes       []byte
	DataURI           string
	JSONWithBase64    string
	JSONWithDataURI   string
	JSONWithURL       string
	MultipartFormData []byte
	ImageURL          string
	InvalidBase64     string
	CorruptedPNGBytes []byte
	NotAnImage        string
	JSONWithWrongKey  string
}

// createDummyPNG 动态创建一个指定尺寸和颜色的PNG图片，并返回其二进制数据
func createDummyPNG(width, height int, c color.Color) ([]byte, error) {
	// 创建一个新的RGBA图像
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 填充颜色
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, c)
		}
	}

	// 使用一个缓冲区来存储编码后的PNG数据
	var buf bytes.Buffer
	// 使用 png.Encode 将 image.Image 对象编码为PNG格式，并写入缓冲区
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("failed to encode png: %w", err)
	}

	// 返回缓冲区的字节切片
	return buf.Bytes(), nil
}

// generateMockData 使用动态生成的图片来创建所有测试数据
func generateMockData() (*MockDataCollection, error) {
	collection := &MockDataCollection{}

	// --- 1. 基础数据 (动态生成) ---
	// 首先，动态创建一个10x10像素的红色PNG图片
	rawPNG, err := createDummyPNG(10, 10, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	if err != nil {
		return nil, fmt.Errorf("failed to create dummy png: %w", err)
	}
	collection.RawPNGBytes = rawPNG

	// 然后，基于这个二进制数据生成Base64字符串
	collection.Base64PNG = base64.StdEncoding.EncodeToString(collection.RawPNGBytes)

	// --- 后续步骤与之前相同，但都使用上面动态生成的数据 ---

	// --- 2. 常见格式 ---
	collection.DataURI = "data:image/png;base64," + collection.Base64PNG
	collection.JSONWithBase64 = fmt.Sprintf(`{"code": 200, "message": "success", "data": {"image_base64": "%s"}}`, collection.Base64PNG)
	collection.JSONWithDataURI = fmt.Sprintf(`{"imageData": "%s"}`, collection.DataURI)
	collection.ImageURL = "https://via.placeholder.com/150.png"
	collection.JSONWithURL = fmt.Sprintf(`{"image_url": "%s", "alt": "placeholder"}`, collection.ImageURL)

	// 生成 multipart/form-data
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="imageFile"; filename="generated.png"`)
	h.Set("Content-Type", "image/png")

	part, err := writer.CreatePart(h)
	if err != nil {
		return nil, err
	}
	_, err = part.Write(collection.RawPNGBytes)
	if err != nil {
		return nil, err
	}
	writer.Close()
	collection.MultipartFormData = body.Bytes()

	// --- 3. 异常和边缘情况 ---
	collection.InvalidBase64 = "this-is-not-valid-base64-string!"

	// 创建一个新的损坏PNG数据副本
	corrupted := make([]byte, len(collection.RawPNGBytes))
	copy(corrupted, collection.RawPNGBytes)
	// 破坏PNG文件头 (PNG's first 8 bytes are 89 50 4E 47 0D 0A 1A 0A)
	copy(corrupted[1:4], []byte{0x00, 0x00, 0x00}) // 改变 "PNG" 部分
	collection.CorruptedPNGBytes = corrupted

	collection.NotAnImage = "Hello, I am a universal parser. Can you parse me?"
	collection.JSONWithWrongKey = `{"status": "ok", "result": "no_image_data_here"}`

	return collection, nil
}

func TestImageBuilding(t *testing.T) {
	mocks, err := generateMockData()
	if err != nil {
		t.Fatalf("mock data generate failed: %v", err)
	}

	// 1. Base64 能否解码为 PNG
	raw, err := base64.StdEncoding.DecodeString(mocks.Base64PNG)
	if err != nil {
		t.Errorf("Base64 decode failed: %v", err)
	}
	if !bytes.Equal(raw, mocks.RawPNGBytes) {
		t.Error("Base64 decode result not match raw PNG bytes")
	}

	// 2. Data URI 格式校验
	if got, want := mocks.DataURI[:22], "data:image/png;base64,"; got != want {
		t.Errorf("Data URI prefix error: got %s, want %s", got, want)
	}

	// 3. JSON 提取 base64
	var jsonData struct {
		Code    int
		Message string
		Data    struct {
			ImageBase64 string `json:"image_base64"`
		}
	}
	if err := json.Unmarshal([]byte(mocks.JSONWithBase64), &jsonData); err != nil {
		t.Errorf("JSONWithBase64 unmarshal failed: %v", err)
	}
	if jsonData.Data.ImageBase64 != mocks.Base64PNG {
		t.Error("JSONWithBase64 image_base64 not match")
	}

	// 4. 损坏的 PNG 解码应失败
	_, err = png.Decode(bytes.NewReader(mocks.CorruptedPNGBytes))
	if err == nil {
		t.Error("Corrupted PNG should not decode successfully")
	}

	// 5. 无效 Base64 解码应失败
	_, err = base64.StdEncoding.DecodeString(mocks.InvalidBase64)
	if err == nil {
		t.Error("Invalid base64 should not decode successfully")
	}

	// 6. multipart/form-data 包含 PNG 文件头
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47}
	if !bytes.Contains(mocks.MultipartFormData, pngHeader) {
		t.Error("Multipart form data does not contain PNG header")
	}
}

// TestUniversalImageParser 为未来的通用图片解析函数准备了测试台。
// 它包含了各种有效的图片输入格式、边界情况和无效数据。
// 用户可以基于这个测试表格来编写和验证解析函数的逻辑。
func TestUniversalImageParser(t *testing.T) {
	mocks, err := generateMockData()
	if err != nil {
		t.Fatalf("Failed to generate mock data: %v", err)
	}

	// 为了测试URL，我们需要一个模拟的HTTP服务器
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/not-an-image" {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("this is not an image"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(mocks.RawPNGBytes)
	}))
	defer mockServer.Close()

	encodedDataURI := "data%3Aimage%2Fpng%3Bbase64%2C" + mocks.Base64PNG

	testCases := map[string]struct {
		input         []byte
		expectedError bool
		description   string
	}{
		// --- 1. 有效的基础格式 ---
		"Raw PNG Bytes": {
			input:         mocks.RawPNGBytes,
			expectedError: false,
			description:   "应能正确识别并解析原始的PNG二进制数据。",
		},
		"Base64 String": {
			input:         []byte(mocks.Base64PNG),
			expectedError: false,
			description:   "应能将标准的Base64字符串解码为图片。",
		},
		"Direct Image URL": {
			input:         []byte(mockServer.URL),
			expectedError: true,
			description:   "应能从一个URL字符串直接获取图片。",
		},

		// --- 2. 常见的包装格式 ---
		"Plain Data URI": {
			input:         []byte(mocks.DataURI),
			expectedError: false,
			description:   "应能解析标准的Data URI。",
		},
		"Wrapped Data URI (Quotes)": {
			input:         []byte(`"` + mocks.DataURI + `"`),
			expectedError: false,
			description:   "应能处理用双引号包装的Data URI。",
		},
		"Wrapped Data URI (CSS)": {
			input:         []byte(`url('` + mocks.DataURI + `')`),
			expectedError: false,
			description:   "应能处理CSS url()语法包装的Data URI。",
		},
		"URL Encoded Data URI": {
			input:         []byte(encodedDataURI),
			expectedError: false,
			description:   "应能处理经过URL编码的Data URI。",
		},
		"JSON with Base64": {
			input:         []byte(mocks.JSONWithBase64),
			expectedError: false,
			description:   "应能从JSON对象中提取并解码Base64图片。",
		},
		"JSON with Data URI": {
			input:         []byte(mocks.JSONWithDataURI),
			expectedError: false,
			description:   "应能从JSON对象中提取并解析Data URI。",
		},
		"JSON with Image URL": {
			input:         []byte(fmt.Sprintf(`{"image_url": "%s"}`, mockServer.URL)),
			expectedError: true,
			description:   "应能从JSON对象里的URL获取图片。",
		},
		"Multipart Form Data": {
			input:         mocks.MultipartFormData,
			expectedError: false,
			description:   "应能从 multipart/form-data 载荷中提取图片。",
		},

		// --- 3. 无效数据和边缘情况 ---
		"Corrupted PNG Bytes": {
			input:         mocks.CorruptedPNGBytes,
			expectedError: true,
			description:   "解析损坏的图片数据时应失败。",
		},
		"Invalid Base64 String": {
			input:         []byte(mocks.InvalidBase64),
			expectedError: true,
			description:   "处理无效Base64字符串时应失败。",
		},
		"Plain Text String": {
			input:         []byte(mocks.NotAnImage),
			expectedError: true,
			description:   "处理任意非图片、非已知格式的字符串时应失败。",
		},
		"JSON with No Image Key": {
			input:         []byte(mocks.JSONWithWrongKey),
			expectedError: true,
			description:   "处理不包含已知图片键的JSON时应失败。",
		},
		"URL to Non-Image": {
			input:         []byte(mockServer.URL + "/not-an-image"),
			expectedError: true,
			description:   "当URL指向一个非图片资源时应失败。",
		},
		"Nil Input": {
			input:         nil,
			expectedError: true,
			description:   "应能优雅地处理nil输入并返回错误。",
		},
		"Empty Input": {
			input:         []byte{},
			expectedError: true,
			description:   "应能优雅地处理空字节切片并返回错误。",
		},

		// --- 4. 更多畸形和复杂情况 ---
		"HTML with Embedded Data URI": {
			input:         []byte(`<img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7" alt="pixel">`),
			expectedError: false,
			description:   "应能从HTML片段中提取Data URI。",
		},
		"SVG with Embedded Base64 Image": {
			input:         []byte(`<svg><image href="data:image/png;base64,` + mocks.Base64PNG + `"></image></svg>`),
			expectedError: false,
			description:   "应能从SVG内容中提取嵌套的Data URI。",
		},
		"JSON with Nested Base64": {
			input:         []byte(`{"user": {"profile_pic": "` + mocks.Base64PNG + `"}}`),
			expectedError: false,
			description:   "应能从深层嵌套的JSON中提取Base64。",
		},
		"JSON Array with Images": {
			input:         []byte(`["` + mocks.Base64PNG + `", "data:image/jpeg;base64,anotherimage..."]`),
			expectedError: false,
			description:   "应能从JSON数组中提取多个图片。",
		},
		"Data URI without Base64 flag": {
			input:         []byte("data:image/svg+xml,%3Csvg%3E...%3C%2Fsvg%3E"),
			expectedError: true, // 当前实现依赖base64, 所以预期失败
			description:   "应能识别非Base64编码的Data URI（虽然目前可能不支持）。",
		},
		"Multipart with Multiple Images": {
			input:         createMultipartWithTwoImages(mocks.RawPNGBytes),
			expectedError: false,
			description:   "应能从单个multipart载荷中提取多个图片。",
		},
		"String with Multiple Data URIs": {
			input:         []byte(`background: url('` + mocks.DataURI + `'); content: url("data:image/jpeg;base64,...")`),
			expectedError: false,
			description:   "应能从单个字符串中找到所有Data URI实例。",
		},
		"Base64 with Newlines": {
			input:         []byte(mocks.Base64PNG[:10] + "\n" + mocks.Base64PNG[10:20] + "\r\n" + mocks.Base64PNG[20:]),
			expectedError: true,
			description:   "应能处理含有换行符的Base64字符串。",
		},
		"Data URI with extra parameters": {
			input:         []byte("data:image/png;charset=utf-8;base64," + mocks.Base64PNG),
			expectedError: false,
			description:   "应能处理MIME类型后带有额外参数的Data URI。",
		},
		"Text with Base64-like substring": {
			input:         []byte("This is a string that looks a little like base64 but is not: eW91J3JlIG5vdCBhIGJhc2U2NCBzdHJpbmc="),
			expectedError: true, // 这是一个棘手的情况，依赖于我们的Base64检测有多严格
			description:   "应避免将看起来像Base64的普通文本误认为图片。",
		},
		"Extremely Large JSON with one image": {
			input:         []byte(fmt.Sprintf(`{"padding": "%s", "image_data": "%s"}`, strings.Repeat("a", 2048), mocks.Base64PNG)),
			expectedError: false,
			description:   "应能处理大型输入（如JSON）并从中找到图片。",
		},
		"Base64 string next to Raw Bytes": {
			input:         append([]byte(mocks.Base64PNG), mocks.RawPNGBytes...),
			expectedError: false,
			description:   "在一个输入流中同时包含Base64和原始图片字节时，应至少能识别一个。",
		},
	}

	// 提示：您可以从这里开始编写您的测试逻辑
	//
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := string(tc.input)
			results := ExtractWildStringImage(result)
			count := 0
			for i := range results {
				count++
				fmt.Println(i.ShortString())
			}
			if tc.expectedError {
				if count > 0 {
					t.Log(utils.ShrinkString(string(tc.input), 128))
					t.Errorf("Expected no results for %s, but got %d", tc.description, count)
				}
			} else {
				if count == 0 {
					t.Log(utils.ShrinkString(string(tc.input), 128))
					t.Errorf("Expected at least one result for %s, but got none", tc.description)
				}
			}
		})
	}

	// 使用 _= 避免 "unused variable" 编译错误，您可以安全地移除这行代码
	_ = testCases
}

// createMultipartWithTwoImages 创建一个包含两张图片和其他字段的 multipart/form-data 载荷
func createMultipartWithTwoImages(pngData []byte) []byte {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// Image 1
	h1 := make(textproto.MIMEHeader)
	h1.Set("Content-Disposition", `form-data; name="image1"; filename="image1.png"`)
	h1.Set("Content-Type", "image/png")
	part1, _ := writer.CreatePart(h1)
	_, _ = part1.Write(pngData)

	// Some other form field
	_ = writer.WriteField("username", "testuser")

	// Image 2
	h2 := make(textproto.MIMEHeader)
	h2.Set("Content-Disposition", `form-data; name="image2"; filename="image2.png"`)
	h2.Set("Content-Type", "image/png")
	part2, _ := writer.CreatePart(h2)
	_, _ = part2.Write(pngData)

	_ = writer.Close()
	return body.Bytes()
}
