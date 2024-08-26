package bruteutils

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func MongoDBAuth(target, username, password string, needAuth bool) (bool, error) {
	ctx := utils.TimeoutContextSeconds(float64(defaultTimeout))
	host, port, _ := utils.ParseStringToHostPort(appendDefaultPort(target, 27017))
	addr := fmt.Sprintf("mongodb://%s:%d", host, port)
	clientOptions := options.Client().ApplyURI(addr).SetDialer(defaultDialer)
	if needAuth {
		clientOptions = clientOptions.SetAuth(options.Credential{Username: username, Password: password})
	}

	mgoCli, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return false, err
	}
	defer mgoCli.Disconnect(ctx)

	err = mgoCli.Ping(ctx, nil)
	if err != nil {
		return false, err
	}

	return true, nil
}

var mongoAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mongodb",
	DefaultPorts:     "27017",
	DefaultUsernames: append([]string{"root", "admin", "mongodb"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 27017)
		result := i.Result()

		ok, err := MongoDBAuth(i.Target, "", "", false)
		if err != nil {
			log.Errorf("mongodb unauth verify failed: %v", err)
		}
		result.Ok = ok
		return result
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 27017)
		result := i.Result()

		ok, err := MongoDBAuth(i.Target, i.Username, i.Password, true)
		if err != nil {
			log.Errorf("mongodb brute pass failed: %v", err)
		}
		result.Ok = ok
		return result
	},
}
