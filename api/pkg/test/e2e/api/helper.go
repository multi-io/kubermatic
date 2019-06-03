package e2e

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	apiv1 "github.com/kubermatic/kubermatic/api/pkg/api/v1"
	oidc "github.com/kubermatic/kubermatic/api/pkg/test/e2e/api/utils"
	apiclient "github.com/kubermatic/kubermatic/api/pkg/test/e2e/api/utils/apiclient/client"
	"github.com/kubermatic/kubermatic/api/pkg/test/e2e/api/utils/apiclient/client/project"
	"github.com/kubermatic/kubermatic/api/pkg/test/e2e/api/utils/apiclient/client/serviceaccounts"
	"github.com/kubermatic/kubermatic/api/pkg/test/e2e/api/utils/apiclient/client/tokens"
	"github.com/kubermatic/kubermatic/api/pkg/test/e2e/api/utils/apiclient/models"

	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

const (
	defaultIssuerURL = "http://dex.oauth:5556"
	host             = "localhost:8080"
	scheme           = "http"
	maxAttempts      = 8
	timeout          = time.Second * 4
)

type APIRunner struct {
	client      *apiclient.Kubermatic
	bearerToken runtime.ClientAuthInfoWriter
	test        *testing.T
}

func GetMasterToken() (string, error) {

	var hClient = &http.Client{
		Timeout: time.Second * 10,
	}

	u, err := getIssuerURL()
	if err != nil {
		return "", err
	}

	requestToken, err := oidc.GetOIDCReqToken(hClient, u, "http://localhost:8000")
	if err != nil {
		return "", err
	}

	login, password := oidc.GetOIDCClient()

	return oidc.GetOIDCAuthToken(hClient, requestToken, u, login, password)
}

func getHost() string {
	return host
}

func getScheme() string {
	return scheme
}

func getIssuerURL() (url.URL, error) {
	issuerURL := os.Getenv("KUBERMATIC_ISSUER")
	if len(issuerURL) == 0 {
		issuerURL = defaultIssuerURL
	}
	u, err := url.Parse(issuerURL)
	if err != nil {
		return url.URL{}, err
	}
	return *u, nil
}

// CreateAPIRunner util method to create APIRunner
func CreateAPIRunner(token string, t *testing.T) *APIRunner {
	client := apiclient.New(httptransport.New(getHost(), "", []string{getScheme()}), strfmt.Default)

	bearerTokenAuth := httptransport.BearerToken(token)
	return &APIRunner{
		client:      client,
		bearerToken: bearerTokenAuth,
		test:        t,
	}
}

// CreateProject creates a new project
func (r *APIRunner) CreateProject(name string) (*apiv1.Project, error) {
	params := &project.CreateProjectParams{Body: project.CreateProjectBody{Name: name}}
	params.WithTimeout(timeout)
	project, err := r.client.Project.CreateProject(params, r.bearerToken)
	if err != nil {
		return nil, err
	}

	var apiProject *apiv1.Project
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		apiProject, err = r.GetProject(project.Payload.ID, maxAttempts)
		if err != nil {
			return nil, err
		}

		if apiProject.Status == "Active" {
			break
		}
		time.Sleep(time.Second)
	}

	if apiProject.Status != "Active" {
		return nil, fmt.Errorf("project is not redy after %d attempts", maxAttempts)
	}

	return apiProject, nil
}

// GetProject gets the project with the given ID
func (r *APIRunner) GetProject(id string, attempts int) (*apiv1.Project, error) {
	params := &project.GetProjectParams{ProjectID: id}
	params.WithTimeout(timeout)

	var err error
	var project *project.GetProjectOK
	for attempt := 0; attempt <= attempts; attempt++ {
		project, err = r.client.Project.GetProject(params, r.bearerToken)
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return nil, err
	}

	return convertProject(project.Payload)
}

// UpdateProject updates the given project
func (r *APIRunner) UpdateProject(projectToUpdate *apiv1.Project) (*apiv1.Project, error) {
	params := &project.UpdateProjectParams{ProjectID: projectToUpdate.ID, Body: &models.Project{Name: projectToUpdate.Name}}
	params.WithTimeout(timeout)
	project, err := r.client.Project.UpdateProject(params, r.bearerToken)
	if err != nil {
		return nil, err
	}

	return convertProject(project.Payload)
}

