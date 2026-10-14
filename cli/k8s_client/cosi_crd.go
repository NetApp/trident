// Copyright 2026 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"context"
	"fmt"

	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	cosicrd "github.com/netapp/trident/cosi/crd"
)

// COSICRDAction is what an installer should do with one COSI CRD.
type COSICRDAction int

const (
	// COSICRDCreate means the CRD is absent and Trident should install it.
	COSICRDCreate COSICRDAction = iota

	// COSICRDUpdate means Trident installed this CRD previously and may update it.
	COSICRDUpdate

	// COSICRDConflict means something other than Trident owns this CRD. The installer must
	// not touch it, and COSI must not be enabled against it.
	COSICRDConflict
)

func (a COSICRDAction) String() string {
	switch a {
	case COSICRDCreate:
		return "create"
	case COSICRDUpdate:
		return "update"
	case COSICRDConflict:
		return "conflict"
	default:
		return "unknown"
	}
}

// PlanCOSICRD decides what to do with one COSI CRD, given whatever is already in the cluster.
//
// Pass existing as nil when the CRD is absent.
//
// This is separated from the code that executes it because the decision is the interesting
// part and the two installers -- tridentctl and the operator -- reach it by different routes.
// It is also the one place where Trident's rule about not owning other people's APIs lives.
//
// The rule: Trident's own CRDs are in a group Trident owns, so the installer can always patch
// them. objectstorage.k8s.io is a community group. A CRD in it may have been installed by the
// upstream COSI controller or by another vendor's driver, and patching it to Trident's pinned
// v1alpha2 schema would silently change the API out from under them. So Trident marks what it
// installs and refuses everything else.
func PlanCOSICRD(existing *apiextensionv1.CustomResourceDefinition) COSICRDAction {
	if existing == nil {
		return COSICRDCreate
	}

	if cosicrd.IsTridentManaged(existing.Labels) {
		return COSICRDUpdate
	}

	return COSICRDConflict
}

// COSICRDConflictError explains a refusal in terms an administrator can act on.
//
// Worth the type: the alternative failure mode is Trident quietly patching a CRD another
// controller depends on, which surfaces days later as that controller rejecting its own
// objects. A blunt error at install time is much cheaper.
type COSICRDConflictError struct {
	CRDName string
}

func (e *COSICRDConflictError) Error() string {
	return fmt.Sprintf("the %s CRD already exists and was not installed by Trident. "+
		"Trident will not modify a COSI API it does not own. Either uninstall the existing COSI "+
		"controller or driver and remove its CRDs, or install Trident without COSI enabled. "+
		"Trident supports being the only COSI provider in a cluster", e.CRDName)
}

// COSICRDNames returns the COSI CRDs Trident installs.
func COSICRDNames() []string {
	return cosicrd.Names()
}

// COSICRDReader is the slice of KubernetesClient that TridentCOSIObjectsExist needs.
// KubernetesClient satisfies it; declaring it here keeps the dependency legible and lets a test
// stand up two methods instead of the whole client.
type COSICRDReader interface {
	CheckCRDExists(crdName string) (bool, error)
	GetCRD(crdName string) (*apiextensionv1.CustomResourceDefinition, error)
}

// TridentCOSIObjectsExist reports whether any custom resources remain under the COSI CRDs
// Trident installed. It answers one question: would disabling COSI abandon anything?
//
// CRDs that are absent, or that exist without Trident's ownership label, are skipped. An
// unlabelled CRD belongs to another COSI provider whose controller is presumably still running,
// so its objects are not being abandoned and are none of Trident's business to report on.
//
// Runs on every reconcile, so it is built to be cheap: the common case is that COSI was never
// enabled and the CRDs do not exist, costing one lookup each and no listing. Where a list is
// needed it asks for a single item, and it stops at the first kind that has one.
func TridentCOSIObjectsExist(
	ctx context.Context, client COSICRDReader, dynamicClient dynamic.Interface,
) (bool, error) {
	for _, crdName := range cosicrd.Names() {
		exists, err := client.CheckCRDExists(crdName)
		if err != nil {
			return false, fmt.Errorf("could not check whether the %s CRD exists; %v", crdName, err)
		}
		if !exists {
			continue
		}

		crd, err := client.GetCRD(crdName)
		if err != nil {
			return false, fmt.Errorf("could not retrieve the %s CRD; %v", crdName, err)
		}

		if !cosicrd.IsTridentManaged(crd.Labels) {
			continue
		}

		gvr := schema.GroupVersionResource{
			Group:    crd.Spec.Group,
			Version:  cosicrd.Version,
			Resource: crd.Spec.Names.Plural,
		}

		list, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			return false, fmt.Errorf("could not list %s; %v", crdName, err)
		}

		if len(list.Items) > 0 {
			return true, nil
		}
	}

	return false, nil
}

// GetCOSICRDsYAML returns every COSI CRD as one multi-document YAML string, for
// --generate-custom-yaml and for callers that want to see what would be installed.
func GetCOSICRDsYAML() string {
	return cosicrd.GetCRDsYAML()
}

// GetOwnedCOSICRDYAML returns one COSI CRD's YAML, labelled as Trident-managed.
func GetOwnedCOSICRDYAML(crdName string) (string, error) {
	return cosicrd.GetOwnedCRDYAML(crdName)
}
