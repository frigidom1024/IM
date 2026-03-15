package etcd

import (
	"fmt"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type ServiceRegistry struct {
	client  *clientv3.Client
	key     string
	value   string
	leaseID clientv3.LeaseID
}

func NewServiceRegistry(module, serverID, addr string) (*ServiceRegistry, error) {
	key := fmt.Sprintf("/gochat/%s/%s", module, serverID)

	lease, err := Client.Grant(ctx, 10)
	if err != nil {
		return nil, err
	}

	if _, err := Client.Put(ctx, key, addr, clientv3.WithLease(lease.ID)); err != nil {
		return nil, err
	}

	ch, err := Client.KeepAlive(ctx, lease.ID)
	if err != nil {
		return nil, err
	}

	sr := &ServiceRegistry{
		client:  Client,
		key:     key,
		value:   addr,
		leaseID: lease.ID,
	}

	go func() {
		for ka := range ch {
			_ = ka
		}
	}()

	return sr, nil
}

func (sr *ServiceRegistry) Close() error {

	_, err := sr.client.Revoke(ctx, sr.leaseID)

	return err

}

func (sr *ServiceRegistry) Client() *clientv3.Client {
	return sr.client
}

type ServiceDiscovery struct {
	client *clientv3.Client
	prefix string
}

func NewServiceDiscovery(module string) *ServiceDiscovery {
	return &ServiceDiscovery{
		client: Client,
		prefix: fmt.Sprintf("/gochat/%s", module),
	}
}

func (sd *ServiceDiscovery) GetServices() ([]string, error) {
	resp, err := sd.client.Get(ctx, sd.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	var addrs []string
	for _, kv := range resp.Kvs {
		addrs = append(addrs, string(kv.Value))
	}
	return addrs, nil
}

func (sd *ServiceDiscovery) WatchService() <-chan []string {
	ch := make(chan []string, 10)
	go func() {
		watchCh := sd.client.Watch(ctx, sd.prefix, clientv3.WithPrefix())
		for watchResp := range watchCh {
			var addrs []string
			for _, event := range watchResp.Events {
				switch event.Type {
				case clientv3.EventTypePut:
					addrs = append(addrs, string(event.Kv.Value))
				case clientv3.EventTypeDelete:
					addrs = append(addrs, string(event.Kv.Value))
				}
			}
			ch <- addrs
		}
	}()
	return ch
}
