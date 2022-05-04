/*
Copyright 2021 NetApp Inc..

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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Images defines the images section of AstraDSDeployment Spec
type Images struct {
	DMSController      string `json:"dmsController"`
	FiretapInstaller   string `json:"firetapInstaller"`
	FiretapUninstaller string `json:"firetapUninstaller"`
	Firegen            string `json:"firegen"`
	FiretapMetrics     string `json:"firetapMetrics"`
	ClusterController  string `json:"clusterController"`
	Support            string `json:"support"`
	LicenseController  string `json:"licenseController"`
	CallhomeListener   string `json:"callhomeListener"`
	NodeInfoController string `json:"nodeInfoController"`
	AutosupportCronjob string `json:"autosupportCronjob"`
}

// ValidatedVersions defines the desired ADS and firetap versions. The fields
// are validated to never be missing or "" (empty string)
type ValidatedVersions struct {
	// +kubebuilder:validation:MinLength:=1
	ADS string `json:"ads"`
	// +kubebuilder:validation:MinLength:=1
	Firetap string `json:"firetap"`
}

func (v ValidatedVersions) String() string {
	return fmt.Sprintf("ADS: %v, Firetap: %v", v.ADS, v.Firetap)
}

// EqualOrUpgrade checks if the versions passed in are equal and which version fields are being upgraded
func (v ValidatedVersions) EqualOrUpgrade(versions Versions) (equal, adsUpgrade, firetapUpgrade bool) {
	adsUpgrade = v.ADS != versions.ADS
	firetapUpgrade = v.Firetap != versions.Firetap
	equal = !adsUpgrade && !firetapUpgrade
	return
}

// Equal checks if the versions passed in are equal
func (v ValidatedVersions) Equal(versions Versions) bool {
	adsEqual := v.ADS == versions.ADS
	firetapEqual := v.Firetap == versions.Firetap
	return adsEqual && firetapEqual
}

// Versions defines the ADS and firetap versions
type Versions struct {
	ADS     string `json:"ads,omitempty"`
	Firetap string `json:"firetap,omitempty"`
}

func (v Versions) Empty() bool {
	return v.ADS == "" && v.Firetap == ""
}

func (v Versions) String() string {
	return fmt.Sprintf("ADS: %v, Firetap: %v", v.ADS, v.Firetap)
}

// AstraDSDeploymentSpec defines the desired state of AstraDSDeployment
type AstraDSDeploymentSpec struct {
	Images   Images            `json:"images,omitempty"`
	Versions ValidatedVersions `json:"versions"`
}

// AstraDSDeploymentStatus defines the observed state of AstraDSDeployment
type AstraDSDeploymentStatus struct {
	Versions Versions `json:"versions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=adsde,categories={ads,all}

// AstraDSDeployment is the Schema for the astradsdeployments API
type AstraDSDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSDeploymentSpec   `json:"spec,omitempty"`
	Status AstraDSDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSDeploymentList contains a list of AstraDSDeployment
type AstraDSDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSDeployment{}, &AstraDSDeploymentList{})
}
