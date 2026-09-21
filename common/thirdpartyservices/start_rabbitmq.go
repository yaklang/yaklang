package thirdpartyservices

import (
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/yaklang/yaklang/common/dockerhttp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	RabbitMQHost          = "127.0.0.1"
	RabbitMQPort          = "5676"
	RabbitUser            = "palm-user"
	RabbitPass            = "awesome-palm-password"
	RabbitMQContainerName = "palm-mq"
	RabbitVHost           = "palm"
)

func GetAMQPUrl() string {
	name := RabbitUser
	pass := RabbitPass

	return fmt.Sprintf("amqp://%v:%v@%v:%v/%v",
		name, pass, RabbitMQHost, RabbitMQPort, RabbitVHost,
	)
}

func StartRabbitMQ() error {
	name := RabbitUser
	pass := RabbitPass

	amqpUrl := GetAMQPUrl()

	// 如果已经成功了
	conn, err := amqp.Dial(amqpUrl)
	if err == nil {
		_ = conn.Close()
		return nil
	}

	cli, err := dockerhttp.New(dockerhttp.FromEnv)
	if err != nil {
		return utils.Errorf("docker env is miss: %v", err)
	}
	defer cli.Close()

	log.Info("try to kill existed rabbit mq container")
	err = cli.ContainerKill(utils.TimeoutContext(10*time.Second), RabbitMQContainerName, dockerhttp.ContainerKillOptions{Signal: "SIGKILL"})
	if err != nil {
		log.Errorf("kill %v failed: %s", RabbitMQContainerName, err)
	}

	var startNewContainer bool
	_, err = cli.ContainerInspect(utils.TimeoutContext(10*time.Second), RabbitMQContainerName)
	if dockerhttp.IsNotFound(err) {
		startNewContainer = true
	} else if err != nil {
		return fmt.Errorf("inspect service container: %w", err)
	}

	if !startNewContainer {
		err = cli.ContainerStart(
			utils.TimeoutContext(10*time.Second),
			RabbitMQContainerName,
		)
		if err != nil {
			return utils.Errorf("start existed rabbitmq container failed: %s", err)
		}
	} else {

		log.Infof("creating rabbitmq container")
		resp, err := cli.ContainerCreate(
			utils.TimeoutContext(10*time.Second),
			&dockerhttp.ContainerConfig{
				ExposedPorts: map[string]struct{}{
					"15672/tcp": {},
					"5672/tcp":  {},
				},
				Env: []string{
					fmt.Sprintf("RABBITMQ_DEFAULT_USER=%v", name),
					fmt.Sprintf("RABBITMQ_DEFAULT_PASS=%v", pass),
					fmt.Sprintf("RABBITMQ_DEFAULT_VHOST=%v", RabbitVHost),
				},
				Image: RabbitMQImageName,
			}, &dockerhttp.HostConfig{
				PortBindings: dockerhttp.PortMap{
					"15672/tcp": []dockerhttp.PortBinding{
						{
							HostIP:   "127.0.0.1",
							HostPort: fmt.Sprint(15672),
						},
					},
					"5672/tcp": []dockerhttp.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: fmt.Sprint(RabbitMQPort),
						},
					},
				},
			}, RabbitMQContainerName,
		)
		if err != nil {
			return utils.Errorf("create rabbitmq container failed: %s", err)
		}

		if len(resp.Warnings) > 0 {
			log.Warnf("%#v", resp.Warnings)
		}

		log.Infof("start to run %v", RabbitMQContainerName)
		err = cli.ContainerStart(utils.TimeoutContext(30*time.Second), resp.ID)
		if err != nil {
			return utils.Errorf("start %v failed: %s", PostgresContainerName, err)
		}

	}

	ticker := time.Tick(1 * time.Second)
	for {
		select {
		case <-ticker:
			conn, err := amqp.Dial(amqpUrl)
			if err != nil {
				continue
			}

			_ = conn.Close()
			return nil
		}
	}
}
