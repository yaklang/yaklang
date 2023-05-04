package spec

import (
	"fmt"
	"github.com/urfave/cli"
	"runtime"
)

type AMQPConfig struct {
	Host        string `json:"host" yaml:"host"`
	Port        int    `json:"port" yaml:"port"`
	Username    string `json:"username" yaml:"username"`
	Password    string `json:"password" yaml:"password"`
	VirtualHost string `json:"virtual_host" yaml:"virtual_host"`
}

func NewDefaultAMQPConfig() *AMQPConfig {
	return &AMQPConfig{
		Host:        "127.0.0.1",
		Port:        5676,
		Username:    "palm-user",
		Password:    "awesome-palm-password",
		VirtualHost: "palm",
	}
}

func (a *AMQPConfig) GetAMQPUrl() string {
	return fmt.Sprintf("amqp://%v:%v@%v:%v/%v",
		a.Username, a.Password, a.Host, a.Port, a.VirtualHost,
	)
}

type PostgresDBConfig struct {
	DatabaseName string `json:"database_name" yaml:"database_name"`
	Host         string `json:"host" yaml:"host"`
	Port         int    `json:"port" yaml:"port"`
	Username     string `json:"username" yaml:"username"`
	Password     string `json:"password" yaml:"password"`
}

func NewDefaultDatabaseConfig() *PostgresDBConfig {
	return &PostgresDBConfig{
		DatabaseName: "palm",
		Host:         "127.0.0.1",
		Port:         5435,
		Username:     "palm-user",
		Password:     "awesome-palm",
	}
}

func (p *PostgresDBConfig) GetPostgresParams() string {
	return fmt.Sprintf("host=%v port=%v user=%v dbname=%v password=%v sslmode=disable",
		p.Host, p.Port, p.Username, p.DatabaseName, p.Password,
	)
}

type AuditLogConfig struct {
	ServerAddr        string `json:"server_addr" yaml:"server_addr"`
	PageSize          int    `json:"page_size" yaml:"page_size"`
	FailReadMaxTicket int    `json:"fail_read_max_ticket" yaml:"fail_read_max_ticket"`
}

func NewDefaultAuditLogConfig() *AuditLogConfig {
	return &AuditLogConfig{
		ServerAddr:        "http://192.168.253.83:11001/access/query-by-type",
		PageSize:          2000,
		FailReadMaxTicket: 10,
	}
}

func GetCliBasicConfig(idPrefix string) []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "server",
			Value: "127.0.0.1",
		},
		cli.IntFlag{
			Name:  "mq-port",
			Value: 5676,
		},
		cli.StringFlag{
			Name:  "mq-user",
			Value: "palm-user",
		},
		cli.StringFlag{
			Name:  "mq-pass",
			Value: "awesome-palm-password",
		},
		//cli.StringFlag{
		//	Name:  "token",
		//	Value: "",
		//},
		cli.StringFlag{
			Name:  "id",
			Usage: "NodeId",
			Value: fmt.Sprintf("%s-[%s]", idPrefix, runtime.GOOS+runtime.GOARCH),
		},
		cli.StringFlag{
			Name:  "server-port",
			Value: "8082",
			Usage: "port of web api",
		},
	}
}

func LoadAMQPConfigFromCliContext(c *cli.Context) *AMQPConfig {
	return &AMQPConfig{
		Host:        c.String("server"),
		Port:        c.Int("mq-port"),
		Username:    c.String("mq-user"),
		Password:    c.String("mq-pass"),
		VirtualHost: "palm",
	}
}
