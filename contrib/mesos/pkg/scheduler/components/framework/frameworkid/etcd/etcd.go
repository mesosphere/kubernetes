/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package etcd

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/components/framework/frameworkid"
	etcdstorage "k8s.io/kubernetes/pkg/storage/etcd"
	"k8s.io/kubernetes/pkg/tools"
)

type storage struct {
	frameworkid.LookupFunc
	frameworkid.StoreFunc
	frameworkid.RemoveFunc
}

func Store(client tools.EtcdClient, path string, ttl time.Duration) frameworkid.Storage {
	// TODO(jdef) validate Config
	return &storage{
		LookupFunc: func(_ context.Context) (string, error) {
			if response, err := client.Get(path, false, false); err != nil {
				if !etcdstorage.IsEtcdNotFound(err) {
					return "", fmt.Errorf("unexpected failure attempting to load framework ID from etcd: %v", err)
				}
			} else {
				return response.Node.Value, nil
			}
			return "", nil
		},
		RemoveFunc: func(_ context.Context) (err error) {
			if _, err = client.Delete(path, true); err != nil {
				if !etcdstorage.IsEtcdNotFound(err) {
					return fmt.Errorf("failed to delete framework ID from etcd: %v", err)
				}
			}
			return
		},
		StoreFunc: func(_ context.Context, id string) (err error) {
			_, err = client.Set(path, id, uint64(ttl.Seconds()))
			return
		},
	}
}
