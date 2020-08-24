// Code generated by go-swagger; DO NOT EDIT.

package project

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"k8c.io/kubermatic/v2/pkg/test/e2e/api/utils/apiclient/models"
)

// ListExternalClustersReader is a Reader for the ListExternalClusters structure.
type ListExternalClustersReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *ListExternalClustersReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewListExternalClustersOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 401:
		result := NewListExternalClustersUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 403:
		result := NewListExternalClustersForbidden()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	default:
		result := NewListExternalClustersDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewListExternalClustersOK creates a ListExternalClustersOK with default headers values
func NewListExternalClustersOK() *ListExternalClustersOK {
	return &ListExternalClustersOK{}
}

/*ListExternalClustersOK handles this case with default header values.

ClusterList
*/
type ListExternalClustersOK struct {
	Payload models.ClusterList
}

func (o *ListExternalClustersOK) Error() string {
	return fmt.Sprintf("[GET /api/v2/projects/{project_id}/kubernetes/clusters][%d] listExternalClustersOK  %+v", 200, o.Payload)
}

func (o *ListExternalClustersOK) GetPayload() models.ClusterList {
	return o.Payload
}

func (o *ListExternalClustersOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// response payload
	if err := consumer.Consume(response.Body(), &o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewListExternalClustersUnauthorized creates a ListExternalClustersUnauthorized with default headers values
func NewListExternalClustersUnauthorized() *ListExternalClustersUnauthorized {
	return &ListExternalClustersUnauthorized{}
}

/*ListExternalClustersUnauthorized handles this case with default header values.

EmptyResponse is a empty response
*/
type ListExternalClustersUnauthorized struct {
}

func (o *ListExternalClustersUnauthorized) Error() string {
	return fmt.Sprintf("[GET /api/v2/projects/{project_id}/kubernetes/clusters][%d] listExternalClustersUnauthorized ", 401)
}

func (o *ListExternalClustersUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewListExternalClustersForbidden creates a ListExternalClustersForbidden with default headers values
func NewListExternalClustersForbidden() *ListExternalClustersForbidden {
	return &ListExternalClustersForbidden{}
}

/*ListExternalClustersForbidden handles this case with default header values.

EmptyResponse is a empty response
*/
type ListExternalClustersForbidden struct {
}

func (o *ListExternalClustersForbidden) Error() string {
	return fmt.Sprintf("[GET /api/v2/projects/{project_id}/kubernetes/clusters][%d] listExternalClustersForbidden ", 403)
}

func (o *ListExternalClustersForbidden) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewListExternalClustersDefault creates a ListExternalClustersDefault with default headers values
func NewListExternalClustersDefault(code int) *ListExternalClustersDefault {
	return &ListExternalClustersDefault{
		_statusCode: code,
	}
}

/*ListExternalClustersDefault handles this case with default header values.

errorResponse
*/
type ListExternalClustersDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// Code gets the status code for the list external clusters default response
func (o *ListExternalClustersDefault) Code() int {
	return o._statusCode
}

func (o *ListExternalClustersDefault) Error() string {
	return fmt.Sprintf("[GET /api/v2/projects/{project_id}/kubernetes/clusters][%d] listExternalClusters default  %+v", o._statusCode, o.Payload)
}

func (o *ListExternalClustersDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *ListExternalClustersDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
