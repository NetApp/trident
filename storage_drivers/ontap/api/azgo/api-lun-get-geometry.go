package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunGetGeometryRequest is a structure to represent a lun-get-geometry Request ZAPI object
type LunGetGeometryRequest struct {
	XMLName xml.Name `xml:"lun-get-geometry"`
	PathPtr *string  `xml:"path"`
}

// LunGetGeometryResponse is a structure to represent a lun-get-geometry Response ZAPI object
type LunGetGeometryResponse struct {
	XMLName         xml.Name                     `xml:"netapp"`
	ResponseVersion string                       `xml:"version,attr"`
	ResponseXmlns   string                       `xml:"xmlns,attr"`
	Result          LunGetGeometryResponseResult `xml:"results"`
}

// NewLunGetGeometryResponse is a factory method for creating new instances of LunGetGeometryResponse objects
func NewLunGetGeometryResponse() *LunGetGeometryResponse {
	return &LunGetGeometryResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetGeometryResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunGetGeometryResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunGetGeometryResponseResult is a structure to represent a lun-get-geometry Response Result ZAPI object
type LunGetGeometryResponseResult struct {
	XMLName              xml.Name `xml:"results"`
	ResultStatusAttr     string   `xml:"status,attr"`
	ResultReasonAttr     string   `xml:"reason,attr"`
	ResultErrnoAttr      string   `xml:"errno,attr"`
	BytesPerSectorPtr    *int     `xml:"bytes-per-sector"`
	CylindersPtr         *int     `xml:"cylinders"`
	MaxResizeSizePtr     *int     `xml:"max-resize-size"`
	SectorsPerTrackPtr   *int     `xml:"sectors-per-track"`
	SizePtr              *int     `xml:"size"`
	TracksPerCylinderPtr *int     `xml:"tracks-per-cylinder"`
}

// NewLunGetGeometryRequest is a factory method for creating new instances of LunGetGeometryRequest objects
func NewLunGetGeometryRequest() *LunGetGeometryRequest {
	return &LunGetGeometryRequest{}
}

// NewLunGetGeometryResponseResult is a factory method for creating new instances of LunGetGeometryResponseResult objects
func NewLunGetGeometryResponseResult() *LunGetGeometryResponseResult {
	return &LunGetGeometryResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunGetGeometryRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunGetGeometryResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetGeometryRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetGeometryResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetGeometryRequest) ExecuteUsing(zr *ZapiRunner) (*LunGetGeometryResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetGeometryRequest) executeWithoutIteration(zr *ZapiRunner) (*LunGetGeometryResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunGetGeometryRequest", NewLunGetGeometryResponse())
	return result.(*LunGetGeometryResponse), err
}

// Path is a 'getter' method
func (o *LunGetGeometryRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryRequest) SetPath(newValue string) *LunGetGeometryRequest {
	o.PathPtr = &newValue
	return o
}

// BytesPerSector is a 'getter' method
func (o *LunGetGeometryResponseResult) BytesPerSector() int {
	r := *o.BytesPerSectorPtr
	return r
}

// SetBytesPerSector is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetBytesPerSector(newValue int) *LunGetGeometryResponseResult {
	o.BytesPerSectorPtr = &newValue
	return o
}

// Cylinders is a 'getter' method
func (o *LunGetGeometryResponseResult) Cylinders() int {
	r := *o.CylindersPtr
	return r
}

// SetCylinders is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetCylinders(newValue int) *LunGetGeometryResponseResult {
	o.CylindersPtr = &newValue
	return o
}

// MaxResizeSize is a 'getter' method
func (o *LunGetGeometryResponseResult) MaxResizeSize() int {
	r := *o.MaxResizeSizePtr
	return r
}

// SetMaxResizeSize is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetMaxResizeSize(newValue int) *LunGetGeometryResponseResult {
	o.MaxResizeSizePtr = &newValue
	return o
}

// SectorsPerTrack is a 'getter' method
func (o *LunGetGeometryResponseResult) SectorsPerTrack() int {
	r := *o.SectorsPerTrackPtr
	return r
}

// SetSectorsPerTrack is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetSectorsPerTrack(newValue int) *LunGetGeometryResponseResult {
	o.SectorsPerTrackPtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *LunGetGeometryResponseResult) Size() int {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetSize(newValue int) *LunGetGeometryResponseResult {
	o.SizePtr = &newValue
	return o
}

// TracksPerCylinder is a 'getter' method
func (o *LunGetGeometryResponseResult) TracksPerCylinder() int {
	r := *o.TracksPerCylinderPtr
	return r
}

// SetTracksPerCylinder is a fluent style 'setter' method that can be chained
func (o *LunGetGeometryResponseResult) SetTracksPerCylinder(newValue int) *LunGetGeometryResponseResult {
	o.TracksPerCylinderPtr = &newValue
	return o
}
