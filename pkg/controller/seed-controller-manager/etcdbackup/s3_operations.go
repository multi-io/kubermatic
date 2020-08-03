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

package etcdbackup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/minio/minio-go"
	"go.etcd.io/etcd/v3/clientv3"
	"go.uber.org/zap"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
)

type s3BackendOperations struct {
	snapshotDir       string
	s3Endpoint        string
	s3BucketName      string
	s3AccessKeyID     string
	s3SecretAccessKey string
}

func (s3ops *s3BackendOperations) takeSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string, cluster *kubermaticv1.Cluster) error {
	client, err := getEtcdClient(cluster)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Errorf("Failed to close etcd client: %v", err)
		}
	}()

	snapshotFileName := fmt.Sprintf("/%s/%s", s3ops.snapshotDir, fileName)
	partFile := snapshotFileName + ".part"
	defer func() {
		if err := os.RemoveAll(partFile); err != nil {
			log.Errorf("Failed to delete snapshot part file: %v", err)
		}
	}()

	var f *os.File
	f, err = os.OpenFile(partFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("could not open %s (%v)", partFile, err)
	}
	log.Debugf("created temporary db file: %v", partFile)

	var rd io.ReadCloser
	deadlinedCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rd, err = client.Snapshot(deadlinedCtx)
	if err != nil {
		return err
	}
	log.Debugf("fetching snapshot")
	var size int64
	size, err = io.Copy(f, rd)
	if err != nil {
		return err
	}
	if !hasChecksum(size) {
		return fmt.Errorf("sha256 checksum not found [bytes: %d]", size)
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	log.Debugf("fetched snapshot")

	if err = os.Rename(partFile, snapshotFileName); err != nil {
		return fmt.Errorf("could not rename %s to %s (%v)", partFile, snapshotFileName, err)
	}
	log.Debugf("saved %v", snapshotFileName)
	return nil
}

func getEtcdClient(cluster *kubermaticv1.Cluster) (*clientv3.Client, error) {
	clusterSize := defaultEtcdReplicas
	if v := cluster.Spec.ComponentsOverride.Etcd.Replicas; v != nil && *v > 0 {
		clusterSize = int(*v)
	}
	endpoints := []string{}
	for i := 0; i < clusterSize; i++ {
		endpoints = append(endpoints, fmt.Sprintf("etcd-%d.etcd.%s.svc.cluster.local:2380", i, cluster.Status.NamespaceName))
	}
	var err error
	for i := 0; i < 5; i++ {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   endpoints,
			DialTimeout: 2 * time.Second,
		})
		if err == nil && cli != nil {
			return cli, nil
		}
		time.Sleep(5 * time.Second)
	}
	return nil, fmt.Errorf("failed to establish client connection: %v", err)
}

// hasChecksum returns "true" if the file size "n"
// has appended sha256 hash digest.
func hasChecksum(n int64) bool {
	// 512 is chosen because it's a minimum disk sector size
	// smaller than (and multiplies to) OS page size in most systems
	return (n % 512) == sha256.Size
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
