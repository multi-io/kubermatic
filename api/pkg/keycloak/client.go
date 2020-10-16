package keycloak

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/kubermatic/kubermatic/api/pkg/util/errors"
)

type Client struct {
	keycloakURL   string
	adminUser     string
	adminPassword string
}

func NewClient(keycloakURL string, adminUser string, adminPassword string) *Client {
	return &Client{
		keycloakURL:   keycloakURL,
		adminUser:     adminUser,
		adminPassword: adminPassword,
	}
}

func extractResponseBodyJSON(resp *http.Response, result interface{}) error {
	defer resp.Body.Close() // nolint: errcheck

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.StatusCode, fmt.Sprintf("HTTP %v: %v", resp.StatusCode, string(body)))
	}

	err = json.Unmarshal(body, result)
	if err != nil {
		return err
	}

	return nil
}

// TODO use a golang oidc client library for authentication and token handling / refreshing?

func (kc *Client) getAdminToken() (string, error) {
	resp, err := http.PostForm(
		kc.keycloakURL+"/auth/realms/master/protocol/openid-connect/token",
		url.Values{
			"client_id":  {"admin-cli"},
			"grant_type": {"password"},
			"username":   {kc.adminUser},
			"password":   {kc.adminPassword},
		},
	)
	if err != nil {
		return "", err
	}

	bodyJSON := map[string]interface{}{}
	err = extractResponseBodyJSON(resp, &bodyJSON)
	if err != nil {
		return "", err
	}

	rawToken := bodyJSON["access_token"]
	token, ok := rawToken.(string)
	if !ok {
		return "", fmt.Errorf("could not parse access token from auth response")
	}

	return token, nil
}

func (kc *Client) GetClientData(realmName string, clientID string) (*ClientData, error) {
	token, err := kc.getAdminToken()
	if err != nil {
		return nil, err
	}

	// find client's internal (keycloak DB) id

	client := &http.Client{}
	req, err := http.NewRequest(
		"GET",
		kc.keycloakURL+"/auth/admin/realms/"+realmName+"/clients?viewableOnly=true&clientId="+url.QueryEscape(clientID),
		nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	type clientResult struct {
		ID       string `json:"id"`
		ClientID string `json:"clientID"`
	}

	bodyJSON := []clientResult{}
	err = extractResponseBodyJSON(resp, &bodyJSON)
	if err != nil {
		if httpErr, ok := err.(errors.HTTPError); ok && httpErr.StatusCode() == http.StatusNotFound {
			return nil, &RealmNotFoundError{realmName}
		}
		return nil, err
	}
	if len(bodyJSON) == 0 {
		return nil, &ClientNotFoundError{realmName, clientID}
	}

	clientInternalID := bodyJSON[0].ID

	// get client secret

	req, err = http.NewRequest(
		"GET",
		kc.keycloakURL+"/auth/admin/realms/"+realmName+"/clients/"+clientInternalID+"/client-secret",
		nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}

	type credentialResult struct {
		Value string `json:"value"`
	}

	credentialJSON := &credentialResult{}
	err = extractResponseBodyJSON(resp, credentialJSON)
	if err != nil {
		return nil, err
	}

	clientSecret := credentialJSON.Value
	if clientSecret == "" {
		return nil, fmt.Errorf("client secret for %v/%v not found", realmName, clientID)
	}

	return &ClientData{
		IssuerURL:    kc.keycloakURL + "/auth/realms/" + realmName,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}, nil
}
