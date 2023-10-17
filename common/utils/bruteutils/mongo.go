package bruteutils

import (
	"context"
	"fmt"
	"net"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDialer struct {
}

var defaultMongoDialer = &MongoDialer{}

func (d *MongoDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return netx.DialContext(ctx, address)
}

var mongoAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mongodb",
	DefaultPorts:     "27017",
	DefaultUsernames: append([]string{"root", "admin", "mongodb"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 27017)
		result := i.Result()

		ctx := context.Background()
		host, port, _ := utils.ParseStringToHostPort(i.Target)
		addr := fmt.Sprintf("mongodb://%s:%d", host, port)
		clientOptions := options.Client().ApplyURI(addr).SetDialer(defaultMongoDialer)
		mgoCli, err := mongo.Connect(ctx, clientOptions)
		if err != nil {
			log.Errorf("connect unauath mongodb failed: %s", err)
			return result
		}
		defer mgoCli.Disconnect(ctx)

		err = mgoCli.Ping(ctx, nil)
		if err != nil {
			log.Errorf("ping unauth mongodb failed: %s", err)
			return result
		}

		_, err = mgoCli.ListDatabaseNames(ctx, bson.M{})
		if err != nil {
			log.Errorf("ping unauth mongodb failed: %s", err)
			return result
		}
		result.Username = ""
		result.Password = ""
		result.Finished = true
		result.Ok = true
		return result
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		username := i.Username
		password := i.Password
		host, port, _ := utils.ParseStringToHostPort(appendDefaultPort(i.Target, 27017))
		ctx := context.Background()

		addr := fmt.Sprintf("mongodb://%s:%d", host, port)
		clientOptions := options.Client().ApplyURI(addr).SetDialer(defaultMongoDialer).SetAuth(options.Credential{Username: username, Password: password})

		mgoCli, err := mongo.Connect(ctx, clientOptions)
		if err != nil {
			autoSetFinishedByConnectionError(err, result)
			return result
		}
		defer mgoCli.Disconnect(ctx)

		err = mgoCli.Ping(ctx, nil)
		if err != nil {
			autoSetFinishedByConnectionError(err, result)
			return result
		}

		result.Finished = true
		result.Ok = true
		return result
	},
}
