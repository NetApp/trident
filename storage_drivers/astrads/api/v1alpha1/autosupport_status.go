/*
Copyright 2021 NetApp Inc.
All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"
	"os"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetState sets the AutoSupport bundle collection state in the AutoSupport object
func (asup *AutoSupportStatus) SetState(t AutoSupportStateType) {
	asup.State = t
}

// GetState retrieves the AutoSupport bundle collection state from the AutoSupport object
func (asup *AutoSupportStatus) GetState() AutoSupportStateType {
	return asup.State
}

// SetLocalPath sets the location of the support bundle(from the view point of controller) in the AutoSupport object
func (asup *AutoSupportStatus) SetLocalPath(p string) {
	asup.LocalPath = fmt.Sprintf("%s:%s", os.Getenv("POD_NAME"), p)
}

// GetLocalPath retrieves the LocalPath value from the AutoSupport object
func (asup *AutoSupportStatus) GetLocalPath() string {
	// When set, status.LocalPath is of the format POD_NAME:path
	// Remove the POD_NAME and return only the path
	pathSlice := strings.Split(asup.LocalPath, ":")
	if len(pathSlice) == 2 {
		return pathSlice[len(pathSlice)-1]
	}
	return asup.LocalPath
}

// ResetLocalPath sets the localpath to ""
func (asup *AutoSupportStatus) ResetLocalPath() {
	asup.SetLocalPath("")
}

// SetBundleSize sets the size of the AutoSupport bundle in the AutoSupport object
func (asup *AutoSupportStatus) SetBundleSize(s int64) {
	asup.Size = *resource.NewQuantity(s, resource.BinarySI)
}

// GetBundleSize retrieves the size of the AutoSupport bundle in the AutoSupport object
func (asup *AutoSupportStatus) GetBundleSize() int64 {
	return asup.Size.Value()
}

// GetBundleSize retrieves the size of the AutoSupport bundle in the AutoSupport object
func (asup *AutoSupportStatus) ResetBundleSize() {
	asup.SetBundleSize(0)
}

// SetSequenceNumber sets the sequenceNumber of the AutoSupport bundle in the AutoSupport object
func (asup *AutoSupportStatus) SetSequenceNumber(s int64) {
	asup.SequenceNumber = s
}

// GetSequenceNumber retrieves the sequenceNumber of the AutoSupport bundle in the AutoSupport object
func (asup *AutoSupportStatus) GetSequenceNumber() int64 {
	return asup.SequenceNumber
}

// SetInitializingCondition sets the AutoSupportInitializing condition in the status of the AutoSupport object
func (asup *AutoSupportStatus) SetInitializingCondition() {
	c := newAutoSupportCondition(
		AutoSupportInitializing,
		v1.ConditionTrue,
		"AutoSupport bundle Initialized",
		"AutoSupport bundle Initialized, Sequence number set")
	asup.setAutoSupportCondition(*c)
}

// SetCollectingCondition sets the AutoSupportCollecting condition in the status of the AutoSupport object
func (asup *AutoSupportStatus) SetCollectingCondition(path string) {
	c := newAutoSupportCondition(
		AutoSupportCollecting,
		v1.ConditionTrue,
		"Collecting AutoSupport bundle",
		fmt.Sprintf("Collecting AutoSupport bundle in %s", path))
	asup.setAutoSupportCondition(*c)
}

// SetCollectedCondition sets the AutoSupportCollected condition in the status of the AutoSupport object
func (asup *AutoSupportStatus) SetCollectedCondition(path string) {
	c := newAutoSupportCondition(
		AutoSupportCollected,
		v1.ConditionTrue,
		"AutoSupport bundle collected",
		fmt.Sprintf("AutoSupport bundle collected in %s", path))
	asup.setAutoSupportCondition(*c)
}

// SetCompressedCondition sets the AutoSupportCompressed condition in the status of the AutoSupport object
func (asup *AutoSupportStatus) SetCompressedCondition(path string) {
	c := newAutoSupportCondition(
		AutoSupportCompressed,
		v1.ConditionTrue,
		"AutoSupport bundle compressed",
		fmt.Sprintf("AutoSupport bundle compressed in %s", path))
	asup.setAutoSupportCondition(*c)
}

// SetUploadReadyCondition sets the AutoSupportUploadReady condition in the status of the AutoSupport object
func (asup *AutoSupportStatus) SetUploadReadyCondition(path string) {
	c := newAutoSupportCondition(
		AutoSupportUploadReady,
		v1.ConditionTrue,
		"AutoSupport bundle ready for upload",
		fmt.Sprintf("Waiting to transfer collected AutoSupport bundle %s", path))
	asup.setAutoSupportCondition(*c)
}

// SetUploadingCondition sets the AutoSupportUploadReady condition in the status of the AutoSupport object
func (asup *AutoSupportStatus) SetUploadingCondition(path string) {
	c := newAutoSupportCondition(
		AutoSupportUploading,
		v1.ConditionTrue,
		"AutoSupport bundle uploading",
		fmt.Sprintf("Transferring the AutoSupport bundle from %s to NetApp", path))
	asup.setAutoSupportCondition(*c)
}

// SetUploadedCondition sets the AutoSupportUploaded condition in the status of the AutoSupport object
func (asup *AutoSupportStatus) SetUploadedCondition() {
	c := newAutoSupportCondition(
		AutoSupportUploaded,
		v1.ConditionTrue,
		"AutoSupport bundle uploaded",
		"Transfer of AutoSupport bundle to NetApp complete")
	asup.setAutoSupportCondition(*c)
}

// SetErrorCondition sets the AutoSupportError condition in the status of the AutoSupport object with the error string specified
func (asup *AutoSupportStatus) SetErrorCondition(err string) {
	c := newAutoSupportCondition(AutoSupportError,
		v1.ConditionTrue,
		"AutoSupport bundle error",
		fmt.Sprintf("AutoSupport bundle collection error: %s", err))
	asup.setAutoSupportCondition(*c)
}

// UpdateConditionTransitionTime sets the LastTransitionTime in the condition if the condition is set
func (asup *AutoSupportStatus) UpdateConditionTransitionTime(t AutoSupportStateType) {
	pos, c := getAutoSupportCondition(asup, t)
	if pos == -1 {
		return
	}
	c.LastTransitionTime = metav1.NewTime(time.Now())
	asup.Conditions[pos] = *c
}

// UpdateHeartBeatTime updates the heartbeat of the given autosupport state
func (asup *AutoSupportStatus) UpdateHeartBeatTime(t AutoSupportStateType) {
	pos, c := getAutoSupportCondition(asup, t)
	if pos == -1 {
		return
	}
	c.LastHeartbeatTime = metav1.NewTime(time.Now())
	asup.Conditions[pos] = *c
}

// ClearCondition clears the given condition from the condition slice in the status of the AutoSupport object
func (asup *AutoSupportStatus) ClearCondition(t AutoSupportStateType) {
	pos, _ := getAutoSupportCondition(asup, t)
	if pos == -1 {
		return
	}
	asup.Conditions = append(asup.Conditions[:pos], asup.Conditions[pos+1:]...)
}

// LastValidState the last valid state of the AutoSupport CR by reading the condition slice
func (asup *AutoSupportStatus) LastValidState() AutoSupportStateType {
	var s AutoSupportStateType
	for i := len(asup.Conditions) - 1; i >= 0; i-- {
		c := asup.Conditions[i]
		if AutoSupportError != c.Type {
			s = c.Type
			break
		}
	}
	return s
}

// LastHeartBeatTime
func (asup *AutoSupportStatus) LastHeartBeat() metav1.Time {
	lastCondition := asup.Conditions[len(asup.Conditions)-1]

	return lastCondition.LastHeartbeatTime
}

// Last Transition Time
func (asup *AutoSupportStatus) LastTransitionTime() metav1.Time {
	lastCondition := asup.Conditions[len(asup.Conditions)-1]

	return lastCondition.LastTransitionTime
}

// setAutoSupportCondition sets the condition in the Status.Conditions array for AutoSupport
func (asup *AutoSupportStatus) setAutoSupportCondition(c AutoSupportCondition) {
	pos, cp := getAutoSupportCondition(asup, c.Type)
	if cp != nil &&
		cp.Status == c.Status && cp.Reason == c.Reason && cp.Message == c.Message {
		return
	}

	if cp != nil {
		asup.Conditions[pos] = c
	} else {
		asup.Conditions = append(asup.Conditions, c)
	}
}

// getAutoSupportCondition retrieves the condition from Status.Conditions array for AutoSupport
func getAutoSupportCondition(status *AutoSupportStatus, t AutoSupportStateType) (int, *AutoSupportCondition) {
	for i, c := range status.Conditions {
		if t == c.Type {
			return i, &c
		}
	}
	return -1, nil
}

// newAutoSupportCondition creates a new struct of type Status.Conditions
func newAutoSupportCondition(condType AutoSupportStateType, status v1.ConditionStatus, reason, message string) *AutoSupportCondition {
	now := metav1.NewTime(time.Now())
	return &AutoSupportCondition{
		Type:               condType,
		Status:             status,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}
