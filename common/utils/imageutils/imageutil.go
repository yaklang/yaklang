package imageutils

import (
	"bytes"
	"image"
	"image/gif"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	_ "image/gif"
	jpg "image/jpeg"
	png "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

type ImageResult struct {
	MIMEType *mimetype.MIME
	RawImage []byte
}

func (i *ImageResult) SaveToFile() (string, error) {
	outputTmp := consts.GetDefaultYakitBaseTempDir()
	outputTmp = filepath.Join(outputTmp, "image-cache")
	_ = os.MkdirAll(outputTmp, os.ModePerm)
	outputTmp = filepath.Join(outputTmp, "image-result-"+utils.RandStringBytes(12)+".img")
	if err := os.WriteFile(outputTmp, i.RawImage, 0644); err != nil {
		return "", utils.Errorf("write image to file %s failed: %v", outputTmp, err)
	}
	return outputTmp, nil
}

func (i *ImageResult) Sha256() string {
	return utils.CalcSha256(i.RawImage)
}

func (i *ImageResult) ShortString() string {
	result := "data:" + i.MIMEType.String() + ";base64," + codec.EncodeBase64(i.RawImage)
	return utils.ShrinkString(result, 256)
}

func (i *ImageResult) ImageURL() string {
	return "data:" + i.MIMEType.String() + ";base64," + codec.EncodeBase64(i.RawImage)
}

func (i *ImageResult) Base64() string {
	return codec.EncodeBase64(i.RawImage)
}

func (i *ImageResult) String() string {
	return "ImageResult{\n" +
		"  MIMEType: " + i.MIMEType.String() + ", \n" +
		"  RawImage: " + utils.ShrinkString(codec.EncodeBase64(i.RawImage), 64) + ", \n" +
		"}"
}

var dataurlfinder = regexp.MustCompile(`data:\s*?image/([a-zA-Z0-9.+\-;=]{1,512}?)\s*;\s*base64\s*,\s*([a-zA-Z0-9+/\-_]+)`)
var needToDecodeRe = regexp.MustCompile(`data%3[Aa]\s*?image%2[fF]`)

// extractFromDataUri extracts images from a data URI string. helpful for extracting images from HTML or other text formats that may contain embedded images in data URI format.
func extractFromDataUri(i string) []*ImageResult {
	var results []*ImageResult
	// testdata
	idx := dataurlfinder.FindAllStringSubmatch(i, -1)
	for _, result := range idx {
		if len(result) > 2 {
			b64data := result[2]
			data, err := codec.DecodeBase64(b64data)
			if err != nil {
				continue
			}
			mime := mimetype.Detect(data)
			if mime != nil && strings.Contains(strings.ToLower(mime.String()), "image") {
				results = append(results, &ImageResult{
					MIMEType: mime,
					RawImage: data,
				})
			}
		}
	}

	if needToDecodeRe.MatchString(i) {
		result, _ := codec.QueryUnescape(i)
		if result != "" {
			idx := dataurlfinder.FindAllStringSubmatch(result, -1)
			for _, res := range idx {
				if len(res) > 2 {
					b64data := res[2]
					data, err := codec.DecodeBase64(b64data)
					if err != nil {
						continue
					}
					mime := mimetype.Detect(data)
					if mime != nil && strings.Contains(strings.ToLower(mime.String()), "image") {
						results = append(results, &ImageResult{
							MIMEType: mime,
							RawImage: data,
						})
					}
				}
			}
		}
	}

	return results
}

var imageSigBase64Re = map[string]*regexp.Regexp{
	"png": regexp.MustCompile(`iVBORw0KG[a-zA-Z0-9+/\-_]{64,}`),
	"jpg": regexp.MustCompile(`/9j/[a-zA-Z0-9+/\-_]{64,}`),
	"gif": regexp.MustCompile(`R0lGODlh[a-zA-Z0-9+/\-_]{64,}`),
}

var imageSigOffset = map[string][]byte{
	"png": []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a},
	"jpg": []byte{0xff, 0xd8, 0xff},
	"gif": []byte{0x47, 0x49, 0x46, 0x38},
}

