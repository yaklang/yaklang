package thirdpartyservices

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/docker/docker/client"
)

const (
	PostgresPassword      = "awesome-palm"
	PostgresDatabaseName  = "palm"
	PostgresHost          = "127.0.0.1"
	PostgresPort          = 5435
	PostgresUser          = "palm-user"
	PostgresContainerName = "palm-postgres"
	PostgresImageName     = "postgres:12.4"
)

// PALM_POSTGRES_HOST
// PALM_POSTGRES_PORT
// PALM_POSTGRES_DB
// PALM_POSTGRES_USER
// PALM_POSTGRES_PASSWORD
func EnvOr(f, value string) string {
	r := os.Getenv(f)
	if r == "" {
		return value
	} else {
		return r
	}
}

func GetPostgresParams() string {
	dbname := EnvOr("PALM_POSTGRES_DB", PostgresDatabaseName)
	pwd := EnvOr("PALM_POSTGRES_PASSWORD", PostgresPassword)
	host := EnvOr("PALM_POSTGRES_HOST", PostgresHost)
	port := EnvOr("PALM_POSTGRES_PORT", fmt.Sprint(PostgresPort))
	user := EnvOr("PALM_POSTGRES_USER", PostgresUser)

	return fmt.Sprintf("host=%s port=%v user=%s dbname=%s password=%s sslmode=disable",
		host, port, user,
		dbname, pwd,
	)
}

func init() {
	/*
		switch runtime.GOOS {
		case "linux":
			dir := "/usr/share/palm/database"
			err := os.MkdirAll(dir, 0666)
			if err != nil {
				panic(fmt.Sprintf("prepare data failed: %s", err))
			}

		default:
			dir, err := os.Getwd()
			if err != nil {
				panic(fmt.Sprintf("get cwd failed: %s", err))
			}
		}
	*/
}

func PullPostgresImage() error {
	log.Infof("[POSTGRES] loading image or pulling image")
	cmd := exec.Command("docker", "pull", PostgresImageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return utils.Errorf("docker pull %s failed: %s", PostgresImageName, err)
	}
	return nil
}

func StartPostgres(pgdir string) error {
	if pgdir != "" {
		if !filepath.IsAbs(pgdir) {
			var err error
			pgdir, err = filepath.Abs(pgdir)
			if err != nil {
				return utils.Errorf("cannot get abs dir: %v", pgdir)
			}
		}
	}

	param := GetPostgresParams()
	password := PostgresPassword
	dbname := PostgresDatabaseName

	log.Info("detecting database connecting... pgdir=%v", pgdir)
	d, err := gorm.Open("postgres", param)
	if err == nil {
		log.Info("detected exsited database.")
		_ = d.Close()
		return nil
	} else {
		log.Warnf("open database failed: %v", err)
	}

	log.Info("try to start a database...")
	cli, err := client.NewEnvClient()
	if err != nil {
		return utils.Errorf("docker env is miss: %v", err)
	}
	defer cli.Close()

	err = cli.ContainerKill(utils.TimeoutContext(10*time.Second), PostgresContainerName, "SIGKILL")
	if err != nil {
		log.Errorf("kill %v failed: %s", PostgresContainerName, err)
	}

	var startNewContainer bool
	_, err = cli.ContainerInspect(utils.TimeoutContext(10*time.Second), PostgresContainerName)
	if err != nil {
		startNewContainer = true
	}

	if !startNewContainer {
		err = cli.ContainerStart(
			utils.TimeoutContext(10*time.Second),
			PostgresContainerName, types.ContainerStartOptions{},
		)
		if err != nil {
			return utils.Errorf("start existed postgres container failed: %s", err)
		}
	} else {
		var mounts []mount.Mount
		if pgdir != "" {
			log.Infof("create bind from %v ===> /var/lib/postgresql/data", pgdir)
			mounts = []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: pgdir,
					Target: "/var/lib/postgresql/data",
				},
			}
			_ = os.MkdirAll(pgdir, os.ModePerm)
		} else {
			log.Warnf("postgres data is buildin docker")
		}

		resp, err := cli.ContainerCreate(
			utils.TimeoutContext(10*time.Second),
			&container.Config{
				ExposedPorts: map[nat.Port]struct{}{
					"5432/tcp": {},
				},
				Env: []string{
					fmt.Sprintf("POSTGRES_PASSWORD=%s", password),
					fmt.Sprintf("POSTGRES_USER=%s", PostgresUser),
					fmt.Sprintf("POSTGRES_DB=%s", dbname),
				},
				Image: PostgresImageName,
				//Volumes: map[string]struct{}{
				//	fmt.Sprintf("%v:/var/lib/postgresql/data", pgdir): {},
				//},
			}, &container.HostConfig{
				PortBindings: nat.PortMap{
					"5432/tcp": []nat.PortBinding{
						{
							HostIP:   PostgresHost,
							HostPort: fmt.Sprint(PostgresPort),
						},
					},
				},
				Mounts: mounts,
			}, nil, PostgresContainerName,
		)
		if err != nil {
			return utils.Errorf("create postgres container failed: %s", err)
		}

		if len(resp.Warnings) > 0 {
			log.Warnf("%#v", resp.Warnings)
		}

		log.Infof("start to run %v", PostgresContainerName)
		err = cli.ContainerStart(utils.TimeoutContext(30*time.Second), resp.ID, types.ContainerStartOptions{})
		if err != nil {
			return utils.Errorf("start %v failed: %s", PostgresContainerName, err)
		}
	}

	ticker := time.Tick(1 * time.Second)
	count := 0
	for {
		select {
		case <-ticker:
			count++
			conn, err := gorm.Open("postgres", param)
			//conn, err := net.Dial("tcp", "127.0.0.1:5432")
			if err != nil {
				log.Warningf("try %v times... waiting for the postgres starting up...", err)
				continue
			}

			_ = conn.Close()
			return nil
		}
	}
}
