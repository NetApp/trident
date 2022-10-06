// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mock_ontap "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

func NewAPIResponse(
	client, version, status, reason, errno string,
) *api.APIResponse {
	result := api.NewAPIResponse(status, reason, errno)
	return result
}

var (
	APIResponsePassed = NewAPIResponse("client", "version", "passed", "reason", "errno")
	APIResponseFailed = NewAPIResponse("client", "version", "failed", "reason", "errno")
)

func TestApiGetError(t *testing.T) {
	ctx := context.Background()

	var snapListResponseErr error
	assert.Nil(t, snapListResponseErr)

	apiErr := api.GetError(ctx, nil, snapListResponseErr)
	assert.NotNil(t, apiErr)
	assert.Equal(t, "API error: nil response", apiErr.Error())

	apiErr = api.GetError(ctx, APIResponsePassed, nil)
	assert.Nil(t, apiErr)

	apiErr = api.GetError(ctx, APIResponsePassed, nil)
	assert.Nil(t, apiErr)

	apiErr = api.GetError(ctx, APIResponseFailed, nil)
	assert.NotNil(t, apiErr)
	assert.Equal(t, "API error: &{ failed reason errno}", apiErr.Error())
}

func newOntapStorageDriverConfig() *drivers.OntapStorageDriverConfig {
	ontapConfig := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DebugTraceFlags: map[string]bool{
				"method": true,
				"trace":  true,
				"api":    true,
			},
		},
	}
	return ontapConfig
}

// TestEnsureSVMWithRest validates we can derive the SVM if it is not specified
func TestEnsureSVMWithRest(t *testing.T) {
	ctx := context.Background()

	// create a config that is missing an SVM
	ontapConfig := newOntapStorageDriverConfig()
	ontapConfig.SVM = ""

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  no SVM set && we CANNOT derive an SVM because of a nil result
	mockRestClient := newMockRestClient(t)
	mockRestClient.EXPECT().SvmGetByName(ctx, gomock.Any()).AnyTimes()
	mockRestClient.EXPECT().SvmList(ctx, gomock.Any()).AnyTimes()
	err := api.EnsureSVMWithRest(ctx, ontapConfig, mockRestClient)
	assert.Equal(t, "cannot derive SVM to use; please specify SVM in config file; result was nil", err.Error())

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  no SVM set && we CANNOT derive an SVM because there are no matching records
	mockRestClient = newMockRestClient(t)
	mockRestClient.EXPECT().SvmGetByName(ctx, gomock.Any()).AnyTimes()
	mockRestClient.EXPECT().SvmList(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string) (*svm.SvmCollectionGetOK, error) {
			result := &svm.SvmCollectionGetOK{
				Payload: &models.SvmResponse{},
			}
			return result, nil
		},
	).AnyTimes()
	err = api.EnsureSVMWithRest(ctx, ontapConfig, mockRestClient)
	assert.Equal(t, "cannot derive SVM to use; please specify SVM in config file", err.Error())

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  no SVM set && we CAN derive an SVM
	svmName := "mySVM"
	svmUUID := svmName + "U-U-I-D"
	mockRestClient = newMockRestClient(t)
	mockRestClient.EXPECT().SvmList(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string) (*svm.SvmCollectionGetOK, error) {
			var records []*models.Svm
			records = append(records, &models.Svm{Name: svmName, UUID: svmUUID})
			result := &svm.SvmCollectionGetOK{
				Payload: &models.SvmResponse{
					NumRecords: int64(len(records)),
					Records:    records,
				},
			}
			return result, nil
		},
	).AnyTimes()
	mockRestClient.EXPECT().SetSVMUUID(gomock.Any()).DoAndReturn(
		func(newUUID string) {
			assert.Equal(t, svmUUID, newUUID) // extra validation that it is set to the new value
		},
	).AnyTimes()
	mockRestClient.EXPECT().SetSVMName(gomock.Any()).DoAndReturn(
		func(newSvmName string) {
			assert.Equal(t, svmName, newSvmName) // extra validation that it is set to the new value
		},
	).AnyTimes()
	err = api.EnsureSVMWithRest(ctx, ontapConfig, mockRestClient)
	assert.Nil(t, err)
}