// extractFromRawData
func extractFromRawData(i string) []*ImageResult {
	var results []*ImageResult
	for typeName, reins := range imageSigBase64Re {
		_ = typeName
		for _, result := range reins.FindAllString(i, -1) {
			b64data := result
			data, err := codec.DecodeBase64(b64data)
			if err != nil {
				continue
			}

			imageIns, _, err := image.Decode(bytes.NewBuffer(data))
			if imageIns == nil {
				if err != nil {
					log.Errorf("decode %v image error: %v", typeName, err)
				}
				continue
			}

			var buf bytes.Buffer
			switch typeName {
			case "png":
				err = png.Encode(&buf, imageIns)
				if err != nil {
					continue
				}
			case "gif":
				err = gif.Encode(&buf, imageIns, nil)
				if err != nil {
					continue
				}
			case "jpg":
				fallthrough
			default:
				err = jpg.Encode(&buf, imageIns, &jpg.Options{Quality: 100})
				if err != nil {
					continue
				}
			}

			data = buf.Bytes()

			mime := mimetype.Detect(data)
			if mime != nil && strings.Contains(strings.ToLower(mime.String()), "image") {
				results = append(results, &ImageResult{
					MIMEType: mime,
					RawImage: data,
				})
			}
		}
	}

	if len(results) <= 0 {
		for typeName, offsetFlag := range imageSigOffset {
			sigIndex := bytes.Index([]byte(i), offsetFlag)
			if sigIndex < 0 {
				continue
			}
			// try to extract image data from the offset
			data := []byte(i)[sigIndex:]
			reader := bytes.NewReader(data)
			imageIns, _, err := image.Decode(reader)
			if imageIns == nil {
				if err != nil {
					log.Errorf("decode %v image error: %v", typeName, err)
				}
				continue
			}
			var buf bytes.Buffer
			switch typeName {
			case "png":
				err = png.Encode(&buf, imageIns)
				if err != nil {
					continue
				}
			case "gif":
				err = gif.Encode(&buf, imageIns, nil)
				if err != nil {
					continue
				}
			case "jpg":
				fallthrough
			default:
				err = jpg.Encode(&buf, imageIns, &jpg.Options{Quality: 100})
			}
			if err != nil {
				log.Errorf("encode %v image error: %v", typeName, err)
				continue
			}
			data = buf.Bytes()
			mime := mimetype.Detect(data)
			if mime != nil && strings.Contains(strings.ToLower(mime.String()), "image") {
				results = append(results, &ImageResult{
					MIMEType: mime,
					RawImage: data,
				})
			}
		}
	}

	return results
}

