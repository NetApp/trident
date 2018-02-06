// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// QtreeRenameRequest is a structure to represent a qtree-rename ZAPI request object
type QtreeRenameRequest struct {
	XMLName xml.Name `xml:"qtree-rename"`

	NewQtreeNamePtr *string `xml:"new-qtree-name"`
	QtreePtr        *string `xml:"qtree"`
}

// ToXML converts this object into an xml string representation
func (o *QtreeRenameRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewQtreeRenameRequest is a factory method for creating new instances of QtreeRenameRequest objects
func NewQtreeRenameRequest() *QtreeRenameRequest { return &QtreeRenameRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QtreeRenameRequest) ExecuteUsing(zr *ZapiRunner) (QtreeRenameResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "QtreeRenameRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return QtreeRenameResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return QtreeRenameResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n QtreeRenameResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return QtreeRenameResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("qtree-rename result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeRenameRequest) String() string {
	var buffer bytes.Buffer
	if o.NewQtreeNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "new-qtree-name", *o.NewQtreeNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("new-qtree-name: nil\n"))
	}
	if o.QtreePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "qtree", *o.QtreePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("qtree: nil\n"))
	}
	return buffer.String()
}

// NewQtreeName is a fluent style 'getter' method that can be chained
func (o *QtreeRenameRequest) NewQtreeName() string {
	r := *o.NewQtreeNamePtr
	return r
}

// SetNewQtreeName is a fluent style 'setter' method that can be chained
func (o *QtreeRenameRequest) SetNewQtreeName(newValue string) *QtreeRenameRequest {
	o.NewQtreeNamePtr = &newValue
	return o
}

// Qtree is a fluent style 'getter' method that can be chained
func (o *QtreeRenameRequest) Qtree() string {
	r := *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QtreeRenameRequest) SetQtree(newValue string) *QtreeRenameRequest {
	o.QtreePtr = &newValue
	return o
}

// QtreeRenameResponse is a structure to represent a qtree-rename ZAPI response object
type QtreeRenameResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result QtreeRenameResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeRenameResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// QtreeRenameResponseResult is a structure to represent a qtree-rename ZAPI object's result
type QtreeRenameResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *QtreeRenameResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewQtreeRenameResponse is a factory method for creating new instances of QtreeRenameResponse objects
func NewQtreeRenameResponse() *QtreeRenameResponse { return &QtreeRenameResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeRenameResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
