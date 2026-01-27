package thirdparty_bin

import (
	"context"
	"testing"
)

func TestManager_Start(t *testing.T) {
	err := DefaultManager.Install("ffmpeg", &InstallOptions{
		Force:      true,
		Context:    context.Background(),
		SystemType: "linux-amd64",
	})
	if err != nil {
		t.Fatalf("failed to install ffmpeg: %v", err)
	}

	// err := DefaultManager.Install("pandoc", &InstallOptions{
	// 	Force:      true,
	// 	Context:    context.Background(),
	// 	SystemType: "linux-amd64",
	// })
	// if err != nil {
	// 	t.Fatalf("failed to install ffmpeg: %v", err)
	// }
}