// ExtractWildStringImage 是一个强大的、多策略的图片提取器，它能从任意输入（字符串、字节切片等）中地毯式搜索并解析出图片。
//
// 此函数的核心设计思想是“鲁棒性优先”，它会自动尝试用多种方法识别和解码可能隐藏在各种格式中的图片数据。
// 函数异步执行，并返回一个通道（channel），所有找到的图片结果都会通过这个通道传出。
// 为了效率，内部通过SHA256哈希对图片进行去重，确保同一张图片只会被发送一次。
//
// 提取过程按以下优先级顺序执行：
//  1. 首先，直接检测输入本身是否为完整的、原始的图片二进制数据（如PNG, JPEG文件内容）。
//  2. 若不是，则检查输入是否为一个完整的Base64字符串，并尝试解码。
//  3. 接着，将输入尝试作为JSON解析，并递归地在所有JSON值中寻找Base64编码的图片。
//  4. 然后，在整个输入中扫描符合 "data:image/..." 格式的Data URI并解码。
//  5. 之后，将输入视作 `multipart/form-data` 载荷进行解析，并对每一个部分递归调用本函数。
//  6. 最后，如果以上所有方法都没有找到图片，则会启用最终回退策略：在文本中通过正则和魔法字节（Magic Bytes）进行启发式扫描，寻找已知的图片格式签名（如 "iVBORw0KG...", "R0lGODlh" 等）。
//
// Example:
//
//	func main() {
//		// 模拟一个复杂的JSON输入，其中混合了Data URI和纯Base64字符串
//		complexInput := `
//		{
//			"request_id": "xyz-123",
//			"user_avatar": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR42mP8z8AAGwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMAA",
//			"attachments": [
//				{
//					"type": "file",
//					"data": "R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"
//				}
//			]
//		}
//		`
//
//		imageChannel := imageutils.ExtractWildStringImage(complexInput)
//
//		fmt.Println("Found the following images:")
//		for img := range imageChannel {
//			// img.ShortString() 会返回一个data URI格式的缩略字符串
//			fmt.Printf("- MIME: %s, Size: %d bytes, SHA256: %s\n", img.MIMEType, len(img.RawImage), img.Sha256())
//			// fmt.Println(img.ImageURL()) // 可以获取完整的Data URI
//		}
//	}
//
// @param i any - 任何可以被转为字符串的输入，如 string, []byte 等。
// @return chan *ImageResult - 一个用于接收图片结果的通道，当所有搜索完成后该通道会被关闭。
func ExtractWildStringImage(i any) chan *ImageResult {
	str := utils.InterfaceToString(i)
	bytesRaw := []byte(str)

	var visited = make(map[string]struct{})
	ch := make(chan *ImageResult)

	var count = new(int64)
	feedch := func(m *mimetype.MIME, raw []byte) {
		if m.IsImage() {
			r := &ImageResult{
				MIMEType: m,
				RawImage: raw,
			}
			if _, ok := visited[r.Sha256()]; !ok {
				visited[r.Sha256()] = struct{}{}
				ch <- r
				atomic.AddInt64(count, 1)
			}
		}
	}

	go func() {
		defer close(ch)

		result := mimetype.Detect(bytesRaw)
		if result.IsImage() {
			feedch(result, bytesRaw)
			return
		}

		if utils.IsBase64(str) {
			if results, err := codec.DecodeBase64(str); err == nil {
				mime := mimetype.Detect(results)
				if mime.IsImage() {
					feedch(mime, results)
					return
				}

			}
		}

		_ = jsonextractor.ExtractStructuredJSONFromStream(
			bytes.NewBuffer(bytesRaw),
			jsonextractor.WithObjectKeyValue(func(string string, data any) {
				if utils.IsBase64(utils.InterfaceToString(data)) {
					if results, err := codec.DecodeBase64(utils.InterfaceToString(data)); err == nil {
						mime := mimetype.Detect(results)
						if mime.IsImage() {
							feedch(mime, results)
						}
					}
				}
			}),
		)

		for _, i := range extractFromDataUri(str) {
			feedch(i.MIMEType, i.RawImage)
		}

		_, _ = lowhttp.FixMultipartBodyWithPart(bytesRaw, func(_header []byte, body []byte) {
			if len(body) > 0 {
				for i := range ExtractWildStringImage(body) {
					if i != nil {
						feedch(i.MIMEType, i.RawImage)
					}
				}
			}
		})

		if atomic.LoadInt64(count) <= 0 {
			for _, i := range extractFromRawData(str) {
				if i != nil {
					feedch(i.MIMEType, i.RawImage)
				}
			}
		}
	}()
	return ch
}

type orderedFile struct {
	idx      int
	filename string
}

func sortOrderedFile(ofs []*orderedFile) []*orderedFile {
	sorted := make([]*orderedFile, len(ofs))
	copy(sorted, ofs)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].idx > sorted[j].idx {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

// GetImageDimensionFromFile return image width and height
func GetImageDimensionFromFile(path string) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	img, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}
	return img.Width, img.Height, nil
}
