package qwen

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func getAPIKey(t *testing.T) string {
	t.Helper()

	apikey := os.Getenv("DASHSCOPE_API_KEY")
	if apikey == "" {
		t.Skip("token is empty")
	}

	return apikey
}

func TestGetUploadCertificate(t *testing.T) {
	t.Parallel()
	apiKey := getAPIKey(t)
	ctx := context.TODO()

	modelCases := []string{
		QwenVLPlus,
		QwenAudioTurbo,
	}

	for _, model := range modelCases {
		resp, err := getUploadCertificate(ctx, model, apiKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
	}
}

// // this local file is not exist for other user
// func TestUploadingLocalImg(t *testing.T) {
// 	t.Parallel()
// 	ctx := context.TODO()

// 	homePath := os.Getenv("HOME")
// 	// ossFilePath, err := UploadLocalFile(ctx, homePath+"/Downloads/dog_and_girl.jpeg", QwenVLPlus, os.Getenv("DASHSCOPE_API_KEY"))
// 	ossFilePath, err := UploadLocalFile(ctx, homePath+"/Desktop/hello_world_female2.wav", QwenAudioTurbo, os.Getenv("DASHSCOPE_API_KEY"))

// 	t.Log(ossFilePath)
// 	require.NoError(t, err)
// 	require.NotEmpty(t, ossFilePath)
// }

func TestUploadingImageFromURL(t *testing.T) {
	t.Parallel()
	apiKey := getAPIKey(t)

	// network problem...
	// testImgURL := "https://github.com/yaklang/yaklang/common/ai/tongyi/blob/main/docs/static/img/parrot-icon.png"
	testImgURL := "https://pic.ntimg.cn/20140113/8800276_184351657000_2.jpg"

	ctx := context.TODO()
	// nolint:all
	var uploadCacher UploadCacher = nil

	ossFilePath, err := UploadFileFromURL(ctx, testImgURL, "qwen-vl-plus", apiKey, uploadCacher)

	require.NoError(t, err)
	require.NotEmpty(t, ossFilePath)
}

/*
func TestUploadingImageFromURLWithCache(t *testing.T) {
	t.Parallel()
	apiKey := getAPIKey(t)

	var uploadCacher UploadCacher = NewMemoryFileCache()
	ctx := context.TODO()

	homePath, _ := os.UserHomeDir()
	localFIlePath := homePath + "/Downloads/pandas_img.jpg"

	ossFilePath, err := UploadLocalFile(ctx, localFIlePath, "qwen-vl-plus", apiKey, uploadCacher)

	require.NoError(t, err)
	require.NotEmpty(t, ossFilePath)

	ossFilePath2, err := UploadLocalFile(ctx, localFIlePath, "qwen-vl-plus", apiKey, uploadCacher)

	require.NoError(t, err)
	require.Equal(t, ossFilePath, ossFilePath2)
}

*/