func TestSanitizeDataLIF(t *testing.T) {
	cases := []struct {
		Input  string
		Output string
	}{
		{
			Input:  "127.0.0.1",
			Output: "127.0.0.1",
		},
		{
			Input:  "[2001:db8::1]",
			Output: "2001:db8::1",
		},
		{
			Input:  "[2a00:1450:400a:804::2004]",
			Output: "2a00:1450:400a:804::2004",
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("test%v", i), func(t *testing.T) {
			result := sanitizeDataLIF(c.Input)
			assert.Equal(t, c.Output, result)
		})
	}
}

func newMockZapiClient(t *testing.T) *mock_ontap.MockZapiClientInterface {
	mockCtrl := gomock.NewController(t)
	mockZapiClient := mock_ontap.NewMockZapiClientInterface(mockCtrl)
	return mockZapiClient
}

func TestZapiGetSVMAggregateSpace(t *testing.T) {
	ctx := context.Background()

	aggr := "aggr1"

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  nil result, ensure we do not panic
	mockZapiClient := newMockZapiClient(t)
	d, err := api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockZapiClient.EXPECT().AggrSpaceGetIterRequest(gomock.Any()).DoAndReturn(
		func(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
			return nil, nil
		},
	).AnyTimes()

	result, err := d.GetSVMAggregateSpace(ctx, aggr)
	assert.Equal(t, "could not determine aggregate space, cannot check aggregate provisioning limits for aggr1",
		err.Error())
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().AggrSpaceGetIterRequest(gomock.Any()).DoAndReturn(
		func(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
			result := &azgo.AggrSpaceGetIterResponse{}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().AggrSpaceGetIterRequest(gomock.Any()).DoAndReturn(
		func(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
			result := &azgo.AggrSpaceGetIterResponse{
				Result: azgo.AggrSpaceGetIterResponseResult{},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().AggrSpaceGetIterRequest(gomock.Any()).DoAndReturn(
		func(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
			result := &azgo.AggrSpaceGetIterResponse{
				Result: azgo.AggrSpaceGetIterResponseResult{
					AttributesListPtr: &azgo.AggrSpaceGetIterResponseResultAttributesList{},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().AggrSpaceGetIterRequest(gomock.Any()).DoAndReturn(
		func(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
			result := &azgo.AggrSpaceGetIterResponse{
				Result: azgo.AggrSpaceGetIterResponseResult{
					AttributesListPtr: &azgo.AggrSpaceGetIterResponseResultAttributesList{
						SpaceInformationPtr: []azgo.SpaceInformationType{},
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().AggrSpaceGetIterRequest(gomock.Any()).DoAndReturn(
		func(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
			result := &azgo.AggrSpaceGetIterResponse{
				Result: azgo.AggrSpaceGetIterResponseResult{
					AttributesListPtr: &azgo.AggrSpaceGetIterResponseResultAttributesList{
						SpaceInformationPtr: []azgo.SpaceInformationType{
							{},
						},
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the space information
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	info1 := azgo.SpaceInformationType{}
	info1.SetAggregate("aggr1")
	info1.SetAggregateSize(11689104961536)
	info1.SetVolumeFootprints(8496407527424)
	info1.SetVolumeFootprintsPercent(73)
	info1.SetUsedIncludingSnapshotReserve(9090249289728)
	info1.SetUsedIncludingSnapshotReservePercent(78)

	info2 := azgo.SpaceInformationType{}
	info2.SetAggregate("aggr1")
	info2.SetTierName("Object Store: S3Bucket")

	mockZapiClient.EXPECT().AggrSpaceGetIterRequest(gomock.Any()).DoAndReturn(
		func(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
			result := &azgo.AggrSpaceGetIterResponse{
				Result: azgo.AggrSpaceGetIterResponseResult{
					AttributesListPtr: &azgo.AggrSpaceGetIterResponseResultAttributesList{
						SpaceInformationPtr: []azgo.SpaceInformationType{
							info1,
							info2,
						},
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(result))

	aggrSpace := result[0]
	assert.Equal(t, int64(info1.AggregateSize()), aggrSpace.Size())
	assert.Equal(t, int64(info1.UsedIncludingSnapshotReserve()), aggrSpace.Used())
	assert.Equal(t, int64(info1.VolumeFootprints()), aggrSpace.Footprint())
}

func newMockRestClient(t *testing.T) *mock_ontap.MockRestClientInterface {
	mockCtrl := gomock.NewController(t)
	mockRestClient := mock_ontap.NewMockRestClientInterface(mockCtrl)
	return mockRestClient
}

func TestRestGetSVMAggregateSpace(t *testing.T) {
	ctx := context.Background()

	aggr := "aggr1"

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  nil result, ensure we do not panic
	mockRestClient := newMockRestClient(t)

	d, err := api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			return nil, nil
		},
	).AnyTimes()

	result, err := d.GetSVMAggregateSpace(ctx, aggr)
	assert.Equal(t, "error looking up aggregate: aggr1", err.Error())
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  nil Payload
	mockRestClient = newMockRestClient(t)

	d, err = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			result := &storage.AggregateCollectionGetOK{
				Payload: nil,
			}

			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Equal(t, "error looking up aggregate: aggr1", err.Error())
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  nil Payload Records
	mockRestClient = newMockRestClient(t)

	d, err = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			result := &storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					Records: nil,
				},
			}

			return result, nil
		},
	).AnyTimes()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  aggr not in list
	mockRestClient = newMockRestClient(t)

	d, err = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			result := &storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					Records: []*models.Aggregate{},
				},
			}

			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  aggr not in list
	mockRestClient = newMockRestClient(t)

	d, err = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			result := &storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					Records: []*models.Aggregate{
						{Name: "aggr2"},
					},
				},
			}

			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  aggr missing space information
	mockRestClient = newMockRestClient(t)

	d, err = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			result := &storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					Records: []*models.Aggregate{
						{
							Name:  aggr,
							Space: nil,
						},
					},
				},
			}

			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  aggr missing space information
	mockRestClient = newMockRestClient(t)

	d, err = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			result := &storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					Records: []*models.Aggregate{
						{
							Name: aggr,
							Space: &models.AggregateSpace{
								BlockStorage: nil,
							},
						},
					},
				},
			}

			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  aggr contains space information
	mockRestClient = newMockRestClient(t)

	d, err = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			result := &storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					Records: []*models.Aggregate{
						{
							Name: aggr,
							Space: &models.AggregateSpace{
								Footprint: 8496407527424,
								BlockStorage: &models.AggregateSpaceBlockStorage{
									Size:                                11689104961536,
									UsedIncludingSnapshotReserve:        9090249289728,
									UsedIncludingSnapshotReservePercent: 78,
									VolumeFootprintsPercent:             73,
								},
							},
						},
					},
				},
			}

			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  aggr contains space information and cloud tier entry
	mockRestClient = newMockRestClient(t)

	d, err = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
			result := &storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					Records: []*models.Aggregate{
						{
							Name: aggr,
							Space: &models.AggregateSpace{
								Footprint: 8496407527424,
								BlockStorage: &models.AggregateSpaceBlockStorage{
									Size:                                11689104961536,
									UsedIncludingSnapshotReserve:        9090249289728,
									UsedIncludingSnapshotReservePercent: 78,
									VolumeFootprintsPercent:             73,
								},
							},
						},
						{
							// extra entry for cloud tier
							Name: aggr,
						},
					},
				},
			}

			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSVMAggregateSpace(ctx, aggr)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(result))
}

func newMockOntapAPI(t *testing.T) *mock_ontap.MockOntapAPI {
	mockCtrl := gomock.NewController(t)
	mockOntapAPI := mock_ontap.NewMockOntapAPI(mockCtrl)
	return mockOntapAPI
}

func TestCheckAggregateLimits(t *testing.T) {
	ctx := context.Background()

	aggr := "aggr1"
	ontapConfig := *newTestOntapDriverConfig(ONTAPTEST_LOCALHOST, "443", aggr)
	ontapConfig.SVM = "svm"
	ontapConfig.LimitAggregateUsage = "95%"
	ontapConfig.Aggregate = aggr

	spaceReserve := "none"
	requestedSizeInt := 1

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  nil result
	mockOntapAPI := newMockOntapAPI(t)
	mockOntapAPI.EXPECT().GetSVMAggregateSpace(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, aggregate string) ([]api.SVMAggregateSpace, error) {
			return nil, nil
		},
	).AnyTimes()

	err := checkAggregateLimits(ctx, aggr, spaceReserve, uint64(requestedSizeInt), ontapConfig, mockOntapAPI)
	assert.Equal(t, "could not find aggregate, cannot check aggregate provisioning limits for aggr1", err.Error())

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockOntapAPI = newMockOntapAPI(t)
	mockOntapAPI.EXPECT().GetSVMAggregateSpace(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, aggregate string) ([]api.SVMAggregateSpace, error) {
			result := []api.SVMAggregateSpace{}
			return result, nil
		},
	).AnyTimes()

	err = checkAggregateLimits(ctx, aggr, spaceReserve, uint64(requestedSizeInt), ontapConfig, mockOntapAPI)
	assert.Equal(t, "could not find aggregate, cannot check aggregate provisioning limits for aggr1", err.Error())

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  values in the result
	mockOntapAPI = newMockOntapAPI(t)
	mockOntapAPI.EXPECT().GetSVMAggregateSpace(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, aggregate string) ([]api.SVMAggregateSpace, error) {
			size := int64(11689104961536)
			used := int64(9090249289728)
			footprint := used

			result := []api.SVMAggregateSpace{
				api.NewSVMAggregateSpace(size, used, footprint),
			}
			return result, nil
		},
	).AnyTimes()

	err = checkAggregateLimits(ctx, aggr, spaceReserve, uint64(requestedSizeInt), ontapConfig, mockOntapAPI)
	assert.Nil(t, err)
}

func newTestOntapDriverConfig(
	vserverAdminHost, vserverAdminPort, vserverAggrName string,
) *drivers.OntapStorageDriverConfig {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["api"] = true
	// config.Labels = map[string]string{"app": "wordpress"}
	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-san-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-san"
	config.StoragePrefix = sp("test_")

	return config
}

func TestGetEncryptionValue(t *testing.T) {
	// sending empty volume attrs
	encryption := ""
	val, err := GetEncryptionValue(encryption)
	assert.NoError(t, err)
	assert.Nil(t, val)

	// sending invalid encryption value
	encryption = "dummy"
	_, err = GetEncryptionValue(encryption)
	assert.Error(t, err)

	// sending encryption value as true
	encryption = "true"
	val, err = GetEncryptionValue(encryption)
	assert.NoError(t, err)
	assert.Equal(t, true, *val)

	// sending encryption value as false
	encryption = "false"
	val, err = GetEncryptionValue(encryption)
	assert.NoError(t, err)
	assert.Equal(t, false, *val)
}

func TestZapiGetSLMLifs(t *testing.T) {
	ctx := context.Background()

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8"}
	nodes := []string{"node1", "node2", "node3", "node4", "node5"}
	reportingNodes := []string{"node1", "node2"}
	ipToNodeMapping := map[string]string{
		ips[0]: nodes[0],
		ips[1]: nodes[0],
		ips[2]: nodes[1],
		ips[3]: nodes[1],
		ips[4]: nodes[2],
		ips[5]: nodes[2],
		ips[6]: nodes[3],
		ips[7]: nodes[3],
	}

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  nil result, ensure we do not panic
	mockZapiClient := newMockZapiClient(t)
	d, err := api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	returnErr := fmt.Errorf("some error")
	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			return nil, returnErr
		},
	).AnyTimes()

	result, err := d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Contains(t, err.Error(), returnErr.Error())
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			return nil, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: []azgo.NetInterfaceInfoType{},
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: []azgo.NetInterfaceInfoType{
							{},
						},
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result when there are no reporting nodes
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos := []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, []string{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result when the reporting nodes is nil
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos = []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, nil)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result when the ips is nil
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos = []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, nil, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result when the ips and reporting nodes are nil
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos = []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, nil, nil)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos = []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs when one of the reporting nodes has no LIF
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos = []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, append(reportingNodes, "node5"))
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs when one of the reporting nodes has no LIF
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos = []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, append(ips, "9.9.9.9"), reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs when net interface is missing IP
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos = []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	// Extra entry but without IP address
	info := azgo.NetInterfaceInfoType{}
	info.SetCurrentNode("node1")
	infos = append(infos, info)

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs when one reporting nodes is missing
	mockZapiClient = newMockZapiClient(t)
	d, _ = api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)

	infos = []azgo.NetInterfaceInfoType{}

	for ip, node := range ipToNodeMapping {
		info := azgo.NetInterfaceInfoType{}
		info.SetCurrentNode(node)
		info.SetAddress(ip)

		infos = append(infos, info)
	}

	// Extra entry but without IP address
	info = azgo.NetInterfaceInfoType{}
	info.SetAddress("1.2.3.4")
	infos = append(infos, info)

	mockZapiClient.EXPECT().NetInterfaceGet().DoAndReturn(
		func() (*azgo.NetInterfaceGetIterResponse, error) {
			result := &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: infos,
					},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, append(ips, "1.2.34"), reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})
}