func convertProject(project *models.Project) (*apiv1.Project, error) {
	apiProject := &apiv1.Project{}
	apiProject.Name = project.Name
	apiProject.ID = project.ID
	apiProject.Status = project.Status

	creationTime, err := time.Parse(time.RFC3339, project.CreationTimestamp.String())
	if err != nil {
		return nil, err
	}
	apiProject.CreationTimestamp = apiv1.NewTime(creationTime)

	return apiProject, nil
}

// DeleteProject deletes given project
func (r *APIRunner) DeleteProject(id string) error {
	params := &project.DeleteProjectParams{ProjectID: id}
	params.WithTimeout(timeout)
	if _, err := r.client.Project.DeleteProject(params, r.bearerToken); err != nil {
		return err
	}
	return nil
}

// CreateServiceAccount method creates a new service account
func (r *APIRunner) CreateServiceAccount(name, group, projectID string) (*apiv1.ServiceAccount, error) {
	params := &serviceaccounts.AddServiceAccountToProjectParams{ProjectID: projectID, Body: &models.ServiceAccount{Name: name, Group: group}}
	params.WithTimeout(timeout)
	params.SetTimeout(timeout)
	sa, err := r.client.Serviceaccounts.AddServiceAccountToProject(params, r.bearerToken)
	if err != nil {
		return nil, err
	}

	var apiServiceAccount *apiv1.ServiceAccount
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		apiServiceAccount, err = r.GetServiceAccount(sa.Payload.ID, projectID)
		if err != nil {
			return nil, err
		}

		if apiServiceAccount.Status == "Active" {
			break
		}
		time.Sleep(time.Second)
	}
	if apiServiceAccount.Status != "Active" {
		return nil, fmt.Errorf("service account is not redy after %d attempts", maxAttempts)
	}

	return apiServiceAccount, nil
}

// GetServiceAccount returns service account for given ID and project
func (r *APIRunner) GetServiceAccount(saID, projectID string) (*apiv1.ServiceAccount, error) {
	params := &serviceaccounts.ListServiceAccountsParams{ProjectID: projectID}
	params.WithTimeout(timeout)

	var err error
	var saList *serviceaccounts.ListServiceAccountsOK
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		saList, err = r.client.Serviceaccounts.ListServiceAccounts(params, r.bearerToken)
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return nil, err
	}

	for _, sa := range saList.Payload {
		if sa.ID == saID {
			return convertServiceAccount(sa)
		}
	}

	return nil, fmt.Errorf("service account %s not found", saID)
}

func convertServiceAccount(sa *models.ServiceAccount) (*apiv1.ServiceAccount, error) {
	apiServiceAccount := &apiv1.ServiceAccount{}
	apiServiceAccount.ID = sa.ID
	apiServiceAccount.Group = sa.Group
	apiServiceAccount.Name = sa.Name
	apiServiceAccount.Status = sa.Status

	creationTime, err := time.Parse(time.RFC3339, sa.CreationTimestamp.String())
	if err != nil {
		return nil, err
	}
	apiServiceAccount.CreationTimestamp = apiv1.NewTime(creationTime)

	return apiServiceAccount, nil
}

// AddTokenToServiceAccount creates a new token for service account
func (r *APIRunner) AddTokenToServiceAccount(name, saID, projectID string) (*apiv1.ServiceAccountToken, error) {
	params := &tokens.AddTokenToServiceAccountParams{ProjectID: projectID, ServiceaccountID: saID, Body: &models.ServiceAccountToken{Name: name}}
	params.WithTimeout(timeout)
	token, err := r.client.Tokens.AddTokenToServiceAccount(params, r.bearerToken)
	if err != nil {
		return nil, err
	}

	return convertServiceAccountToken(token.Payload)
}

func convertServiceAccountToken(saToken *models.ServiceAccountToken) (*apiv1.ServiceAccountToken, error) {
	apiServiceAccountToken := &apiv1.ServiceAccountToken{}
	apiServiceAccountToken.ID = saToken.ID
	apiServiceAccountToken.Name = saToken.Name
	apiServiceAccountToken.Token = saToken.Token

	expiry, err := time.Parse(time.RFC3339, saToken.Expiry.String())
	if err != nil {
		return nil, err
	}
	apiServiceAccountToken.Expiry = apiv1.NewTime(expiry)

	return apiServiceAccountToken, nil
}