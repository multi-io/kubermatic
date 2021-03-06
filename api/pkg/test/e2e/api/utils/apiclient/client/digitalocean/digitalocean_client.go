// Code generated by go-swagger; DO NOT EDIT.

package digitalocean

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
)

// New creates a new digitalocean API client.
func New(transport runtime.ClientTransport, formats strfmt.Registry) ClientService {
	return &Client{transport: transport, formats: formats}
}

/*
Client for digitalocean API
*/
type Client struct {
	transport runtime.ClientTransport
	formats   strfmt.Registry
}

// ClientService is the interface for Client methods
type ClientService interface {
	ListDigitaloceanSizes(params *ListDigitaloceanSizesParams, authInfo runtime.ClientAuthInfoWriter) (*ListDigitaloceanSizesOK, error)

	ListDigitaloceanSizesNoCredentials(params *ListDigitaloceanSizesNoCredentialsParams, authInfo runtime.ClientAuthInfoWriter) (*ListDigitaloceanSizesNoCredentialsOK, error)

	SetTransport(transport runtime.ClientTransport)
}

/*
  ListDigitaloceanSizes Lists sizes from digitalocean
*/
func (a *Client) ListDigitaloceanSizes(params *ListDigitaloceanSizesParams, authInfo runtime.ClientAuthInfoWriter) (*ListDigitaloceanSizesOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewListDigitaloceanSizesParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "listDigitaloceanSizes",
		Method:             "GET",
		PathPattern:        "/api/v1/providers/digitalocean/sizes",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"https"},
		Params:             params,
		Reader:             &ListDigitaloceanSizesReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	success, ok := result.(*ListDigitaloceanSizesOK)
	if ok {
		return success, nil
	}
	// unexpected success response
	unexpectedSuccess := result.(*ListDigitaloceanSizesDefault)
	return nil, runtime.NewAPIError("unexpected success response: content available as default response in error", unexpectedSuccess, unexpectedSuccess.Code())
}

/*
  ListDigitaloceanSizesNoCredentials Lists sizes from digitalocean
*/
func (a *Client) ListDigitaloceanSizesNoCredentials(params *ListDigitaloceanSizesNoCredentialsParams, authInfo runtime.ClientAuthInfoWriter) (*ListDigitaloceanSizesNoCredentialsOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewListDigitaloceanSizesNoCredentialsParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "listDigitaloceanSizesNoCredentials",
		Method:             "GET",
		PathPattern:        "/api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/providers/digitalocean/sizes",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"https"},
		Params:             params,
		Reader:             &ListDigitaloceanSizesNoCredentialsReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	success, ok := result.(*ListDigitaloceanSizesNoCredentialsOK)
	if ok {
		return success, nil
	}
	// unexpected success response
	unexpectedSuccess := result.(*ListDigitaloceanSizesNoCredentialsDefault)
	return nil, runtime.NewAPIError("unexpected success response: content available as default response in error", unexpectedSuccess, unexpectedSuccess.Code())
}

// SetTransport changes the transport on the client
func (a *Client) SetTransport(transport runtime.ClientTransport) {
	a.transport = transport
}