func TestRestGetSLMLifs(t *testing.T) {
	ctx := context.Background()

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8"}
	nodes := []string{"node1", "node2", "node3", "node4", "node5"}
	reportingNodes := []string{"node1", "node2"}
	ipToNodeMapping := map[string]string{
		ips[0]: nodes[0],
		ips[1]: nodes[0],
		ips[2]: nodes[1],
		ips[3]: nodes[1],
		ips[4]: nodes[2],
		ips[5]: nodes[2],
		ips[6]: nodes[3],
		ips[7]: nodes[3],
	}

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  nil result, ensure we do not panic
	mockRestClient := newMockRestClient(t)
	d, err := api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)
	assert.Nil(t, err)
	assert.NotNil(t, d)

	returnErr := fmt.Errorf("some error")
	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			return nil, returnErr
		},
	).AnyTimes()

	result, err := d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Contains(t, err.Error(), returnErr.Error())
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			return nil, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: []*models.IPInterface{},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: []*models.IPInterface{},
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result when there are no reporting nodes
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos := []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, []string{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result when the reporting nodes is nil
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos = []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, nil)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result when the ips is nil
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos = []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, nil, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result when the ips and reporting nodes are nil
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos = []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, nil, nil)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(result))

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos = []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs when one of the reporting nodes has no LIF
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos = []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, append(reportingNodes, "node5"))
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs when one of the reporting nodes has no LIF
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos = []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, append(ips, "9.9.9.9"), reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs when net interface is missing IP
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos = []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	// Extra entry but without IP address
	info := &models.IPInterface{
		Location: &models.IPInterfaceLocation{
			Node: &models.IPInterfaceLocationNode{
				Name: "node1",
			},
		},
	}
	infos = append(infos, info)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, ips, reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case:  should be able to return the reporting LIFs when one reporting nodes is missing
	mockRestClient = newMockRestClient(t)
	d, _ = api.NewOntapAPIRESTFromRestClientInterface(mockRestClient)

	infos = []*models.IPInterface{}

	for ip, node := range ipToNodeMapping {
		info := &models.IPInterface{
			Location: &models.IPInterfaceLocation{
				Node: &models.IPInterfaceLocationNode{
					Name: node,
				},
			},
			IP: &models.IPInfo{
				Address: models.IPAddress(ip),
			},
		}

		infos = append(infos, info)
	}

	// Extra entry but without IP address
	info = &models.IPInterface{
		IP: &models.IPInfo{
			Address: "1.2.3.4",
		},
	}
	infos = append(infos, info)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					Records: infos,
				},
			}
			return result, nil
		},
	).AnyTimes()

	result, err = d.GetSLMDataLifs(ctx, append(ips, "1.2.3.4"), reportingNodes)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(result))

	assert.ElementsMatch(t, result, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"})
}
