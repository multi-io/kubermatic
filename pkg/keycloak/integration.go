package keycloak

import kubermaticv1 "github.com/kubermatic/kubermatic/pkg/crd/kubermatic/v1"

func Sys11AuthToOidcSettings(sys11Settings kubermaticv1.Sys11AuthSettings, keycloakFacade Facade) (*kubermaticv1.OIDCSettings, error) {
	result := &kubermaticv1.OIDCSettings{}

	clientData, err := keycloakFacade.GetClientData(sys11Settings.Realm, "metakube-cluster")
	if err != nil {
		return nil, err
	}

	result.IssuerURL = clientData.IssuerURL
	result.ClientID = clientData.ClientID
	result.ClientSecret = clientData.ClientSecret
	result.UsernameClaim = "email"
	result.GroupsClaim = "groups"

	return result, nil
}
