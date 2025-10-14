package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/semaphore"

	drivers "github.com/netapp/trident/storage_drivers"
)

func TestValidateZAPIResponse(t *testing.T) {
	// Sample of exploit-capable xml
	// from https://github.com/mattermost/xml-roundtrip-validator/blob/master/validator_test.go used as a sanity test
	simpleInvalidResponseBody := `<Root ::attr="x">]]><x::Element/></Root>`

	sampleResponse := &http.Response{
		Body:          io.NopCloser(bytes.NewBufferString(simpleInvalidResponseBody)),
		ContentLength: int64(len(simpleInvalidResponseBody)),
	}
	_, err := ValidateZAPIResponse(sampleResponse)
	assert.Error(t, err, "should error on input with colons in an attribute's name")

	// <! <<!-- -->!-->"--> " ><! ">" <X/>>
	// Is an invalid xml directive
	// Make sure that the validator combined with Go 1.17+ passes the invalid directive in a ZAPI XML response
	// that would be valid in versions of Trident without the validation or would fail validation with older Go versions.
	validToUnpatchedTridentResponseBody := `<?xml version='1.0' encoding='UTF-8' ?>
<!DOCTYPE netapp SYSTEM 'file:/etc/netapp_gx.dtd'><netapp version='1.180' xmlns='http://www.netapp.com/filer/admin'>
<! <<!-- -->!-->"--> " ><! ">" <X/>><results status="passed"><attributes-list><initiator-group-info>
<initiator-group-alua-enabled>true</initiator-group-alua-enabled><initiator-group-delete-on-unmap>false
</initiator-group-delete-on-unmap><initiator-group-name>presnellbr</initiator-group-name>
<initiator-group-os-type>linux</initiator-group-os-type><initiator-group-throttle-borrow>false
</initiator-group-throttle-borrow><initiator-group-throttle-reserve>0</initiator-group-throttle-reserve>
<initiator-group-type>iscsi</initiator-group-type><initiator-group-use-partner>true</initiator-group-use-partner>
<initiator-group-uuid>1c7b3edc-aead-11eb-b8e0-00a0986e75a0</initiator-group-uuid>
<initiator-group-vsa-enabled>false</initiator-group-vsa-enabled><initiators><initiator-info>
<initiator-name>iqn.2005-03.org.open-iscsi:cb50e2753efd</initiator-name>
</initiator-info></initiators><vserver>CXE</vserver></initiator-group-info></attributes-list><num-records>1
</num-records></results></netapp>"`

	sampleResponse = &http.Response{
		Body:          io.NopCloser(bytes.NewBufferString(validToUnpatchedTridentResponseBody)),
		ContentLength: int64(len(validToUnpatchedTridentResponseBody)),
	}

	_, err = ValidateZAPIResponse(sampleResponse)
	assert.NoError(t, err, "an invalid XML directive in an otherwise valid ZAPI XML response caused"+
		" the validation to return an error")

	var zapiResp IgroupGetIterResponse
	unmarshalErr := xml.Unmarshal([]byte(validToUnpatchedTridentResponseBody), &zapiResp)
	errMsg := "the xml document should be unmarshalled without issue when not validated properly"
	assert.Nil(t, unmarshalErr, errMsg)
}

func TestGetSVM(t *testing.T) {
	svmName := "test_svm"
	zRunner := ZapiRunner{
		m:   &sync.RWMutex{},
		svm: svmName,
	}
	// Test with a valid SVM name
	svm := zRunner.GetSVM()
	assert.Equal(t, svmName, svm, "GetSVM should return the correct SVM name")
}

func TestSetOntapVersion(t *testing.T) {
	ontapVersion := "9.8"
	zRunner := ZapiRunner{
		m: &sync.RWMutex{},
	}
	// Set a new ONTAP version
	zRunner.SetOntapApiVersion(ontapVersion)
	assert.Equal(t, ontapVersion, zRunner.ontapApiVersion, "SetOntapVersion should update the ONTAP version correctly")
}

func TestGetOntapVersion(t *testing.T) {
	ontapVersion := "9.8"
	zRunner := ZapiRunner{
		m:               &sync.RWMutex{},
		ontapApiVersion: ontapVersion,
	}
	// Test with a valid ONTAP version
	version := zRunner.GetOntapApiVersion()
	assert.Equal(t, ontapVersion, version, "GetOntapVersion should return the correct ONTAP version")
}

func TestCopyForNontunneledZapiRunner(t *testing.T) {
	// Create a ZapiRunner instance
	originalZRunner := &ZapiRunner{
		m:               &sync.RWMutex{},
		svm:             "test_svm",
		ontapApiVersion: "9.8",
	}

	copiedZRunner := originalZRunner.CopyForNontunneledZapiRunner()

	assert.NotSame(t, originalZRunner, copiedZRunner, "Copied ZapiRunner should not be the same instance")
	assert.Equal(t, "", copiedZRunner.svm, "SVM name should be empty in copied ZapiRunner")
	assert.Equal(t, originalZRunner.ontapApiVersion, copiedZRunner.ontapApiVersion, "ONTAP versions should match")
	assert.NotSame(t, originalZRunner.m, copiedZRunner.m, "Mutexes should not be the same instance")
	assert.NotNil(t, copiedZRunner.m, "Copied ZapiRunner should have a non-nil mutex")
}

func CreateTestZapiRunner(n int) *ZapiRunner {
	managementLIF := "1.2.3.4"
	svm := fmt.Sprintf("svm-%d", n)
	username := fmt.Sprintf("user%d", n)
	password := fmt.Sprintf("password%d", n)
	clientPrivateKey := ""
	clientCertificate := ""
	clientCACert := ""
	secure := false
	ontapApiVersion := "9.8"
	debugTraceFlags := map[string]bool{}
	return &ZapiRunner{
		ManagementLIF:        managementLIF,
		svm:                  svm,
		Username:             username,
		Password:             password,
		ClientPrivateKey:     clientPrivateKey,
		ClientCertificate:    clientCertificate,
		TrustedCACertificate: clientCACert,
		Secure:               secure,
		ontapApiVersion:      ontapApiVersion,
		DebugTraceFlags:      debugTraceFlags,
		m:                    &sync.RWMutex{},
	}
}

type RoundTripFunc func(*http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

var mockZapiErrorVserverNotFound string = `<?xml version="1.0" encoding="UTF-8"?>
<netapp version="1.21" xmlns="http://www.netapp.com/filer/admin">
  <results status="failed" errno="15698" reason="Simulated error for testing"/>
</netapp>`

// TestZapiRunner_ExecuteUsing_Parallel_MC tests the parallel execution of ExecuteUsing where the SVM name changes
func TestZapiRunner_ExecuteUsing_Parallel_MC(t *testing.T) {
	// Test parallel calls to ExecuteUsing with MC name changes
	zRunner := CreateTestZapiRunner(1)
	mockTransport := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(mockZapiErrorVserverNotFound)),
			Header:     make(http.Header),
		}, nil
	})
	zRunner.httpClient = &http.Client{
		Transport: drivers.NewLimitedRetryTransport(semaphore.NewWeighted(1), mockTransport),
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = zRunner.ExecuteUsing(&VolumeCreateRequest{}, "VolumeCreateRequest", NewVolumeCreateResponse())
		}(i)
	}
	wg.Wait() // Wait for all goroutines to finish

	// Check the SVM name, it should be one of the expected values
	assert.True(t, zRunner.svm == "svm-1-mc" || zRunner.svm == "svm-1")
}
