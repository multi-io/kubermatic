package keycloak

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

type backendMock struct {
	numberOfReads int
}

func (b *backendMock) GetClientData(realmName string, clientID string) (*ClientData, error) {
	b.numberOfReads++
	return &ClientData{
		IssuerURL:    "https://some.where",
		ClientID:     clientID,
		ClientSecret: "someSecret",
	}, nil
}

func TestCache(t *testing.T) {
	backend := &backendMock{}
	cache := NewCache(backend, 1*time.Second)

	assert.Equal(t, 0, backend.numberOfReads, "unexpected number of backend reads")

	data, err := cache.GetClientData("aRealm", "aClient")
	assert.Nil(t, err, "unexpected error %v", err)
	assert.NotNil(t, data, "data return expected")
	assert.Equal(t, "aClient", data.ClientID, "wrong clientId")
	assert.Equal(t, 1, backend.numberOfReads, "unexpected number of backend reads")

	data, err = cache.GetClientData("aRealm", "aClient")
	assert.Nil(t, err, "unexpected error %v", err)
	assert.Equal(t, "aClient", data.ClientID, "wrong clientId")
	assert.Equal(t, 1, backend.numberOfReads, "unexpected number of backend reads")

	_, _ = cache.GetClientData("aRealm", "aClient")
	_, err = cache.GetClientData("aRealm", "aClient")
	assert.Nil(t, err, "unexpected error %v", err)
	assert.Equal(t, 1, backend.numberOfReads, "unexpected number of backend reads")
	_, err = cache.GetClientData("anotherRealm", "anotherClient")
	assert.Nil(t, err, "unexpected error %v", err)
	assert.Equal(t, 2, backend.numberOfReads, "unexpected number of backend reads")
	_, _ = cache.GetClientData("anotherRealm", "anotherClient")
	data, err = cache.GetClientData("anotherRealm", "anotherClient")
	assert.Nil(t, err, "unexpected error %v", err)
	assert.Equal(t, "anotherClient", data.ClientID, "wrong clientId")
	assert.Equal(t, 2, backend.numberOfReads, "unexpected number of backend reads")

	// white-box test
	assert.Equal(t, 2, len(cache.data), "unexpected")

	time.Sleep(2 * time.Second)

	data, err = cache.GetClientData("aRealm", "aClient")
	assert.Nil(t, err, "unexpected error %v", err)
	assert.Equal(t, "aClient", data.ClientID, "wrong clientId")
	assert.Equal(t, 3, backend.numberOfReads, "unexpected number of backend reads")
	_, _ = cache.GetClientData("aRealm", "aClient")
	_, _ = cache.GetClientData("aRealm", "aClient")
	data, err = cache.GetClientData("aRealm", "aClient")
	assert.Nil(t, err, "unexpected error %v", err)
	assert.Equal(t, "aClient", data.ClientID, "wrong clientId")
	assert.Equal(t, 3, backend.numberOfReads, "unexpected number of backend reads")

	assert.Equal(t, 2, len(cache.data), "unexpected")
}
