// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// VSphereDatacenterSpec VSphereDatacenterSpec specifies a datacenter of VSphere.
//
// swagger:model VSphereDatacenterSpec
type VSphereDatacenterSpec struct {

	// cluster
	Cluster string `json:"cluster,omitempty"`

	// datacenter
	Datacenter string `json:"datacenter,omitempty"`

	// datastore
	Datastore string `json:"datastore,omitempty"`

	// datastore cluster
	DatastoreCluster string `json:"datastoreCluster,omitempty"`

	// endpoint
	Endpoint string `json:"endpoint,omitempty"`

	// templates
	Templates ImageList `json:"templates,omitempty"`
}

// Validate validates this v sphere datacenter spec
func (m *VSphereDatacenterSpec) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateTemplates(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *VSphereDatacenterSpec) validateTemplates(formats strfmt.Registry) error {

	if swag.IsZero(m.Templates) { // not required
		return nil
	}

	if err := m.Templates.Validate(formats); err != nil {
		if ve, ok := err.(*errors.Validation); ok {
			return ve.ValidateName("templates")
		}
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *VSphereDatacenterSpec) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *VSphereDatacenterSpec) UnmarshalBinary(b []byte) error {
	var res VSphereDatacenterSpec
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
