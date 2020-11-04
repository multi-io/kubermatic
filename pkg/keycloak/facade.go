package keycloak

import "fmt"

type ClientData struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
}

type Facade interface {
	GetClientData(realmName string, clientID string) (*ClientData, error)
}

type RealmNotFoundError struct {
	RealmName string
}

func (err *RealmNotFoundError) Error() string {
	return fmt.Sprintf("Realm not found: %s", err.RealmName)
}

type ClientNotFoundError struct {
	RealmName string
	ClientID  string
}

func (err *ClientNotFoundError) Error() string {
	return fmt.Sprintf("Client not found: realm=%s clientId=%s", err.RealmName, err.ClientID)
}
