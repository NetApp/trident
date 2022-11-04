// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/utils"
)

func init() {
	testing.Init()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
}

func TestNewNode(t *testing.T) {
	// Build node
	utilsNode := &utils.Node{
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
