// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/models"
)

func init() {
	testing.Init()
	if *debug {
		_ = InitLogLevel("debug")
	}
}

func TestNewNode(t *testing.T) {
	// Build node
	utilsNode := &models.Node{
		Name: "test",
		IQN:  "iqn",
		IPs: []string{
			"192.168.0.1",
		},
	}

	// Convert to Kubernetes Object using the NewTridentBackend method
	node, err := NewTridentNode(utilsNode)
	if err != nil {
		t.Fatal("Unable to construct TridentNode CRD: ", err)
	}

	if node.Name != utilsNode.Name {
		t.Fatalf("%v differs:  '%v' != '%v'", "Name", node.Name, utilsNode.Name)
	}

	if node.IQN != utilsNode.IQN {
		t.Fatalf("%v differs:  '%v' != '%v'", "IQN", node.IQN, utilsNode.IQN)
	}

	if len(node.IPs) != len(utilsNode.IPs) {
		t.Fatalf("%v differs:  '%v' != '%v'", "IPs", node.IPs, utilsNode.IPs)
	}

	for i := range node.IPs {
		if node.IPs[i] != utilsNode.IPs[i] {
			t.Fatalf("%v differs:  '%v' != '%v'", "IPs", node.IPs, utilsNode.IPs)
		}
	}

	if node == nil {
		t.Fatal("Unable to construct TridentNode CRD")
	}
}

func TestNewTridentNode_ObjectMetaNameUsesNameFix(t *testing.T) {
	utilsNode := &models.Node{
		Name: "Worker-Node_1",
		IQN:  "iqn",
	}
	node, err := NewTridentNode(utilsNode)
	if err != nil {
		t.Fatal(err)
	}
	want := NameFix(utilsNode.Name)
	if node.ObjectMeta.Name != want {
		t.Fatalf("ObjectMeta.Name %q != NameFix(...) %q", node.ObjectMeta.Name, want)
	}
}

func TestNewTridentNode_ApplyPopulatesStatus_NodeCreateShouldClearStatus(t *testing.T) {
	utilsNode := &models.Node{
		Name:             "test-node",
		IQN:              "iqn",
		PublicationState: models.NodeClean,
		Deleted:          false,
	}
	node, err := NewTridentNode(utilsNode)
	if err != nil {
		t.Fatal(err)
	}
	if got := node.Status.PublicationState; got != string(models.NodeClean) {
		t.Fatalf("Apply should mirror publication state into status: got %q", got)
	}
	// Node registration Create must not send controller-owned status (see kubernetes helper RegisterNode).
	node.Status = TridentNodeStatus{}
	var zero TridentNodeStatus
	if !reflect.DeepEqual(node.Status, zero) {
		t.Fatal("expected cleared Status to be zero value for Create")
	}
}

func TestTridentNode_Persistent_MapsLoggingFromStatus(t *testing.T) {
	nodeCR := &TridentNode{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Spec:       TridentNodeSpec{NodeName: "worker-1"},
		Status: TridentNodeStatus{
			LogLevel:         "debug",
			LogWorkflows:     "all",
			LogLayers:        "csi",
			Registered:       true,
			PublicationState: string(models.NodeClean),
		},
	}
	got, err := nodeCR.Persistent()
	if err != nil {
		t.Fatal(err)
	}
	if got.LogLevel != "debug" || got.LogWorkflows != "all" || got.LogLayers != "csi" {
		t.Fatalf("logging fields: level=%q workflows=%q layers=%q", got.LogLevel, got.LogWorkflows, got.LogLayers)
	}
}

func TestTridentNode_Persistent_LoggingFallsBackToLegacyRootFields(t *testing.T) {
	nodeCR := &TridentNode{
		ObjectMeta:   metav1.ObjectMeta{Name: "worker-1"},
		Spec:         TridentNodeSpec{NodeName: "worker-1"},
		LogLevel:     "info",
		LogWorkflows: "frontend",
		LogLayers:    "node",
	}
	got, err := nodeCR.Persistent()
	if err != nil {
		t.Fatal(err)
	}
	if got.LogLevel != "info" || got.LogWorkflows != "frontend" || got.LogLayers != "node" {
		t.Fatalf("logging fallback: level=%q workflows=%q layers=%q", got.LogLevel, got.LogWorkflows, got.LogLayers)
	}
}
