// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// JobScheduleInfoType is a structure to represent a job-schedule-info ZAPI object
type JobScheduleInfoType struct {
	XMLName                   xml.Name          `xml:"job-schedule-info"`
	JobScheduleClusterPtr     *ClusterNameType  `xml:"job-schedule-cluster"`
	JobScheduleDescriptionPtr *string           `xml:"job-schedule-description"`
	JobScheduleNamePtr        *string           `xml:"job-schedule-name"`
	JobScheduleTypePtr        *ScheduleTypeType `xml:"job-schedule-type"`
}

// NewJobScheduleInfoType is a factory method for creating new instances of JobScheduleInfoType objects
func NewJobScheduleInfoType() *JobScheduleInfoType {
	return &JobScheduleInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *JobScheduleInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobScheduleInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// JobScheduleCluster is a 'getter' method
func (o *JobScheduleInfoType) JobScheduleCluster() ClusterNameType {
	var r ClusterNameType
	if o.JobScheduleClusterPtr == nil {
		return r
	}
	r = *o.JobScheduleClusterPtr
	return r
}

// SetJobScheduleCluster is a fluent style 'setter' method that can be chained
func (o *JobScheduleInfoType) SetJobScheduleCluster(newValue ClusterNameType) *JobScheduleInfoType {
	o.JobScheduleClusterPtr = &newValue
	return o
}

// JobScheduleDescription is a 'getter' method
func (o *JobScheduleInfoType) JobScheduleDescription() string {
	var r string
	if o.JobScheduleDescriptionPtr == nil {
		return r
	}
	r = *o.JobScheduleDescriptionPtr
	return r
}

// SetJobScheduleDescription is a fluent style 'setter' method that can be chained
func (o *JobScheduleInfoType) SetJobScheduleDescription(newValue string) *JobScheduleInfoType {
	o.JobScheduleDescriptionPtr = &newValue
	return o
}

// JobScheduleName is a 'getter' method
func (o *JobScheduleInfoType) JobScheduleName() string {
	var r string
	if o.JobScheduleNamePtr == nil {
		return r
	}
	r = *o.JobScheduleNamePtr
	return r
}

// SetJobScheduleName is a fluent style 'setter' method that can be chained
func (o *JobScheduleInfoType) SetJobScheduleName(newValue string) *JobScheduleInfoType {
	o.JobScheduleNamePtr = &newValue
	return o
}

// JobScheduleType is a 'getter' method
func (o *JobScheduleInfoType) JobScheduleType() ScheduleTypeType {
	var r ScheduleTypeType
	if o.JobScheduleTypePtr == nil {
		return r
	}
	r = *o.JobScheduleTypePtr
	return r
}

// SetJobScheduleType is a fluent style 'setter' method that can be chained
func (o *JobScheduleInfoType) SetJobScheduleType(newValue ScheduleTypeType) *JobScheduleInfoType {
	o.JobScheduleTypePtr = &newValue
	return o
}
