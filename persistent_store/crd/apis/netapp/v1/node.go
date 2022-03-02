// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/utils"
)

// NewTridentNode creates a new node CRD object from a internal utils.TridentNode object.
func NewTridentNode(persistent *utils.Node) (*TridentNode, error) {

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
func (in *TridentNode) Apply(persistent *utils.Node) error {
	if NameFix(persistent.Name) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	in.Name = persistent.Name
	in.IQN = persistent.IQN
	in.IPs = persistent.IPs
	in.Deleted = persistent.Deleted

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
func (in *TridentNode) Persistent() (*utils.Node, error) {
	persistent := &utils.Node{
		Name:     in.Name,
		IQN:      in.IQN,
		IPs:      in.IPs,
		NodePrep: &utils.NodePrep{},
		HostInfo: &utils.HostSystem{},
		Deleted:  in.Deleted,
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

func (in *TridentNode) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentNode) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentNode) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}
