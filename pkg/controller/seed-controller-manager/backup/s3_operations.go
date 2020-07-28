/*
Copyright 2020 The Kubermatic Kubernetes Platform contributors.

Copyright 2017 the Velero contributors. (func parseCronSchedule)

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

package backup

import (
	"context"
	"fmt"
	kubermaticv1 "github.com/kubermatic/kubermatic/pkg/crd/kubermatic/v1"
	"github.com/minio/minio-go"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/snapshot"
	"go.uber.org/zap"
	"os"
	"time"
)

type s3BackendOperations struct {
	snapshotDir       string
	s3Endpoint        string
	s3BucketName      string
	s3AccessKeyID     string
	s3SecretAccessKey string
}

func (s3ops *s3BackendOperations) takeSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string, cluster *kubermaticv1.Cluster) error {
	sp := snapshot.NewV3(log.Desugar())

	snapshotFileName := fmt.Sprintf("/%s/%s", s3ops.snapshotDir, fileName)

	deadlinedCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := sp.Save(deadlinedCtx, *getEtcdConfig(cluster), snapshotFileName); err != nil {
		return err
	}

	return nil
}

func getEtcdConfig(cluster *kubermaticv1.Cluster) *clientv3.Config {
	clusterSize := cluster.Spec.ComponentsOverride.Etcd.ClusterSize
	if clusterSize == 0 {
		clusterSize = defaultClusterSize
	}
	endpoints := []string{}
	for i := 0; i < clusterSize; i++ {
		endpoints = append(endpoints, fmt.Sprintf("etcd-%d.etcd.%s.svc.cluster.local:2380", i, cluster.Status.NamespaceName))
	}
	return &clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 2 * time.Second,
	}
}

func (s3ops *s3BackendOperations) getS3Client() (*minio.Client, error) {
	client, err := minio.New(s3ops.s3Endpoint, s3ops.s3AccessKeyID, s3ops.s3SecretAccessKey, true)
	if err != nil {
		return nil, err
	}
	client.SetAppInfo("kubermatic", "v0.1")
	return client, nil
}

func (s3ops *s3BackendOperations) uploadSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	client, err := s3ops.getS3Client()
	if err != nil {
		return err
	}

	exists, err := client.BucketExists(s3ops.s3BucketName)
	if err != nil {
		return err
	}
	if !exists {
		log.Debugf("Creating bucket: %v", s3ops.s3BucketName)
		if err := client.MakeBucket(s3ops.s3BucketName, ""); err != nil {
			return err
		}
	}

	snapshotFileName := fmt.Sprintf("/%s/%s", s3ops.snapshotDir, fileName)

	_, err = client.FPutObject(s3ops.s3BucketName, fileName, snapshotFileName, minio.PutObjectOptions{})

	return err
}

func (s3ops *s3BackendOperations) deleteSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	snapshotFileName := fmt.Sprintf("/%s/%s", s3ops.snapshotDir, fileName)
	return os.RemoveAll(snapshotFileName)
}

func (s3ops *s3BackendOperations) deleteUploadedSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	client, err := s3ops.getS3Client()
	if err != nil {
		return err
	}

	return client.RemoveObject(s3ops.s3BucketName, fileName)
}
