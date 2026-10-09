package etcd

import (
	"context"
	"gochat/internal/pkg/config"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var Client *clientv3.Client

var ctx = context.Background()

func Init(cfg *config.EtcdConfig) error {
	var err error
	Client, err = clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		panic("failed to init etcd client: " + err.Error())
	}
	return nil
}

func Close() error {
	return Client.Close()
}

func GetClient() *clientv3.Client {

	return Client

}
