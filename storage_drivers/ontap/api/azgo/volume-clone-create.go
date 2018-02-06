// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeCloneCreateRequest is a structure to represent a volume-clone-create ZAPI request object
type VolumeCloneCreateRequest struct {
	XMLName xml.Name `xml:"volume-clone-create"`

	CachingPolicyPtr         *string `xml:"caching-policy"`
	ParentSnapshotPtr        *string `xml:"parent-snapshot"`
	ParentVolumePtr          *string `xml:"parent-volume"`
	ParentVserverPtr         *string `xml:"parent-vserver"`
	QosPolicyGroupNamePtr    *string `xml:"qos-policy-group-name"`
	SpaceReservePtr          *string `xml:"space-reserve"`
	UseSnaprestoreLicensePtr *bool   `xml:"use-snaprestore-license"`
	VolumePtr                *string `xml:"volume"`
	VolumeTypePtr            *string `xml:"volume-type"`
	VserverPtr               *string `xml:"vserver"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeCloneCreateRequest is a factory method for creating new instances of VolumeCloneCreateRequest objects
func NewVolumeCloneCreateRequest() *VolumeCloneCreateRequest { return &VolumeCloneCreateRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeCloneCreateRequest) ExecuteUsing(zr *ZapiRunner) (VolumeCloneCreateResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeCloneCreateRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeCloneCreateResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeCloneCreateResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeCloneCreateResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeCloneCreateResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-clone-create result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateRequest) String() string {
	var buffer bytes.Buffer
	if o.CachingPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "caching-policy", *o.CachingPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("caching-policy: nil\n"))
	}
	if o.ParentSnapshotPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "parent-snapshot", *o.ParentSnapshotPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("parent-snapshot: nil\n"))
	}
	if o.ParentVolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "parent-volume", *o.ParentVolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("parent-volume: nil\n"))
	}
	if o.ParentVserverPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "parent-vserver", *o.ParentVserverPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("parent-vserver: nil\n"))
	}
	if o.QosPolicyGroupNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "qos-policy-group-name", *o.QosPolicyGroupNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("qos-policy-group-name: nil\n"))
	}
	if o.SpaceReservePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "space-reserve", *o.SpaceReservePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("space-reserve: nil\n"))
	}
	if o.UseSnaprestoreLicensePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "use-snaprestore-license", *o.UseSnaprestoreLicensePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("use-snaprestore-license: nil\n"))
	}
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	if o.VolumeTypePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-type", *o.VolumeTypePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-type: nil\n"))
	}
	if o.VserverPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "vserver", *o.VserverPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("vserver: nil\n"))
	}
	return buffer.String()
}

// CachingPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) CachingPolicy() string {
	r := *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetCachingPolicy(newValue string) *VolumeCloneCreateRequest {
	o.CachingPolicyPtr = &newValue
	return o
}

// ParentSnapshot is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) ParentSnapshot() string {
	r := *o.ParentSnapshotPtr
	return r
}

// SetParentSnapshot is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetParentSnapshot(newValue string) *VolumeCloneCreateRequest {
	o.ParentSnapshotPtr = &newValue
	return o
}

// ParentVolume is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) ParentVolume() string {
	r := *o.ParentVolumePtr
	return r
}

// SetParentVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetParentVolume(newValue string) *VolumeCloneCreateRequest {
	o.ParentVolumePtr = &newValue
	return o
}

// ParentVserver is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) ParentVserver() string {
	r := *o.ParentVserverPtr
	return r
}

// SetParentVserver is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetParentVserver(newValue string) *VolumeCloneCreateRequest {
	o.ParentVserverPtr = &newValue
	return o
}

// QosPolicyGroupName is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) QosPolicyGroupName() string {
	r := *o.QosPolicyGroupNamePtr
	return r
}

// SetQosPolicyGroupName is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetQosPolicyGroupName(newValue string) *VolumeCloneCreateRequest {
	o.QosPolicyGroupNamePtr = &newValue
	return o
}

// SpaceReserve is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) SpaceReserve() string {
	r := *o.SpaceReservePtr
	return r
}

// SetSpaceReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetSpaceReserve(newValue string) *VolumeCloneCreateRequest {
	o.SpaceReservePtr = &newValue
	return o
}

// UseSnaprestoreLicense is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) UseSnaprestoreLicense() bool {
	r := *o.UseSnaprestoreLicensePtr
	return r
}

// SetUseSnaprestoreLicense is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetUseSnaprestoreLicense(newValue bool) *VolumeCloneCreateRequest {
	o.UseSnaprestoreLicensePtr = &newValue
	return o
}

// Volume is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetVolume(newValue string) *VolumeCloneCreateRequest {
	o.VolumePtr = &newValue
	return o
}

// VolumeType is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) VolumeType() string {
	r := *o.VolumeTypePtr
	return r
}

// SetVolumeType is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetVolumeType(newValue string) *VolumeCloneCreateRequest {
	o.VolumeTypePtr = &newValue
	return o
}

// Vserver is a fluent style 'getter' method that can be chained
func (o *VolumeCloneCreateRequest) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetVserver(newValue string) *VolumeCloneCreateRequest {
	o.VserverPtr = &newValue
	return o
}

// VolumeCloneCreateResponse is a structure to represent a volume-clone-create ZAPI response object
type VolumeCloneCreateResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeCloneCreateResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeCloneCreateResponseResult is a structure to represent a volume-clone-create ZAPI object's result
type VolumeCloneCreateResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeCloneCreateResponse is a factory method for creating new instances of VolumeCloneCreateResponse objects
func NewVolumeCloneCreateResponse() *VolumeCloneCreateResponse { return &VolumeCloneCreateResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
