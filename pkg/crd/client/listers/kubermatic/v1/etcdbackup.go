// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/kubermatic/kubermatic/pkg/crd/kubermatic/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// EtcdBackupLister helps list EtcdBackups.
type EtcdBackupLister interface {
	// List lists all EtcdBackups in the indexer.
	List(selector labels.Selector) (ret []*v1.EtcdBackup, err error)
	// EtcdBackups returns an object that can list and get EtcdBackups.
	EtcdBackups(namespace string) EtcdBackupNamespaceLister
	EtcdBackupListerExpansion
}

// etcdBackupLister implements the EtcdBackupLister interface.
type etcdBackupLister struct {
	indexer cache.Indexer
}

// NewEtcdBackupLister returns a new EtcdBackupLister.
func NewEtcdBackupLister(indexer cache.Indexer) EtcdBackupLister {
	return &etcdBackupLister{indexer: indexer}
}

// List lists all EtcdBackups in the indexer.
func (s *etcdBackupLister) List(selector labels.Selector) (ret []*v1.EtcdBackup, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.EtcdBackup))
	})
	return ret, err
}

// EtcdBackups returns an object that can list and get EtcdBackups.
func (s *etcdBackupLister) EtcdBackups(namespace string) EtcdBackupNamespaceLister {
	return etcdBackupNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// EtcdBackupNamespaceLister helps list and get EtcdBackups.
type EtcdBackupNamespaceLister interface {
	// List lists all EtcdBackups in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1.EtcdBackup, err error)
	// Get retrieves the EtcdBackup from the indexer for a given namespace and name.
	Get(name string) (*v1.EtcdBackup, error)
	EtcdBackupNamespaceListerExpansion
}

// etcdBackupNamespaceLister implements the EtcdBackupNamespaceLister
// interface.
type etcdBackupNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all EtcdBackups in the indexer for a given namespace.
func (s etcdBackupNamespaceLister) List(selector labels.Selector) (ret []*v1.EtcdBackup, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.EtcdBackup))
	})
	return ret, err
}

// Get retrieves the EtcdBackup from the indexer for a given namespace and name.
func (s etcdBackupNamespaceLister) Get(name string) (*v1.EtcdBackup, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("etcdbackup"), name)
	}
	return obj.(*v1.EtcdBackup), nil
}
