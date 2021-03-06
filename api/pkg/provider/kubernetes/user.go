/*
Copyright 2020 The Kubermatic Kubernetes Platform contributors.

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

package kubernetes

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	kubermaticv1 "github.com/kubermatic/kubermatic/api/pkg/crd/kubermatic/v1"
	"github.com/kubermatic/kubermatic/api/pkg/provider"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewUserProvider returns a user provider
func NewUserProvider(client ctrlruntimeclient.Client, isServiceAccountFunc func(email string) bool) *UserProvider {
	return &UserProvider{
		client:               client,
		isServiceAccountFunc: isServiceAccountFunc,
	}
}

// UserProvider manages user resources
type UserProvider struct {
	client ctrlruntimeclient.Client
	// since service account are special type of user this functions
	// helps to determine if the given email address belongs to a service account
	isServiceAccountFunc func(email string) bool
}

// UserByID returns a user by the given ID
func (p *UserProvider) UserByID(id string) (*kubermaticv1.User, error) {
	user := &kubermaticv1.User{}
	if err := p.client.Get(context.Background(), ctrlruntimeclient.ObjectKey{Name: id}, user); err != nil {
		return nil, err
	}
	return user, nil
}

// UserByEmail returns a user by the given email
func (p *UserProvider) UserByEmail(email string) (*kubermaticv1.User, error) {
	users := &kubermaticv1.UserList{}
	if err := p.client.List(context.Background(), users); err != nil {
		return nil, err
	}

	for _, user := range users.Items {
		if strings.EqualFold(user.Spec.Email, email) {
			return user.DeepCopy(), nil
		}
	}

	return nil, provider.ErrNotFound
}

// CreateUser creates a new user.
//
// Note that:
// The name of the newly created resource will be unique and it is derived from the user's email address (sha256(email)
// This prevents creating multiple resources for the same user with the same email address.
//
// In the beginning I was considering to hex-encode the email address as it will produce a unique output because the email address in unique.
// The only issue I have found with this approach is that the length can get quite long quite fast.
// Thus decided to use sha256 as it produces fixed output and the hash collisions are very, very, very, very rare.
func (p *UserProvider) CreateUser(id, name, email string) (*kubermaticv1.User, error) {
	if len(id) == 0 || len(name) == 0 || len(email) == 0 {
		return nil, kerrors.NewBadRequest("Email, ID and Name cannot be empty when creating a new user resource")
	}

	if p.isServiceAccountFunc(email) {
		return nil, kerrors.NewBadRequest(fmt.Sprintf("cannot add a user with the given email %s as the name is reserved, please try a different email address", email))
	}

	user := &kubermaticv1.User{
		ObjectMeta: v1.ObjectMeta{
			Name: fmt.Sprintf("%x", sha256.Sum256([]byte(email))),
		},
		Spec: kubermaticv1.UserSpec{
			ID:    id,
			Name:  name,
			Email: email,
		},
	}

	if err := p.client.Create(context.Background(), user); err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateUser updates user.
func (p *UserProvider) UpdateUser(user *kubermaticv1.User) (*kubermaticv1.User, error) {
	if err := p.client.Update(context.Background(), user); err != nil {
		return nil, err
	}
	return user, nil
}
