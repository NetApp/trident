package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// JobGetIterRequest is a structure to represent a job-get-iter Request ZAPI object
type JobGetIterRequest struct {
	XMLName              xml.Name                            `xml:"job-get-iter"`
	DesiredAttributesPtr *JobGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                `xml:"max-records"`
	QueryPtr             *JobGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                             `xml:"tag"`
}

// JobGetIterResponse is a structure to represent a job-get-iter Response ZAPI object
type JobGetIterResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          JobGetIterResponseResult `xml:"results"`
}

// NewJobGetIterResponse is a factory method for creating new instances of JobGetIterResponse objects
func NewJobGetIterResponse() *JobGetIterResponse {
	return &JobGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *JobGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// JobGetIterResponseResult is a structure to represent a job-get-iter Response Result ZAPI object
type JobGetIterResponseResult struct {
	XMLName           xml.Name                                `xml:"results"`
	ResultStatusAttr  string                                  `xml:"status,attr"`
	ResultReasonAttr  string                                  `xml:"reason,attr"`
	ResultErrnoAttr   string                                  `xml:"errno,attr"`
	AttributesListPtr *JobGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                 `xml:"next-tag"`
	NumRecordsPtr     *int                                    `xml:"num-records"`
}

// NewJobGetIterRequest is a factory method for creating new instances of JobGetIterRequest objects
func NewJobGetIterRequest() *JobGetIterRequest {
	return &JobGetIterRequest{}
}

// NewJobGetIterResponseResult is a factory method for creating new instances of JobGetIterResponseResult objects
func NewJobGetIterResponseResult() *JobGetIterResponseResult {
	return &JobGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *JobGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *JobGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *JobGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*JobGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *JobGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*JobGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "JobGetIterRequest", NewJobGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*JobGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *JobGetIterRequest) executeWithIteration(zr *ZapiRunner) (*JobGetIterResponse, error) {
	combined := NewJobGetIterResponse()
	combined.Result.SetAttributesList(JobGetIterResponseResultAttributesList{})
	var nextTagPtr *string
	done := false
	for done != true {
		n, err := o.executeWithoutIteration(zr)

		if err != nil {
			return nil, err
		}
		nextTagPtr = n.Result.NextTagPtr
		if nextTagPtr == nil {
			done = true
		} else {
			o.SetTag(*nextTagPtr)
		}

		if n.Result.NumRecordsPtr == nil {
			done = true
		} else {
			recordsRead := n.Result.NumRecords()
			if recordsRead == 0 {
				done = true
			}
		}

		if n.Result.AttributesListPtr != nil {
			if combined.Result.AttributesListPtr == nil {
				combined.Result.SetAttributesList(JobGetIterResponseResultAttributesList{})
			}
			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()

			resultAttributesList := n.Result.AttributesList()
			resultAttributes := resultAttributesList.values()

			combined.Result.AttributesListPtr.setValues(append(combinedAttributes, resultAttributes...))
		}

		if done == true {

			combined.Result.ResultErrnoAttr = n.Result.ResultErrnoAttr
			combined.Result.ResultReasonAttr = n.Result.ResultReasonAttr
			combined.Result.ResultStatusAttr = n.Result.ResultStatusAttr

			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()
			combined.Result.SetNumRecords(len(combinedAttributes))

		}
	}
	return combined, nil
}

// JobGetIterRequestDesiredAttributes is a wrapper
type JobGetIterRequestDesiredAttributes struct {
	XMLName    xml.Name     `xml:"desired-attributes"`
	JobInfoPtr *JobInfoType `xml:"job-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// JobInfo is a 'getter' method
func (o *JobGetIterRequestDesiredAttributes) JobInfo() JobInfoType {
	r := *o.JobInfoPtr
	return r
}

// SetJobInfo is a fluent style 'setter' method that can be chained
func (o *JobGetIterRequestDesiredAttributes) SetJobInfo(newValue JobInfoType) *JobGetIterRequestDesiredAttributes {
	o.JobInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *JobGetIterRequest) DesiredAttributes() JobGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *JobGetIterRequest) SetDesiredAttributes(newValue JobGetIterRequestDesiredAttributes) *JobGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *JobGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *JobGetIterRequest) SetMaxRecords(newValue int) *JobGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// JobGetIterRequestQuery is a wrapper
type JobGetIterRequestQuery struct {
	XMLName    xml.Name     `xml:"query"`
	JobInfoPtr *JobInfoType `xml:"job-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// JobInfo is a 'getter' method
func (o *JobGetIterRequestQuery) JobInfo() JobInfoType {
	r := *o.JobInfoPtr
	return r
}

// SetJobInfo is a fluent style 'setter' method that can be chained
func (o *JobGetIterRequestQuery) SetJobInfo(newValue JobInfoType) *JobGetIterRequestQuery {
	o.JobInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *JobGetIterRequest) Query() JobGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *JobGetIterRequest) SetQuery(newValue JobGetIterRequestQuery) *JobGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *JobGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *JobGetIterRequest) SetTag(newValue string) *JobGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// JobGetIterResponseResultAttributesList is a wrapper
type JobGetIterResponseResultAttributesList struct {
	XMLName    xml.Name      `xml:"attributes-list"`
	JobInfoPtr []JobInfoType `xml:"job-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// JobInfo is a 'getter' method
func (o *JobGetIterResponseResultAttributesList) JobInfo() []JobInfoType {
	r := o.JobInfoPtr
	return r
}

// SetJobInfo is a fluent style 'setter' method that can be chained
func (o *JobGetIterResponseResultAttributesList) SetJobInfo(newValue []JobInfoType) *JobGetIterResponseResultAttributesList {
	newSlice := make([]JobInfoType, len(newValue))
	copy(newSlice, newValue)
	o.JobInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *JobGetIterResponseResultAttributesList) values() []JobInfoType {
	r := o.JobInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *JobGetIterResponseResultAttributesList) setValues(newValue []JobInfoType) *JobGetIterResponseResultAttributesList {
	newSlice := make([]JobInfoType, len(newValue))
	copy(newSlice, newValue)
	o.JobInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *JobGetIterResponseResult) AttributesList() JobGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *JobGetIterResponseResult) SetAttributesList(newValue JobGetIterResponseResultAttributesList) *JobGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *JobGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *JobGetIterResponseResult) SetNextTag(newValue string) *JobGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *JobGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *JobGetIterResponseResult) SetNumRecords(newValue int) *JobGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
