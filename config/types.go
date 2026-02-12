package config

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type ContainersResourceRequirements map[string]*ContainerResource

// Resources mirrors trident/operator/crd/apis/netapp/v1/Resources exactly.
// The duplication exists because Trident currently has no admission webhook to validate CRD fields.
// TODO(pshashan): Remove or refactor this if an admission webhook is ever implemented.
type Resources struct {
	Controller ContainersResourceRequirements `json:"controller,omitempty"`
	Node       *NodeResources                 `json:"node,omitempty"`
}

type NodeResources struct {
	Linux   ContainersResourceRequirements `json:"linux,omitempty"`
	Windows ContainersResourceRequirements `json:"windows,omitempty"`
}

type ContainerResource struct {
	Requests *ResourceRequirements `json:"requests,omitempty"`
	Limits   *ResourceRequirements `json:"limits,omitempty"`
}

type ResourceRequirements struct {
	CPU    *resource.Quantity `json:"cpu,omitempty"`
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// These manual DeepCopy implementations are required because:
// 1. The Resources types above are used in the operator CRD (trident/operator/crd/apis/netapp/v1/types.go)
// 2. Kubernetes controller-gen requires DeepCopy methods for all types used in CRDs
// 3. controller-gen only generates DeepCopy methods for types within the same package, not for imported types
// 4. Since these types are defined in the config package but referenced by the operator CRD package,
//    controller-gen cannot auto-generate DeepCopy methods for them in the operator package

// DeepCopyInto creates a deep copy of the receiver, writing into out.
func (in ContainersResourceRequirements) DeepCopyInto(out *ContainersResourceRequirements) {
	{
		in := &in
		*out = make(ContainersResourceRequirements, len(*in))
		for key, val := range *in {
			var outVal *ContainerResource
			if val == nil {
				(*out)[key] = nil
			} else {
				in, out := &val, &outVal
				*out = new(ContainerResource)
				(*in).DeepCopyInto(*out)
			}
			(*out)[key] = outVal
		}
		return
	}
}

// DeepCopy creates a deep copy of the receiver.
func (in ContainersResourceRequirements) DeepCopy() ContainersResourceRequirements {
	if in == nil {
		return nil
	}
	out := new(ContainersResourceRequirements)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto creates a deep copy of the receiver, writing into out.
func (in *Resources) DeepCopyInto(out *Resources) {
	*out = *in
	if in.Controller != nil {
		in, out := &in.Controller, &out.Controller
		(*in).DeepCopyInto(out)
	}
	if in.Node != nil {
		in, out := &in.Node, &out.Node
		*out = new(NodeResources)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy creates a deep copy of the receiver.
func (in *Resources) DeepCopy() *Resources {
	if in == nil {
		return nil
	}
	out := new(Resources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto creates a deep copy of the receiver, writing into out.
func (in *NodeResources) DeepCopyInto(out *NodeResources) {
	*out = *in
	if in.Linux != nil {
		in, out := &in.Linux, &out.Linux
		(*in).DeepCopyInto(out)
	}
	if in.Windows != nil {
		in, out := &in.Windows, &out.Windows
		(*in).DeepCopyInto(out)
	}
}

// DeepCopy creates a deep copy of the receiver.
func (in *NodeResources) DeepCopy() *NodeResources {
	if in == nil {
		return nil
	}
	out := new(NodeResources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto creates a deep copy of the receiver, writing into out.
func (in *ContainerResource) DeepCopyInto(out *ContainerResource) {
	*out = *in
	if in.Requests != nil {
		in, out := &in.Requests, &out.Requests
		*out = new(ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.Limits != nil {
		in, out := &in.Limits, &out.Limits
		*out = new(ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy creates a deep copy of the receiver.
func (in *ContainerResource) DeepCopy() *ContainerResource {
	if in == nil {
		return nil
	}
	out := new(ContainerResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto creates a deep copy of the receiver, writing into out.
func (in *ResourceRequirements) DeepCopyInto(out *ResourceRequirements) {
	*out = *in
	if in.CPU != nil {
		in, out := &in.CPU, &out.CPU
		x := (*in).DeepCopy()
		*out = &x
	}
	if in.Memory != nil {
		in, out := &in.Memory, &out.Memory
		x := (*in).DeepCopy()
		*out = &x
	}
}

// DeepCopy creates a deep copy of the receiver.
func (in *ResourceRequirements) DeepCopy() *ResourceRequirements {
	if in == nil {
		return nil
	}
	out := new(ResourceRequirements)
	in.DeepCopyInto(out)
	return out
}
