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

package v1beta1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Images defines the images section of AstraDSVersion Spec
type Images struct {
	// DMSController is the repository path to the DMS controller image
	DMSController string `json:"dmsController"`
	// FiretapInstaller is the repository path to the Firetap installer image
	FiretapInstaller string `json:"firetapInstaller"`
	// FiretapUninstaller is the repository path to the Firetap uninstaller image
	FiretapUninstaller string `json:"firetapUninstaller"`
	// Firegen is the repository path to the Firegen image
	Firegen string `json:"firegen"`
	// FiretapMetrics is the repository path to the Firetap metrics image
	FiretapMetrics string `json:"firetapMetrics"`
	// ClusterController is the repository path to the cluster controller image
	ClusterController string `json:"clusterController"`
	// Support is the repository path to the support controller image
	Support string `json:"support"`
	// LicenseController is the repository path to the license controller image
	LicenseController string `json:"licenseController"`
	// CallhomeListener is the repository path to the callhome listener image
	CallhomeListener string `json:"callhomeListener"`
	// NodeInfoController is the repository path to the node info controller image
	NodeInfoController string `json:"nodeInfoController"`
	// AutosupportCronjob is the repository path to the autosupport cronjob image
	AutosupportCronjob string `json:"autosupportCronjob"`
}

// ValidatedVersions defines the desired ADS and firetap versions. The fields
// are validated to never be missing or "" (empty string)
type ValidatedVersions struct {
	// +kubebuilder:validation:MinLength:=1
	// ADS is the current version of the ADS system
	ADS string `json:"ads"`
	// +kubebuilder:validation:MinLength:=1
	// Firetap is the current version of the Firetap system
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

// AdsEqual checks if versions passed in have equal ADS version
func (v ValidatedVersions) AdsEqual(versions Versions) bool {
	return v.ADS == versions.ADS
}

// FiretapEqual checks if versions passes in have equal Firetap version
func (v ValidatedVersions) FiretapEqual(versions Versions) bool {
	return v.Firetap == versions.Firetap
}

// Versions defines ADS and firetap versions
type Versions struct {
	// ADS is the current version of the ADS system
	ADS string `json:"ads,omitempty"`
	// Firetap is the current version of the Firetap system
	Firetap string `json:"firetap,omitempty"`
}

// ADSCertificateUpdateStatus defines the ongoing certificate update status
type ADSCertificateUpdateStatus struct {
	// CompletedClusterCount is the number of storage clusters that had finished
	// the installation of the new certificates which are chained back to the new ADS root CA.
	CompletedClusterCount int `json:"completedClusterCount,omitempty"`
}

func (v Versions) Empty() bool {
	return v.ADS == "" && v.Firetap == ""
}

func (v Versions) ADSEmpty() bool {
	return v.ADS == ""
}

func (v Versions) FiretapEmpty() bool {
	return v.Firetap == ""
}

func (v Versions) String() string {
	return fmt.Sprintf("ADS: %v, Firetap: %v", v.ADS, v.Firetap)
}

// AstraDSVersionSpec defines the desired state of AstraDSVersion
type AstraDSVersionSpec struct {
	Images   Images            `json:"images"`
	Versions ValidatedVersions `json:"versions"`
}

// AstraDSVersionStatus defines the observed state of AstraDSVersion
type AstraDSVersionStatus struct {
	Images               Images                      `json:"images"`
	Versions             Versions                    `json:"versions"`
	ADSCertificateUpdate *ADSCertificateUpdateStatus `json:"adsCertificateUpdate,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=adsve,categories={ads,all}

// AstraDSVersion is the Schema for the astradsversions API
type AstraDSVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSVersionSpec   `json:"spec,omitempty"`
	Status AstraDSVersionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSVersionList contains a list of AstraDSVersion
type AstraDSVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSVersion{}, &AstraDSVersionList{})
}
