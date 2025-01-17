package yakdocker

import (
	"github.com/docker/docker/api/types"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"path"
)

func pull(imageName string, opt ...Option) error {
	config := NewConfig(opt...)
	client, err := config.GetDockerClient()
	if err != nil {
		return err
	}

	imagePath, splitImageName := path.Split(imageName)
	if splitImageName == "" {
		return utils.Errorf("(%v)'s image name is empty", imageName)
	}
	if imagePath == "" {
		imageName = path.Join(`docker.io/library`, splitImageName)
	}

	output, err := client.ImagePull(config.Context, imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	io.Copy(os.Stdout, ctxio.NewReader(config.Context, output))
	return nil
}
