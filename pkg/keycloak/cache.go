package keycloak

import (
	"sync"
	"time"
)

type Cache struct {
	keycloak   Facade
	expiryTime time.Duration
	data       map[key]*item
	mutex      sync.Mutex
}

type key struct {
	realmName string
	clientID  string
}

type item struct {
	data      *ClientData
	expiresAt time.Time
}

func NewCache(keycloak Facade, expiryTime time.Duration) *Cache {
	return &Cache{
		keycloak:   keycloak,
		expiryTime: expiryTime,
		data:       map[key]*item{},
	}
}

func (kc *Cache) GetClientData(realmName string, clientID string) (*ClientData, error) {
	kc.mutex.Lock()
	defer kc.mutex.Unlock()

	data := kc.getCachedClientData(realmName, clientID)
	if data == nil {
		backendData, err := kc.keycloak.GetClientData(realmName, clientID)
		if err != nil {
			return nil, err
		}
		data = backendData
		kc.putCachedClientData(realmName, clientID, data)
	}
	return data, nil
}

func (kc *Cache) getCachedClientData(realmName string, clientID string) *ClientData {
	key := key{realmName, clientID}
	item := kc.data[key]
	if item == nil {
		return nil
	}
	if item.expiresAt.Before(time.Now()) {
		delete(kc.data, key)
		return nil
	}
	return item.data
}

func (kc *Cache) putCachedClientData(realmName string, clientID string, data *ClientData) {
	key := key{realmName, clientID}
	item := &item{
		data:      data,
		expiresAt: time.Now().Add(kc.expiryTime),
		// alternative: use kc.expiryTime as a half-life (median) rather than constant time of expiry
		//    to possibly achieve more uniform distribution of expiry times
		//expiresAt: time.Now().Add(time.Duration(- float64(kc.expiryTime) * math.Log2(rand.Float64()))),
	}
	kc.data[key] = item
}
