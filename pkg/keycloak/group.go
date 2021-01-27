package keycloak

type Group struct {
	keycloaks           []Facade
	keycloakByRealmName map[string]Facade
}

func NewGroup() *Group {
	return &Group{
		keycloaks:           []Facade{},
		keycloakByRealmName: map[string]Facade{},
	}
}

func (kg *Group) RegisterKeycloak(kcClient Facade) {
	kg.keycloaks = append(kg.keycloaks, kcClient)
}

func (kg *Group) GetClientData(realmName string, clientID string) (*ClientData, error) {
	kcClient, ok := kg.keycloakByRealmName[realmName]
	if ok {
		return kcClient.GetClientData(realmName, clientID)
	}

	for _, kc := range kg.keycloaks {
		data, err := kc.GetClientData(realmName, clientID)
		if _, ok := err.(*RealmNotFoundError); ok {
			// kc doesn't contain the realm, try the next one
			continue
		} else if err != nil {
			return nil, err
		}
		kg.keycloakByRealmName[realmName] = kc
		return data, nil
	}

	return nil, &RealmNotFoundError{realmName}
}
