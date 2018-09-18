// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// LunGetGeometryRequest is a structure to represent a lun-get-geometry ZAPI request object
type LunGetGeometryRequest struct {
	XMLName xml.Name `xml:"lun-get-geometry"`

	PathPtr *string `xml:"path"`
}

// ToXML converts this object into an xml string representation
func (o *LunGetGeometryRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunGetGeometryRequest is a factory method for creating new instances of LunGetGeometryRequest objects
func NewLunGetGeometryRequest() *LunGetGeometryRequest { return &LunGetGeometryRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetGeometryRequest) ExecuteUsing(zr *ZapiRunner) (LunGetGeometryResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunGetGeometryRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunGetGeometryResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunGetGeometryResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunGetGeometryResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunGetGeometryResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-get-geometry result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetGeometryRequest) String() string {
	var buffer bytes.Buffer
	if o.PathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "path", *o.PathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("path: nil\n"))
	}
	return buffer.String()
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunGetGeometryRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryRequest) SetPath(newValue string) *LunGetGeometryRequest {
	o.PathPtr = &newValue
	return o
}

// LunGetGeometryResponse is a structure to represent a lun-get-geometry ZAPI response object
type LunGetGeometryResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunGetGeometryResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetGeometryResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunGetGeometryResponseResult is a structure to represent a lun-get-geometry ZAPI object's result
type LunGetGeometryResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr     string `xml:"status,attr"`
	ResultReasonAttr     string `xml:"reason,attr"`
	ResultErrnoAttr      string `xml:"errno,attr"`
	BytesPerSectorPtr    *int   `xml:"bytes-per-sector"`
	CylindersPtr         *int   `xml:"cylinders"`
	MaxResizeSizePtr     *int   `xml:"max-resize-size"`
	SectorsPerTrackPtr   *int   `xml:"sectors-per-track"`
	SizePtr              *int   `xml:"size"`
	TracksPerCylinderPtr *int   `xml:"tracks-per-cylinder"`
}

// ToXML converts this object into an xml string representation
func (o *LunGetGeometryResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunGetGeometryResponse is a factory method for creating new instances of LunGetGeometryResponse objects
func NewLunGetGeometryResponse() *LunGetGeometryResponse { return &LunGetGeometryResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetGeometryResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.BytesPerSectorPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "bytes-per-sector", *o.BytesPerSectorPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("bytes-per-sector: nil\n"))
	}
	if o.CylindersPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "cylinders", *o.CylindersPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("cylinders: nil\n"))
	}
	if o.MaxResizeSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-resize-size", *o.MaxResizeSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-resize-size: nil\n"))
	}
	if o.SectorsPerTrackPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "sectors-per-track", *o.SectorsPerTrackPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("sectors-per-track: nil\n"))
	}
	if o.SizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "size", *o.SizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("size: nil\n"))
	}
	if o.TracksPerCylinderPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "tracks-per-cylinder", *o.TracksPerCylinderPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("tracks-per-cylinder: nil\n"))
	}
	return buffer.String()
}

// BytesPerSector is a fluent style 'getter' method that can be chained
func (o *LunGetGeometryResponseResult) BytesPerSector() int {
	r := *o.BytesPerSectorPtr
	return r
}

// SetBytesPerSector is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetBytesPerSector(newValue int) *LunGetGeometryResponseResult {
	o.BytesPerSectorPtr = &newValue
	return o
}

// Cylinders is a fluent style 'getter' method that can be chained
func (o *LunGetGeometryResponseResult) Cylinders() int {
	r := *o.CylindersPtr
	return r
}

// SetCylinders is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetCylinders(newValue int) *LunGetGeometryResponseResult {
	o.CylindersPtr = &newValue
	return o
}

// MaxResizeSize is a fluent style 'getter' method that can be chained
func (o *LunGetGeometryResponseResult) MaxResizeSize() int {
	r := *o.MaxResizeSizePtr
	return r
}

// SetMaxResizeSize is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetMaxResizeSize(newValue int) *LunGetGeometryResponseResult {
	o.MaxResizeSizePtr = &newValue
	return o
}

// SectorsPerTrack is a fluent style 'getter' method that can be chained
func (o *LunGetGeometryResponseResult) SectorsPerTrack() int {
	r := *o.SectorsPerTrackPtr
	return r
}

// SetSectorsPerTrack is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetSectorsPerTrack(newValue int) *LunGetGeometryResponseResult {
	o.SectorsPerTrackPtr = &newValue
	return o
}

// Size is a fluent style 'getter' method that can be chained
func (o *LunGetGeometryResponseResult) Size() int {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetSize(newValue int) *LunGetGeometryResponseResult {
	o.SizePtr = &newValue
	return o
}

// TracksPerCylinder is a fluent style 'getter' method that can be chained
func (o *LunGetGeometryResponseResult) TracksPerCylinder() int {
	r := *o.TracksPerCylinderPtr
	return r
}

// SetTracksPerCylinder is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetTracksPerCylinder(newValue int) *LunGetGeometryResponseResult {
	o.TracksPerCylinderPtr = &newValue
	return o
}
