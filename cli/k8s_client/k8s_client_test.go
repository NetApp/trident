// Copyright 2023 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

var mockError = fmt.Errorf("mock error")

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

// resetThirdPartyFunctions resets all third-party, first-class function variables back to their real definitions.
func resetThirdPartyFunctions() {
	yamlToJSON = yaml.YAMLToJSON
	jsonMarshal = json.Marshal
	jsonUnmarshal = json.Unmarshal
	jsonMergePatch = jsonpatch.MergePatch
}

func TestPatchCRD(t *testing.T) {
	// Set up types to make test cases easier
	// Input mirrors parameters for the function in question
	type input struct {
		crdName    string
		patchBytes []byte
		patchType  types.PatchType
	}

	// Output specifies what to expect
	type output struct {
		errorExpected bool
	}

	// Event is a client side event; what events the underlying clientset needs to return may be specified here.
	type event struct {
		error error
	}

	type testCase struct {
		input  input
		output output
		event  event
	}

	// Setup variables for test cases.
	clientError := errors.New("client error")
	crdName := "trident.netapp.io"
	crdYAML := `
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: false
      storage: false
      schema:
          openAPIV3Schema:
              type: object
              nullable: false`

	// Setup test cases.
	tests := map[string]testCase{
		"expect to pass with no error from the client set": {
			input: input{
				crdName:    crdName,
				patchBytes: []byte(crdYAML),
				patchType:  types.MergePatchType,
			},
			output: output{
				errorExpected: false,
			},
			event: event{
				error: nil,
			},
		},
		"expect to fail with an error from the client set": {
			input: input{
				crdName:    crdName,
				patchBytes: []byte(crdYAML),
				patchType:  types.MergePatchType,
			},
			output: output{
				errorExpected: true,
			},
			event: event{
				error: clientError,
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// Setting up a fake client set, adding reactors and injecting the client
				// into the K8s Client allows us to "mock" out anticipated
				// actions and objects from the underlying clientSet within the K8s Client.

				// Initialize the fakeClient
				fakeClient := fake.NewClientset()

				// Prepend a reactor for each anticipated event.
				event := test.event
				fakeClient.Fake.PrependReactor(
					"patch" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(actionCopy k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						switch actionCopy.(type) {
						case k8stesting.PatchActionImpl:
							// here it doesn't matter what is returned for the runtime.Object as the method
							// in question only returns an error (nil or non-nil)
							return true, nil, event.error
						default:
							Log().Errorf("~~~ unhandled type: %T\n",
								actionCopy) // use this to find if any unanticipated actions occurred.
						}
						return false, nil, nil
					},
				)

				// Inject the fakeClient into a KubeClient instance
				k := &KubeClient{extClientset: fakeClient}

				// Call the function being tested.
				err := k.PatchCRD(test.input.crdName, test.input.patchBytes, test.input.patchType)

				// Assert values
				if test.output.errorExpected {
					assert.NotNil(t, err, "expected non-nil error")
				} else {
					assert.Nil(t, err, "expected nil error")
				}
			},
		)
	}
}

func TestGenericPatch(t *testing.T) {
	// Set up an empty slice of bytes to use throughout the test cases.
	var emptyObject apiextensionv1.CustomResourceDefinition
	var crd apiextensionv1.CustomResourceDefinition

	newCrdYAML := `
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentversions.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: false
      storage: false
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
      - name: Version
        type: string
        description: The Trident version
        priority: 0
        jsonPath: .trident_version
  scope: Namespaced
  names:
    plural: tridentversions
    singular: tridentversion
    kind: TridentVersion
    shortNames:
    - tver
    - tversion
    categories:
    - trident
    - trident-internal`

	patchBytes := []byte(newCrdYAML)
	if err := yaml.Unmarshal([]byte(GetVersionCRDYAML()), &crd); err != nil {
		t.Fatalf("failed to unmarshall test yaml")
	}

	type input struct {
		crdObject  *apiextensionv1.CustomResourceDefinition
		patchBytes []byte
	}

	// Set up the test cases
	tests := map[string]struct {
		input         input
		errorExpected bool
		mocks         func() // this allows for easy patching of unmockable functions at runtime.
	}{
		"expect to fail when jsonMarshal fails": {
			input: input{
				crdObject:  emptyObject.DeepCopy(),
				patchBytes: patchBytes,
			},
			errorExpected: true,
			mocks: func() {
				jsonMarshal = func(any) ([]byte, error) {
					return nil, mockError
				}
			},
		},
		"expect to fail when yamlToJSON fails": {
			input: input{
				crdObject:  emptyObject.DeepCopy(),
				patchBytes: patchBytes,
			},
			errorExpected: true,
			mocks: func() {
				jsonMarshal = func(any) ([]byte, error) {
					return patchBytes, nil
				}
				yamlToJSON = func(yaml []byte) ([]byte, error) {
					return nil, mockError
				}
			},
		},
		"expect to fail when jsonMergePatch fails": {
			input: input{
				crdObject:  emptyObject.DeepCopy(),
				patchBytes: patchBytes,
			},
			errorExpected: true,
			mocks: func() {
				jsonMarshal = func(any) ([]byte, error) {
					return patchBytes, nil
				}
				yamlToJSON = func(yaml []byte) ([]byte, error) {
					return patchBytes, nil
				}
				jsonMergePatch = func(originalJSON, modifiedJSON []byte) ([]byte, error) {
					return nil, mockError
				}
			},
		},
		"expect to pass when no third-party function fails": {
			input: input{
				crdObject:  emptyObject.DeepCopy(),
				patchBytes: patchBytes,
			},
			errorExpected: false,
			mocks: func() {
				jsonMarshal = func(any) ([]byte, error) {
					return patchBytes, nil
				}
				yamlToJSON = func(yaml []byte) ([]byte, error) {
					return patchBytes, nil
				}
				jsonMergePatch = func(originalJSON, modifiedJSON []byte) ([]byte, error) {
					return patchBytes, nil
				}
			},
		},
		"expect to pass when valid YAML and objects are used": {
			input: input{
				crdObject:  crd.DeepCopy(),
				patchBytes: patchBytes,
			},
			errorExpected: false,
			mocks: func() {
				resetThirdPartyFunctions()
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// Get the input from the test case.
				crd, patchBytes := test.input.crdObject, test.input.patchBytes

				// test.mocks() mocks out different permutations of the 3rd-party functions.
				// We can test the logic of the function in question without worrying about implementation details of
				// those libraries.
				test.mocks()

				patchedCRD, actualErr := GenericPatch(crd, patchBytes)
				if test.errorExpected {
					assert.NotNil(t, actualErr, "expected non-nil error")
				} else {
					assert.Nil(t, actualErr, "expected nil error")

					// Get the []byte representation of the existingCRD.
					existingCRD, err := json.Marshal(crd)
					if err != nil {
						t.Fatalf("unable to Marshal object")
					}

					assert.NotEqual(t, string(existingCRD), string(patchedCRD), "expected YAML to be different")
				}

				// Always reset the third party functions after every test.
				resetThirdPartyFunctions()
			},
		)
	}
}
