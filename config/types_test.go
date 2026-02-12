package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestContainersResourceRequirementsDeepCopy(t *testing.T) {
	cpu100m := resource.MustParse("100m")
	mem128Mi := resource.MustParse("128Mi")

	tests := []struct {
		name     string
		input    ContainersResourceRequirements
		validate func(t *testing.T, original ContainersResourceRequirements, copied ContainersResourceRequirements)
	}{
		{
			name:  "nil map",
			input: nil,
			validate: func(t *testing.T, original, copied ContainersResourceRequirements) {
				assert.Nil(t, copied, "DeepCopy of nil map should return nil")
			},
		},
		{
			name:  "empty map",
			input: ContainersResourceRequirements{},
			validate: func(t *testing.T, original, copied ContainersResourceRequirements) {
				assert.NotNil(t, copied)
				assert.Empty(t, copied)
			},
		},
		{
			name: "map with single container",
			input: ContainersResourceRequirements{
				"container1": &ContainerResource{
					Requests: &ResourceRequirements{
						CPU:    &cpu100m,
						Memory: &mem128Mi,
					},
				},
			},
			validate: func(t *testing.T, original, copied ContainersResourceRequirements) {
				assert.NotNil(t, copied)
				assert.Len(t, copied, 1)
				assert.Contains(t, copied, "container1")
				assert.NotSame(t, original["container1"], copied["container1"], "ContainerResource pointer should be different")
				assert.NotSame(t, original["container1"].Requests, copied["container1"].Requests, "Requests pointer should be different")
				assert.NotSame(t, original["container1"].Requests.CPU, copied["container1"].Requests.CPU, "CPU pointer should be different")
				assert.NotSame(t, original["container1"].Requests.Memory, copied["container1"].Requests.Memory, "Memory pointer should be different")
				assert.True(t, original["container1"].Requests.CPU.Equal(*copied["container1"].Requests.CPU))
				assert.True(t, original["container1"].Requests.Memory.Equal(*copied["container1"].Requests.Memory))
			},
		},
		{
			name: "map with multiple containers",
			input: ContainersResourceRequirements{
				"container1": &ContainerResource{
					Requests: &ResourceRequirements{CPU: &cpu100m},
				},
				"container2": &ContainerResource{
					Limits: &ResourceRequirements{Memory: &mem128Mi},
				},
			},
			validate: func(t *testing.T, original, copied ContainersResourceRequirements) {
				assert.NotNil(t, copied)
				assert.Len(t, copied, 2)
				assert.Contains(t, copied, "container1")
				assert.Contains(t, copied, "container2")
				assert.NotSame(t, original["container1"], copied["container1"])
				assert.NotSame(t, original["container2"], copied["container2"])
				assert.NotSame(t, original["container1"].Requests, copied["container1"].Requests, "Container1 Requests pointer should be different")
				assert.NotSame(t, original["container1"].Requests.CPU, copied["container1"].Requests.CPU, "Container1 CPU pointer should be different")
				assert.NotSame(t, original["container2"].Limits, copied["container2"].Limits, "Container2 Limits pointer should be different")
				assert.NotSame(t, original["container2"].Limits.Memory, copied["container2"].Limits.Memory, "Container2 Memory pointer should be different")
				assert.True(t, original["container1"].Requests.CPU.Equal(*copied["container1"].Requests.CPU))
				assert.True(t, original["container2"].Limits.Memory.Equal(*copied["container2"].Limits.Memory))
			},
		},
		{
			name: "map with nil value",
			input: ContainersResourceRequirements{
				"container1": nil,
				"container2": &ContainerResource{
					Requests: &ResourceRequirements{CPU: &cpu100m},
				},
			},
			validate: func(t *testing.T, original, copied ContainersResourceRequirements) {
				assert.NotNil(t, copied)
				assert.Len(t, copied, 2)
				assert.Nil(t, copied["container1"], "Nil value should remain nil")
				assert.NotNil(t, copied["container2"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.input.DeepCopy()
			tt.validate(t, tt.input, copied)
		})
	}
}

func TestResourcesDeepCopy(t *testing.T) {
	cpu100m := resource.MustParse("100m")
	mem128Mi := resource.MustParse("128Mi")

	tests := []struct {
		name     string
		input    *Resources
		validate func(t *testing.T, original *Resources, copied *Resources)
	}{
		{
			name:  "nil Resources",
			input: nil,
			validate: func(t *testing.T, original, copied *Resources) {
				assert.Nil(t, copied, "DeepCopy of nil should return nil")
			},
		},
		{
			name:  "empty Resources",
			input: &Resources{},
			validate: func(t *testing.T, original, copied *Resources) {
				assert.NotNil(t, copied)
				assert.Nil(t, copied.Controller)
				assert.Nil(t, copied.Node)
			},
		},
		{
			name: "Resources with Controller only",
			input: &Resources{
				Controller: ContainersResourceRequirements{
					"trident-main": &ContainerResource{
						Requests: &ResourceRequirements{CPU: &cpu100m},
					},
				},
			},
			validate: func(t *testing.T, original, copied *Resources) {
				assert.NotNil(t, copied)
				assert.NotNil(t, copied.Controller)
				assert.Nil(t, copied.Node)
				assert.Len(t, copied.Controller, 1)
				assert.NotSame(t, original.Controller["trident-main"], copied.Controller["trident-main"])
				assert.NotSame(t, original.Controller["trident-main"].Requests, copied.Controller["trident-main"].Requests, "Requests pointer should be different")
				assert.NotSame(t, original.Controller["trident-main"].Requests.CPU, copied.Controller["trident-main"].Requests.CPU, "CPU pointer should be different")
				assert.True(t, original.Controller["trident-main"].Requests.CPU.Equal(*copied.Controller["trident-main"].Requests.CPU))
			},
		},
		{
			name: "Resources with Node only",
			input: &Resources{
				Node: &NodeResources{
					Linux: ContainersResourceRequirements{
						"node-container": &ContainerResource{
							Limits: &ResourceRequirements{Memory: &mem128Mi},
						},
					},
				},
			},
			validate: func(t *testing.T, original, copied *Resources) {
				assert.NotNil(t, copied)
				assert.Nil(t, copied.Controller)
				assert.NotNil(t, copied.Node)
				assert.NotSame(t, original.Node, copied.Node, "Node pointer should be different")
				assert.NotNil(t, copied.Node.Linux)
				assert.Len(t, copied.Node.Linux, 1)
				assert.NotSame(t, original.Node.Linux["node-container"], copied.Node.Linux["node-container"], "ContainerResource pointer should be different")
				assert.NotSame(t, original.Node.Linux["node-container"].Limits, copied.Node.Linux["node-container"].Limits, "Limits pointer should be different")
				assert.NotSame(t, original.Node.Linux["node-container"].Limits.Memory, copied.Node.Linux["node-container"].Limits.Memory, "Memory pointer should be different")
				assert.True(t, original.Node.Linux["node-container"].Limits.Memory.Equal(*copied.Node.Linux["node-container"].Limits.Memory))
			},
		},
		{
			name: "Resources with both Controller and Node",
			input: &Resources{
				Controller: ContainersResourceRequirements{
					"controller-container": &ContainerResource{
						Requests: &ResourceRequirements{CPU: &cpu100m},
					},
				},
				Node: &NodeResources{
					Linux: ContainersResourceRequirements{
						"node-container": &ContainerResource{
							Limits: &ResourceRequirements{Memory: &mem128Mi},
						},
					},
				},
			},
			validate: func(t *testing.T, original, copied *Resources) {
				assert.NotNil(t, copied)
				assert.NotNil(t, copied.Controller)
				assert.NotNil(t, copied.Node)
				assert.Len(t, copied.Controller, 1)
				assert.NotSame(t, original.Controller["controller-container"], copied.Controller["controller-container"])
				assert.NotSame(t, original.Node, copied.Node)
				assert.NotNil(t, copied.Node.Linux)
				// Assert Controller nested pointers
				assert.NotSame(t, original.Controller["controller-container"].Requests, copied.Controller["controller-container"].Requests, "Controller Requests pointer should be different")
				assert.NotSame(t, original.Controller["controller-container"].Requests.CPU, copied.Controller["controller-container"].Requests.CPU, "Controller CPU pointer should be different")
				assert.True(t, original.Controller["controller-container"].Requests.CPU.Equal(*copied.Controller["controller-container"].Requests.CPU))
				// Assert Node nested pointers
				assert.NotSame(t, original.Node.Linux["node-container"], copied.Node.Linux["node-container"], "Node ContainerResource pointer should be different")
				assert.NotSame(t, original.Node.Linux["node-container"].Limits, copied.Node.Linux["node-container"].Limits, "Node Limits pointer should be different")
				assert.NotSame(t, original.Node.Linux["node-container"].Limits.Memory, copied.Node.Linux["node-container"].Limits.Memory, "Node Memory pointer should be different")
				assert.True(t, original.Node.Linux["node-container"].Limits.Memory.Equal(*copied.Node.Linux["node-container"].Limits.Memory))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.input.DeepCopy()
			tt.validate(t, tt.input, copied)
		})
	}
}

func TestNodeResourcesDeepCopy(t *testing.T) {
	cpu100m := resource.MustParse("100m")
	mem128Mi := resource.MustParse("128Mi")

	tests := []struct {
		name     string
		input    *NodeResources
		validate func(t *testing.T, original *NodeResources, copied *NodeResources)
	}{
		{
			name:  "nil NodeResources",
			input: nil,
			validate: func(t *testing.T, original, copied *NodeResources) {
				assert.Nil(t, copied, "DeepCopy of nil should return nil")
			},
		},
		{
			name:  "empty NodeResources",
			input: &NodeResources{},
			validate: func(t *testing.T, original, copied *NodeResources) {
				assert.NotNil(t, copied)
				assert.Nil(t, copied.Linux)
				assert.Nil(t, copied.Windows)
			},
		},
		{
			name: "NodeResources with Linux only",
			input: &NodeResources{
				Linux: ContainersResourceRequirements{
					"node-main": &ContainerResource{
						Requests: &ResourceRequirements{CPU: &cpu100m},
					},
				},
			},
			validate: func(t *testing.T, original, copied *NodeResources) {
				assert.NotNil(t, copied)
				assert.NotNil(t, copied.Linux)
				assert.Nil(t, copied.Windows)
				assert.Len(t, copied.Linux, 1)
				assert.NotSame(t, original.Linux["node-main"], copied.Linux["node-main"])
				assert.NotSame(t, original.Linux["node-main"].Requests, copied.Linux["node-main"].Requests, "Requests pointer should be different")
				assert.NotSame(t, original.Linux["node-main"].Requests.CPU, copied.Linux["node-main"].Requests.CPU, "CPU pointer should be different")
				assert.True(t, original.Linux["node-main"].Requests.CPU.Equal(*copied.Linux["node-main"].Requests.CPU))
			},
		},
		{
			name: "NodeResources with Windows only",
			input: &NodeResources{
				Windows: ContainersResourceRequirements{
					"node-main": &ContainerResource{
						Requests: &ResourceRequirements{Memory: &mem128Mi},
					},
				},
			},
			validate: func(t *testing.T, original, copied *NodeResources) {
				assert.NotNil(t, copied)
				assert.Nil(t, copied.Linux)
				assert.NotNil(t, copied.Windows)
				assert.Len(t, copied.Windows, 1)
				assert.NotSame(t, original.Windows["node-main"], copied.Windows["node-main"])
				assert.NotSame(t, original.Windows["node-main"].Requests, copied.Windows["node-main"].Requests, "Requests pointer should be different")
				assert.NotSame(t, original.Windows["node-main"].Requests.Memory, copied.Windows["node-main"].Requests.Memory, "Memory pointer should be different")
				assert.True(t, original.Windows["node-main"].Requests.Memory.Equal(*copied.Windows["node-main"].Requests.Memory))
			},
		},
		{
			name: "NodeResources with both Linux and Windows",
			input: &NodeResources{
				Linux: ContainersResourceRequirements{
					"linux-container": &ContainerResource{
						Requests: &ResourceRequirements{CPU: &cpu100m},
					},
				},
				Windows: ContainersResourceRequirements{
					"windows-container": &ContainerResource{
						Limits: &ResourceRequirements{Memory: &mem128Mi},
					},
				},
			},
			validate: func(t *testing.T, original, copied *NodeResources) {
				assert.NotNil(t, copied)
				assert.NotNil(t, copied.Linux)
				assert.NotNil(t, copied.Windows)
				assert.Len(t, copied.Linux, 1)
				assert.Len(t, copied.Windows, 1)
				assert.NotSame(t, original.Linux["linux-container"], copied.Linux["linux-container"])
				assert.NotSame(t, original.Windows["windows-container"], copied.Windows["windows-container"])
				// Assert Linux nested pointers
				assert.NotSame(t, original.Linux["linux-container"].Requests, copied.Linux["linux-container"].Requests, "Linux Requests pointer should be different")
				assert.NotSame(t, original.Linux["linux-container"].Requests.CPU, copied.Linux["linux-container"].Requests.CPU, "Linux CPU pointer should be different")
				assert.True(t, original.Linux["linux-container"].Requests.CPU.Equal(*copied.Linux["linux-container"].Requests.CPU))
				// Assert Windows nested pointers
				assert.NotSame(t, original.Windows["windows-container"].Limits, copied.Windows["windows-container"].Limits, "Windows Limits pointer should be different")
				assert.NotSame(t, original.Windows["windows-container"].Limits.Memory, copied.Windows["windows-container"].Limits.Memory, "Windows Memory pointer should be different")
				assert.True(t, original.Windows["windows-container"].Limits.Memory.Equal(*copied.Windows["windows-container"].Limits.Memory))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.input.DeepCopy()
			tt.validate(t, tt.input, copied)
		})
	}
}

func TestContainerResourceDeepCopy(t *testing.T) {
	cpu100m := resource.MustParse("100m")
	mem128Mi := resource.MustParse("128Mi")
	cpu200m := resource.MustParse("200m")
	mem256Mi := resource.MustParse("256Mi")

	tests := []struct {
		name     string
		input    *ContainerResource
		validate func(t *testing.T, original *ContainerResource, copied *ContainerResource)
	}{
		{
			name:  "nil ContainerResource",
			input: nil,
			validate: func(t *testing.T, original, copied *ContainerResource) {
				assert.Nil(t, copied, "DeepCopy of nil should return nil")
			},
		},
		{
			name:  "empty ContainerResource",
			input: &ContainerResource{},
			validate: func(t *testing.T, original, copied *ContainerResource) {
				assert.NotNil(t, copied)
				assert.Nil(t, copied.Requests)
				assert.Nil(t, copied.Limits)
			},
		},
		{
			name: "ContainerResource with Requests only",
			input: &ContainerResource{
				Requests: &ResourceRequirements{
					CPU:    &cpu100m,
					Memory: &mem128Mi,
				},
			},
			validate: func(t *testing.T, original, copied *ContainerResource) {
				assert.NotNil(t, copied)
				assert.NotNil(t, copied.Requests)
				assert.Nil(t, copied.Limits)
				assert.NotSame(t, original.Requests, copied.Requests, "Requests pointer should be different")
				assert.NotSame(t, original.Requests.CPU, copied.Requests.CPU, "CPU pointer should be different")
				assert.NotSame(t, original.Requests.Memory, copied.Requests.Memory, "Memory pointer should be different")
				assert.True(t, original.Requests.CPU.Equal(*copied.Requests.CPU))
				assert.True(t, original.Requests.Memory.Equal(*copied.Requests.Memory))
			},
		},
		{
			name: "ContainerResource with Limits only",
			input: &ContainerResource{
				Limits: &ResourceRequirements{
					CPU:    &cpu200m,
					Memory: &mem256Mi,
				},
			},
			validate: func(t *testing.T, original, copied *ContainerResource) {
				assert.NotNil(t, copied)
				assert.Nil(t, copied.Requests)
				assert.NotNil(t, copied.Limits)
				assert.NotSame(t, original.Limits, copied.Limits, "Limits pointer should be different")
				assert.NotSame(t, original.Limits.CPU, copied.Limits.CPU, "CPU pointer should be different")
				assert.NotSame(t, original.Limits.Memory, copied.Limits.Memory, "Memory pointer should be different")
				assert.True(t, original.Limits.CPU.Equal(*copied.Limits.CPU))
				assert.True(t, original.Limits.Memory.Equal(*copied.Limits.Memory))
			},
		},
		{
			name: "ContainerResource with both Requests and Limits",
			input: &ContainerResource{
				Requests: &ResourceRequirements{
					CPU:    &cpu100m,
					Memory: &mem128Mi,
				},
				Limits: &ResourceRequirements{
					CPU:    &cpu200m,
					Memory: &mem256Mi,
				},
			},
			validate: func(t *testing.T, original, copied *ContainerResource) {
				assert.NotNil(t, copied)
				assert.NotNil(t, copied.Requests)
				assert.NotNil(t, copied.Limits)
				assert.NotSame(t, original.Requests, copied.Requests, "Requests pointer should be different")
				assert.NotSame(t, original.Limits, copied.Limits, "Limits pointer should be different")
				// Assert Requests nested pointers
				assert.NotSame(t, original.Requests.CPU, copied.Requests.CPU, "Requests CPU pointer should be different")
				assert.NotSame(t, original.Requests.Memory, copied.Requests.Memory, "Requests Memory pointer should be different")
				assert.True(t, original.Requests.CPU.Equal(*copied.Requests.CPU))
				assert.True(t, original.Requests.Memory.Equal(*copied.Requests.Memory))
				// Assert Limits nested pointers
				assert.NotSame(t, original.Limits.CPU, copied.Limits.CPU, "Limits CPU pointer should be different")
				assert.NotSame(t, original.Limits.Memory, copied.Limits.Memory, "Limits Memory pointer should be different")
				assert.True(t, original.Limits.CPU.Equal(*copied.Limits.CPU))
				assert.True(t, original.Limits.Memory.Equal(*copied.Limits.Memory))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.input.DeepCopy()
			tt.validate(t, tt.input, copied)
		})
	}
}

func TestResourceRequirementsDeepCopy(t *testing.T) {
	cpu100m := resource.MustParse("100m")
	cpu200m := resource.MustParse("200m")
	mem128Mi := resource.MustParse("128Mi")
	mem256Mi := resource.MustParse("256Mi")

	tests := []struct {
		name     string
		input    *ResourceRequirements
		validate func(t *testing.T, original *ResourceRequirements, copied *ResourceRequirements)
	}{
		{
			name:  "nil ResourceRequirements",
			input: nil,
			validate: func(t *testing.T, original, copied *ResourceRequirements) {
				assert.Nil(t, copied, "DeepCopy of nil should return nil")
			},
		},
		{
			name:  "empty ResourceRequirements",
			input: &ResourceRequirements{},
			validate: func(t *testing.T, original, copied *ResourceRequirements) {
				assert.NotNil(t, copied)
				assert.Nil(t, copied.CPU)
				assert.Nil(t, copied.Memory)
			},
		},
		{
			name: "ResourceRequirements with CPU only",
			input: &ResourceRequirements{
				CPU: &cpu100m,
			},
			validate: func(t *testing.T, original, copied *ResourceRequirements) {
				assert.NotNil(t, copied)
				assert.NotNil(t, copied.CPU)
				assert.Nil(t, copied.Memory)
				assert.NotSame(t, original.CPU, copied.CPU, "CPU pointer should be different")
				assert.True(t, original.CPU.Equal(*copied.CPU), "CPU values should be equal")
			},
		},
		{
			name: "ResourceRequirements with Memory only",
			input: &ResourceRequirements{
				Memory: &mem128Mi,
			},
			validate: func(t *testing.T, original, copied *ResourceRequirements) {
				assert.NotNil(t, copied)
				assert.Nil(t, copied.CPU)
				assert.NotNil(t, copied.Memory)
				assert.NotSame(t, original.Memory, copied.Memory, "Memory pointer should be different")
				assert.True(t, original.Memory.Equal(*copied.Memory), "Memory values should be equal")
			},
		},
		{
			name: "ResourceRequirements with both CPU and Memory",
			input: &ResourceRequirements{
				CPU:    &cpu200m,
				Memory: &mem256Mi,
			},
			validate: func(t *testing.T, original, copied *ResourceRequirements) {
				assert.NotNil(t, copied)
				assert.NotNil(t, copied.CPU)
				assert.NotNil(t, copied.Memory)
				assert.NotSame(t, original.CPU, copied.CPU, "CPU pointer should be different")
				assert.NotSame(t, original.Memory, copied.Memory, "Memory pointer should be different")
				assert.True(t, original.CPU.Equal(*copied.CPU), "CPU values should be equal")
				assert.True(t, original.Memory.Equal(*copied.Memory), "Memory values should be equal")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.input.DeepCopy()
			tt.validate(t, tt.input, copied)
		})
	}
}
