package thirdpartyservices

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/streadway/amqp"
	"os"
	"os/exec"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	RabbitMQHost          = "127.0.0.1"
	RabbitMQPort          = "5676"
	RabbitUser            = "palm-user"
	RabbitPass            = "awesome-palm-password"
	RabbitMQContainerName = "palm-mq"
	RabbitMQImageName     = "rabbitmq:3-management"
	RabbitVHost           = "palm"
)

func GetAMQPUrl() string {
	name := RabbitUser
	pass := RabbitPass

	return fmt.Sprintf("amqp://%v:%v@%v:%v/%v",
		name, pass, RabbitMQHost, RabbitMQPort, RabbitVHost,
	)
}

func PullRabbitMQImage() error {
	log.Infof("[RABBITMQ] loading image or pulling image")
	cmd := exec.Command("docker", "pull", RabbitMQImageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return utils.Errorf("docker pull %s failed: %s", RabbitMQImageName, err)
	}
	//_, err := cli.ImagePull(utils.TimeoutContext(time.Hour*1), RabbitMQImageName, types.ImagePullOptions{})
	//if err != nil {
	//	return utils.Errorf("pull rabbitmq image [%v] failed: %s", RabbitMQContainerName, err)
	//}
	return nil
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

	cli, err := client.NewEnvClient()
	if err != nil {
		return utils.Errorf("docker env is miss: %v", err)
	}
	defer cli.Close()

	log.Info("try to kill existed rabbit mq container")
	err = cli.ContainerKill(utils.TimeoutContext(10*time.Second), RabbitMQContainerName, "SIGKILL")
	if err != nil {
		log.Errorf("kill %v failed: %s", RabbitMQContainerName, err)
	}

	var startNewContainer bool
	_, err = cli.ContainerInspect(utils.TimeoutContext(10*time.Second), RabbitMQContainerName)
	if err != nil {
		startNewContainer = true
	}

	if !startNewContainer {
		err = cli.ContainerStart(
			utils.TimeoutContext(10*time.Second),
			RabbitMQContainerName, types.ContainerStartOptions{},
		)
		if err != nil {
			return utils.Errorf("start existed rabbitmq container failed: %s", err)
		}
	} else {

		log.Infof("creating rabbitmq container")
		resp, err := cli.ContainerCreate(
			utils.TimeoutContext(10*time.Second),
			&container.Config{
				ExposedPorts: map[nat.Port]struct{}{
					"15672/tcp": {},
					"5672/tcp":  {},
				},
				Env: []string{
					fmt.Sprintf("RABBITMQ_DEFAULT_USER=%v", name),
					fmt.Sprintf("RABBITMQ_DEFAULT_PASS=%v", pass),
					fmt.Sprintf("RABBITMQ_DEFAULT_VHOST=%v", RabbitVHost),
				},
				Image: RabbitMQImageName,
			}, &container.HostConfig{
				PortBindings: nat.PortMap{
					"15672/tcp": []nat.PortBinding{
						{
							HostIP:   "127.0.0.1",
							HostPort: fmt.Sprint(15672),
						},
					},
					"5672/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: fmt.Sprint(RabbitMQPort),
						},
					},
				},
			}, nil, RabbitMQContainerName,
		)
		if err != nil {
			return utils.Errorf("create rabbitmq container failed: %s", err)
		}

		if len(resp.Warnings) > 0 {
			log.Warnf("%#v", resp.Warnings)
		}

		log.Infof("start to run %v", RabbitMQContainerName)
		err = cli.ContainerStart(utils.TimeoutContext(30*time.Second), resp.ID, types.ContainerStartOptions{})
		if err != nil {
			return utils.Errorf("start %v failed: %s", PostgresContainerName, err)
		}

		//cmd := exec.Command(
		//	"docker",
		//	"run", "-d",
		//	"--name", RabbitMQContainerName,
		//	"-e", fmt.Sprintf("RABBITMQ_DEFAULT_USER=%v", name),
		//	"-e", fmt.Sprintf("RABBITMQ_DEFAULT_PASS=%v", pass),
		//	"-e", fmt.Sprintf("RABBITMQ_DEFAULT_VHOST=%v", RabbitVHost),
		//	"-p", "127.0.0.1:15672:15672",
		//	"-p", fmt.Sprintf("%v:5672", RabbitMQPort),
		//
		//	"rabbitmq:3-management",
		//)
		//cmd.Stdout = os.Stdout
		//cmd.Stderr = os.Stderr
		//
		//if err = cmd.Run(); err != nil {
		//	log.Errorf("run %v %v failed: %v", cmd.Path, strings.Join(cmd.Args, " "), err)
		//	return errors.Errorf("run rabbitmq failed: %s", err)
		//}
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
