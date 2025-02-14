// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/utils/models"
)

// NewTridentNode creates a new node CRD object from a internal utils.TridentNode object.
func NewTridentNode(persistent *models.Node) (*TridentNode, error) {
	node := &TridentNode{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentNode",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(persistent.Name),
			Finalizers: GetTridentFinalizers(),
		},
	}

	if err := node.Apply(persistent); err != nil {
		return nil, err
	}

	return node, nil
}

// Apply applies changes from an internal utils.TridentNode
// object to its Kubernetes CRD equivalent.
func (in *TridentNode) Apply(persistent *models.Node) error {
	if NameFix(persistent.Name) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	in.Name = persistent.Name
	in.IQN = persistent.IQN
	in.NQN = persistent.NQN
	in.IPs = persistent.IPs
	in.HostWWPNMap = persistent.HostWWPNMap
	in.Deleted = persistent.Deleted
	in.PublicationState = string(persistent.PublicationState)
	in.LogLevel = persistent.LogLevel
	in.LogWorkflows = persistent.LogWorkflows
	in.LogLayers = persistent.LogLayers

	nodePrep, err := json.Marshal(persistent.NodePrep)
	if err != nil {
		return err
	}
	in.NodePrep.Raw = nodePrep
	hostInfo, err := json.Marshal(persistent.HostInfo)
	if err != nil {
		return err
	}
	in.HostInfo.Raw = hostInfo

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// utils.TridentNode equivalent.
func (in *TridentNode) Persistent() (*models.Node, error) {
	publicationState := models.NodePublicationState(in.PublicationState)

	if publicationState == "" {
		publicationState = models.NodeClean
	}
	persistent := &models.Node{
		Name:             in.Name,
		IQN:              in.IQN,
		NQN:              in.NQN,
		IPs:              in.IPs,
		HostWWPNMap:      in.HostWWPNMap,
		NodePrep:         &models.NodePrep{},
		HostInfo:         &models.HostSystem{},
		Deleted:          in.Deleted,
		PublicationState: publicationState,
	}

	if string(in.NodePrep.Raw) != "" {
		err := json.Unmarshal(in.NodePrep.Raw, persistent.NodePrep)
		if err != nil {
			return persistent, err
		}
	}
	if string(in.HostInfo.Raw) != "" {
		err := json.Unmarshal(in.HostInfo.Raw, persistent.HostInfo)
		if err != nil {
			return persistent, err
		}
	}

	return persistent, nil
}

func (in *TridentNode) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentNode) GetKind() string {
	return "TridentNode"
}

func (in *TridentNode) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentNode) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentNode) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
