package thirdpartyservices

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

const PostgresImageName = "postgres:12.4"

var RabbitMQImageName = "rabbitmq:3-management"

func PullPostgresImage() error { return pullServiceImage(PostgresImageName) }
func PullRabbitMQImage() error { return pullServiceImage(RabbitMQImageName) }

func pullServiceImage(ref string) error {
	cli, err := dockerhttp.New(dockerhttp.FromEnv)
	if err != nil {
		return err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return cli.ImagePull(ctx, ref, dockerhttp.ImagePullOptions{})
}
