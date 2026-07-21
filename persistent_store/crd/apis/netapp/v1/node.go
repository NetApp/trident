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

// Apply applies changes from an internal utils.TridentNode object to its Kubernetes CRD equivalent.
//
// N+1 dual-write: it updates root-level fields, spec, and mirrored status fields so dual-read paths
// can still resolve node data. PublicationState and Deleted are not part of Spec; they are written to
// root-level fields and Status. Callers must pass a models.Node that reflects the update they intend—a
// partially populated or default Node can overwrite orchestrator truth on update if it does not
// incorporate current controller state. Registration uses a Node built from local discovery; other
// call sites should merge persisted state into models.Node before Apply when updating existing objects.
func (in *TridentNode) Apply(persistent *models.Node) error {
	if NameFix(persistent.Name) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	// N+1 dual-write: keep root-level fields and spec/status fields in sync so
	// dual-read paths can continue reading node data.
	// TODO (N+2): Remove redundant root-level writes once spec/status-only is the canonical persisted format.
	in.NodeName = persistent.Name
	in.Spec.NodeName = persistent.Name
	in.IQN = persistent.IQN
	in.Spec.IQN = persistent.IQN
	in.NQN = persistent.NQN
	in.Spec.NQN = persistent.NQN
	in.IPs = persistent.IPs
	in.Spec.IPs = persistent.IPs
	in.HostWWPNMap = persistent.HostWWPNMap
	in.Spec.HostWWPNMap = persistent.HostWWPNMap
	in.Deleted = persistent.Deleted
	in.Status.Deleted = persistent.Deleted
	in.PublicationState = string(persistent.PublicationState)
	in.Status.PublicationState = string(persistent.PublicationState)
	in.LogLevel = persistent.LogLevel
	in.LogWorkflows = persistent.LogWorkflows
	in.LogLayers = persistent.LogLayers

	nodePrep, err := json.Marshal(persistent.NodePrep)
	if err != nil {
		return err
	}
	in.NodePrep.Raw = nodePrep
	in.Spec.NodePrep.Raw = nodePrep
	hostInfo, err := json.Marshal(persistent.HostInfo)
	if err != nil {
		return err
	}
	in.HostInfo.Raw = hostInfo
	in.Spec.HostInfo.Raw = hostInfo

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// utils.TridentNode equivalent.
func (in *TridentNode) Persistent() (*models.Node, error) {
	// N+1 dual-read: prefer status/spec, then fall back to root-level fields.
	// TODO (N+2): Remove root-field fallbacks after migration guarantees all TridentNode objects are spec/status-only.
	publicationStateValue := in.Status.PublicationState
	if publicationStateValue == "" {
		publicationStateValue = in.PublicationState
	}
	publicationState := models.NodePublicationState(publicationStateValue)

	if publicationState == "" {
		publicationState = models.NodeClean
	}
	nodeName := in.Spec.NodeName
	if nodeName == "" {
		nodeName = in.NodeName
	}
	if nodeName == "" {
		nodeName = in.ObjectMeta.Name
	}
	iqn := in.Spec.IQN
	if iqn == "" {
		iqn = in.IQN
	}
	nqn := in.Spec.NQN
	if nqn == "" {
		nqn = in.NQN
	}
	ips := in.Spec.IPs
	if ips == nil {
		ips = in.IPs
	}
	hostWWPNMap := in.Spec.HostWWPNMap
	if hostWWPNMap == nil {
		hostWWPNMap = in.HostWWPNMap
	}

	deleted := in.Status.Deleted || in.Deleted
	logLevel := in.Status.LogLevel
	if logLevel == "" {
		logLevel = in.LogLevel
	}
	logWorkflows := in.Status.LogWorkflows
	if logWorkflows == "" {
		logWorkflows = in.LogWorkflows
	}
	logLayers := in.Status.LogLayers
	if logLayers == "" {
		logLayers = in.LogLayers
	}
	persistent := &models.Node{
		Name:             nodeName,
		IQN:              iqn,
		NQN:              nqn,
		IPs:              ips,
		HostWWPNMap:      hostWWPNMap,
		NodePrep:         &models.NodePrep{},
		HostInfo:         &models.HostSystem{},
		Deleted:          deleted,
		PublicationState: publicationState,
		TopologyLabels:   in.Status.TopologyLabels,
		LogLevel:         logLevel,
		LogWorkflows:     logWorkflows,
		LogLayers:        logLayers,
	}

	nodePrepRaw := in.Spec.NodePrep.Raw
	if string(nodePrepRaw) == "" {
		nodePrepRaw = in.NodePrep.Raw
	}
	if string(nodePrepRaw) != "" {
		err := json.Unmarshal(nodePrepRaw, persistent.NodePrep)
		if err != nil {
			return persistent, err
		}
	}
	hostInfoRaw := in.Spec.HostInfo.Raw
	if string(hostInfoRaw) == "" {
		hostInfoRaw = in.HostInfo.Raw
	}
	if string(hostInfoRaw) != "" {
		err := json.Unmarshal(hostInfoRaw, persistent.HostInfo)
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
