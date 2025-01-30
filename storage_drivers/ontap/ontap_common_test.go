// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mock_ontap "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	ontap_storage "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils/errors"
	tridentmodels "github.com/netapp/trident/utils/models"
)

// ToIPAddressPointer takes a models.IPAddress and returns a pointer
// func ToIPAddressPointer(ipAddress models.IPAddress) *models.IPAddress {
// 	return &ipAddress
// }

func NewAPIResponse(
	_, _, status, reason, errno string,
) *api.APIResponse {
	return api.NewAPIResponse(status, reason, errno)
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
			records = append(records, &models.Svm{Name: convert.ToPtr(svmName), UUID: convert.ToPtr(svmUUID)})
			result := &svm.SvmCollectionGetOK{
				Payload: &models.SvmResponse{
					NumRecords:               convert.ToPtr(int64((len(records)))),
					SvmResponseInlineRecords: records,
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
			result := &ontap_storage.AggregateCollectionGetOK{
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
			result := &ontap_storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					AggregateResponseInlineRecords: nil,
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
			result := &ontap_storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					AggregateResponseInlineRecords: []*models.Aggregate{},
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
			result := &ontap_storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					AggregateResponseInlineRecords: []*models.Aggregate{
						{Name: convert.ToPtr("aggr2")},
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
			result := &ontap_storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					AggregateResponseInlineRecords: []*models.Aggregate{
						{
							Name:  convert.ToPtr(aggr),
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
			result := &ontap_storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					AggregateResponseInlineRecords: []*models.Aggregate{
						{
							Name: convert.ToPtr(aggr),
							Space: &models.AggregateInlineSpace{
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
			result := &ontap_storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					AggregateResponseInlineRecords: []*models.Aggregate{
						{
							Name: convert.ToPtr(aggr),
							Space: &models.AggregateInlineSpace{
								Footprint: convert.ToPtr(int64(8496407527424)),
								BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{
									Size:                                convert.ToPtr(int64(11689104961536)),
									Used:                                convert.ToPtr(int64(9090249289728)),
									UsedIncludingSnapshotReserve:        convert.ToPtr(int64(9090249289728)),
									UsedIncludingSnapshotReservePercent: convert.ToPtr(int64(78)),
									VolumeFootprintsPercent:             convert.ToPtr(int64(73)),
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

	mockRestClient.EXPECT().AggregateList(gomock.Any(), aggr, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string, fields []string) (*ontap_storage.AggregateCollectionGetOK, error) {
			result := &ontap_storage.AggregateCollectionGetOK{
				Payload: &models.AggregateResponse{
					AggregateResponseInlineRecords: []*models.Aggregate{
						{
							Name: convert.ToPtr(aggr),
							Space: &models.AggregateInlineSpace{
								Footprint: convert.ToPtr(int64(8496407527424)),
								BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{
									Size:                                convert.ToPtr(int64(11689104961536)),
									Used:                                convert.ToPtr(int64(9090249289728)),
									UsedIncludingSnapshotReserve:        convert.ToPtr(int64(9090249289728)),
									UsedIncludingSnapshotReservePercent: convert.ToPtr(int64(78)),
									VolumeFootprintsPercent:             convert.ToPtr(int64(73)),
								},
							},
						},
						{
							// extra entry for cloud tier
							Name: convert.ToPtr(aggr),
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

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  aggregate name is empty

	aggr = ""

	err = checkAggregateLimits(ctx, aggr, spaceReserve, uint64(requestedSizeInt), ontapConfig, mockOntapAPI)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  GetSVMAggregateSpace returns error

	aggr = "aggr1"
	mockOntapAPI = newMockOntapAPI(t)
	mockOntapAPI.EXPECT().GetSVMAggregateSpace(gomock.Any(), aggr).Return([]api.SVMAggregateSpace{},
		fmt.Errorf("GetSVMAggregateSpace returned error"))

	err = checkAggregateLimits(ctx, aggr, spaceReserve, uint64(requestedSizeInt), ontapConfig, mockOntapAPI)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  spaceReserve is thick (requested size is reserved)

	spaceReserve = "volume"
	ontapConfig.LimitAggregateUsage = "25%"
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

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  aggregate usage exceeds the limit

	spaceReserve = "none"
	ontapConfig.LimitAggregateUsage = "25%"
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

	assert.Error(t, err)
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
	val, configVal, err := GetEncryptionValue(encryption)
	assert.NoError(t, err)
	assert.Nil(t, val)
	assert.Zero(t, configVal)

	// sending invalid encryption value
	encryption = "dummy"
	_, configVal, err = GetEncryptionValue(encryption)
	assert.Error(t, err)
	assert.Nil(t, val)
	assert.Zero(t, configVal)

	// sending encryption value as true
	encryption = "true"
	val, configVal, err = GetEncryptionValue(encryption)
	assert.NoError(t, err)
	assert.Equal(t, true, *val)
	assert.Equal(t, "true", configVal)

	// sending encryption value as false
	encryption = "false"
	val, configVal, err = GetEncryptionValue(encryption)
	assert.NoError(t, err)
	assert.Equal(t, false, *val)
	assert.Equal(t, "false", configVal)
}

func getValidOntapNASPool() *storage.StoragePool {
	pool := &storage.StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})
	pool.SetInternalAttributes(
		map[string]string{
			SpaceReserve:    "none",
			SnapshotPolicy:  "none",
			SnapshotReserve: "0",
			UnixPermissions: "777",
			SnapshotDir:     "false",
			ExportPolicy:    "default",
			SecurityStyle:   "unix",
			Encryption:      "false",
			SplitOnClone:    "false",
			TieringPolicy:   "",
			Size:            "1Gi",
			NameTemplate:    "pool_{{.labels.clusterName}}_{{.volume.Namespace}}_{{.volume.RequestName}}",
		},
	)
	return pool
}

func getValidOntapSANPool() *storage.StoragePool {
	pool := &storage.StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})
	pool.SetInternalAttributes(
		map[string]string{
			SpaceReserve:    "none",
			SnapshotPolicy:  "none",
			SnapshotReserve: "0",
			UnixPermissions: "777",
			SnapshotDir:     "false",
			ExportPolicy:    "default",
			SecurityStyle:   "unix",
			Encryption:      "false",
			SplitOnClone:    "false",
			TieringPolicy:   "",
			Size:            "1Gi",
			SpaceAllocation: "true",
			FileSystemType:  "ext4",
		},
	)
	return pool
}

func TestValidateStoragePools_Valid_OntapNAS(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test no pools, ontap NAS
	storageDriver := newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools := map[string]storage.Pool{}
	virtualPools := map[string]storage.Pool{}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools

	err := ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 1)

	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test one valid virtual pool with NASType = NFS
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.Config.NASType = sa.NFS
	storageDriver.virtualPools["test"].InternalAttributes()[SecurityStyle] = "mixed"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test one valid virtual pool with NASType = SMB
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.Config.NASType = sa.SMB
	storageDriver.virtualPools["test"].InternalAttributes()[SecurityStyle] = "mixed"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test one valid virtual pool with invalid securityStyle
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.Config.NASType = sa.SMB
	storageDriver.virtualPools["test"].InternalAttributes()[SecurityStyle] = "invalidValue"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test two valid virtual pools
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool(), "test2": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test with snapshotpolicy empty

	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[SnapshotPolicy] = ""

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test with Invalid value of Encryption
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[Encryption] = "fakeValue"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test with empty snapshot dir
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[SnapshotDir] = ""

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test Invalid value of snapshot dir
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[SnapshotDir] = "fakeValue"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test one valid virtual with invalid value for label in pool
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, -1)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test one valid virtual with Invalid value of SecurityStyle attribute
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[SecurityStyle] = "fakeValue"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test one valid virtual with empty Export Policy
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[ExportPolicy] = ""

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test one valid virtual with Unix Permission empty
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	storageDriver.Config.NASType = sa.NFS
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[UnixPermissions] = ""

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test one valid virtual with Invalid value of Tiering policy
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[TieringPolicy] = "fakePolicy"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test one valid virtual with AdaptiveQosPolicy policy
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().SupportsFeature(ctx, api.QosPolicies).AnyTimes().Return(false)
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	storageDriver.API = mockAPI
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[AdaptiveQosPolicy] = "fake"

	err = ValidateStoragePools(ctx, physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test one valid virtual with both QOS and AdaptiveQosPolicy policy set

	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().SupportsFeature(ctx, api.QosPolicies).AnyTimes().Return(true)
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	storageDriver.API = mockAPI
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[QosPolicy] = "fake"
	storageDriver.virtualPools["test"].InternalAttributes()[AdaptiveQosPolicy] = "fake"

	err = ValidateStoragePools(ctx, physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// Negative case: Test to check that qtrees do not support adaptive QoS policies
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().SupportsFeature(ctx, api.QosPolicies).AnyTimes().Return(true)
	storageDriverQtree := newNASQtreeStorageDriver(mockAPI)
	storageDriver.API = mockAPI
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriverQtree.virtualPools = virtualPools
	storageDriverQtree.physicalPools = physicalPools
	storageDriverQtree.virtualPools["test"].InternalAttributes()[AdaptiveQosPolicy] = "fake"

	err = ValidateStoragePools(ctx, physicalPools, virtualPools, storageDriverQtree, 0)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test with media type set

	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[Media] = "hdd"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test with Invalid media type

	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[Media] = "fake"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test with less size of the pool

	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[Size] = "1000"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test with invalid value for size in pool

	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[Media] = "hdd"
	storageDriver.virtualPools["test"].InternalAttributes()[Size] = "xyz"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test with invalid value for splitOnClone

	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[SplitOnClone] = "fake"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test with empty value for splitOnClone

	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapNASPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[SplitOnClone] = ""

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test with empty value for SpaceAllocation

	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	storageDriverSAN := newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName, true, nil, mockAPI)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapSANPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[SpaceAllocation] = ""

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriverSAN, 0)

	assert.Error(t, err)

	// // ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// // Negative case: Test with Invalid value for SpaceAllocation

	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	storageDriverSAN = newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName, true, nil, mockAPI)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapSANPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[SpaceAllocation] = "fake"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriverSAN, 0)

	assert.Error(t, err)

	// // ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// // Negative case: Test with empty value for FileSystemType

	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	storageDriverSAN = newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName, true, nil, mockAPI)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapSANPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[FileSystemType] = ""

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriverSAN, 0)

	assert.Error(t, err)
	// // ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// // Negative case: Test with Invalid value for FileSystemType

	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	storageDriverSAN = newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName, true, nil, mockAPI)
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": getValidOntapSANPool()}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	storageDriver.virtualPools["test"].InternalAttributes()[FileSystemType] = "fake"

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriverSAN, 0)

	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Volume name template invalid, ontap NAS
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	pool1 := getValidOntapNASPool()
	poolAttribute := pool1.InternalAttributes()
	poolAttribute[NameTemplate] = "pool_{{.labels.Cluster}}_{{.volume.Namespac}_.volume." +
		"RequestName}}" // invalid template
	pool1.SetInternalAttributes(poolAttribute)
	pool1.SetName("test")

	virtualPools = map[string]storage.Pool{"test": pool1}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)

	assert.Error(t, err, "template is invalid, expected an error")

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: label template invalid
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	storagePool := getValidOntapNASPool()

	poolAttribute = storagePool.InternalAttributes()
	poolAttribute[NameTemplate] = "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_.volume.RequestName}}"
	storagePool.SetInternalAttributes(poolAttribute)
	storagePool.SetName("test")

	virtualPools = map[string]storage.Pool{"test": storagePool}
	virtualPools["test"].Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"template": "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName",
	})
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)
	assert.Error(t, err, "template is invalid, expected an error")

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Postive case: Name and label template valid
	storageDriver = newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)
	physicalPools = map[string]storage.Pool{}
	storagePool = getValidOntapNASPool()

	poolAttribute = storagePool.InternalAttributes()
	poolAttribute[NameTemplate] = "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_.volume.RequestName}}"
	storagePool.SetInternalAttributes(poolAttribute)
	storagePool.SetName("test")

	virtualPools = map[string]storage.Pool{"test": storagePool}
	virtualPools["test"].Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"template": "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}",
	})
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools

	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)
	assert.NoError(t, err, "template is valid, expected no error")
}

func TestValidateStoragePools_LUKS(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test one virtual pool, LUKS and ONTAP-SAN allowed
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	sanStorageDriver := newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName, false, nil, mockAPI)
	pool := getValidOntapSANPool()
	pool.InternalAttributes()[LUKSEncryption] = "true"
	physicalPools := map[string]storage.Pool{}
	virtualPools := map[string]storage.Pool{"test": pool}
	sanStorageDriver.virtualPools = virtualPools
	sanStorageDriver.physicalPools = physicalPools
	err := ValidateStoragePools(context.Background(), physicalPools, virtualPools, sanStorageDriver, 0)
	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test one virtual pool, LUKS and ONTAP-SAN-Economy allowed
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	sanEcoStorageDriver := newTestOntapSanEcoDriver(t, vserverAdminHost, "443", vserverAggrName, false, nil, mockAPI)
	pool = getValidOntapSANPool()
	pool.InternalAttributes()[LUKSEncryption] = "true"
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": pool}
	sanEcoStorageDriver.virtualPools = virtualPools
	sanEcoStorageDriver.physicalPools = physicalPools
	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, sanEcoStorageDriver, 0)
	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test one virtual pool, LUKS and NAS not allowed
	storageDriver := newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName, tridentconfig.DriverContext("CSI"),
		false, nil)
	pool = getValidOntapNASPool()
	pool.InternalAttributes()[LUKSEncryption] = "true"
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": pool}
	storageDriver.virtualPools = virtualPools
	storageDriver.physicalPools = physicalPools
	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)
	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test one virtual pool, LUKS and ONTAP-SAN allowed but invalid value
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	sanStorageDriver = newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName, false, nil, mockAPI)
	pool = getValidOntapSANPool()
	pool.InternalAttributes()[LUKSEncryption] = "invalid-not-a-bool"
	physicalPools = map[string]storage.Pool{}
	virtualPools = map[string]storage.Pool{"test": pool}
	sanStorageDriver.virtualPools = virtualPools
	sanStorageDriver.physicalPools = physicalPools
	err = ValidateStoragePools(context.Background(), physicalPools, virtualPools, sanStorageDriver, 0)
	assert.Error(t, err)
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
					IPInterfaceResponseInlineRecords: []*models.IPInterface{},
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
					IPInterfaceResponseInlineRecords: []*models.IPInterface{},
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	// Extra entry but without IP address
	info := &models.IPInterface{
		Location: &models.IPInterfaceInlineLocation{
			Node: &models.IPInterfaceInlineLocationInlineNode{
				Name: convert.ToPtr("node1"),
			},
		},
	}
	infos = append(infos, info)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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
			Location: &models.IPInterfaceInlineLocation{
				Node: &models.IPInterfaceInlineLocationInlineNode{
					Name: convert.ToPtr(node),
				},
			},
			IP: &models.IPInfo{
				Address: convert.ToPtr(models.IPAddress(ip)),
			},
		}

		infos = append(infos, info)
	}

	// Extra entry but without IP address
	info = &models.IPInterface{
		IP: &models.IPInfo{
			Address: convert.ToPtr(models.IPAddress("1.2.3.4")),
		},
	}
	infos = append(infos, info)

	mockRestClient.EXPECT().NetworkIPInterfacesList(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
			result := &networking.NetworkIPInterfacesGetOK{
				Payload: &models.IPInterfaceResponse{
					IPInterfaceResponseInlineRecords: infos,
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

func TestConstructOntapNASVolumeAccessPath(t *testing.T) {
	ctx := context.Background()

	volConfig := &storage.VolumeConfig{
		InternalName: "vol",
	}

	tests := []struct {
		smbShare     string
		volName      string
		protocol     string
		expectedPath string
	}{
		{"test_share", "/vol", "smb", "\\test_share\\vol"},
		{"", "/vol", "smb", "\\vol"},
		{"", "/vol", "nfs", "/vol"},
		{"", "/vol1", "nfs", "/vol1"},
	}

	for _, test := range tests {
		t.Run(test.smbShare, func(t *testing.T) {
			result := ConstructOntapNASVolumeAccessPath(ctx, test.smbShare, test.volName, volConfig, test.protocol)
			assert.Equal(t, test.expectedPath, result, "unable to construct Ontap-NAS volume access path")
		})
	}
}

func TestConstructOntapNASVolumeAccessPath_ROClone(t *testing.T) {
	ctx := context.Background()

	volConfig := &storage.VolumeConfig{
		InternalName:              "vol",
		ReadOnlyClone:             true,
		CloneSourceVolumeInternal: "sourceVol",
		CloneSourceSnapshot:       "snapshot-abcd-1234-wxyz",
	}

	tests := []struct {
		smbShare     string
		protocol     string
		expectedPath string
	}{
		{"test_share", "smb", "\\test_share\\sourceVol\\~snapshot\\snapshot-abcd-1234-wxyz"},
		{"", "smb", "\\sourceVol\\~snapshot\\snapshot-abcd-1234-wxyz"},
		{"", "nfs", "/sourceVol/.snapshot/snapshot-abcd-1234-wxyz"},
	}

	for _, test := range tests {
		t.Run(test.smbShare, func(t *testing.T) {
			result := ConstructOntapNASVolumeAccessPath(ctx, test.smbShare, "/vol", volConfig, test.protocol)
			assert.Equal(t, test.expectedPath, result, "unable to construct Ontap-NAS volume access path")
		})
	}
}

func TestConstructOntapNASFlexGroupSMBVolumePath(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		smbShare     string
		expectedPath string
	}{
		{"test_share", "\\test_sharevol"},
		{"", "vol"},
	}

	for _, test := range tests {
		t.Run(test.smbShare, func(t *testing.T) {
			result := ConstructOntapNASFlexGroupSMBVolumePath(ctx, test.smbShare, "vol")
			assert.Equal(t, test.expectedPath, result, "unable to construct Ontap-NAS-QTree SMB volume path")
		})
	}
}

func TestConstructOntapNASQTreeVolumePath(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		smbShare     string
		flexvol      string
		volConfig    *storage.VolumeConfig
		protocol     string
		expectedPath string
	}{
		{
			"test_share",
			"flex-vol",
			&storage.VolumeConfig{
				Name:                      "pvc-vol",
				InternalName:              "trident_pvc_vol",
				CloneSourceVolumeInternal: "cloneSourceInternal",
				CloneSourceSnapshot:       "sourceSnapShot",
				ReadOnlyClone:             false,
			},
			sa.SMB,
			"\\test_share\\flex-vol\\trident_pvc_vol",
		},
		{
			"",
			"flex-vol",
			&storage.VolumeConfig{
				Name:                      "volumeConfig",
				InternalName:              "trident_pvc_vol",
				CloneSourceVolumeInternal: "cloneSourceInternal",
				CloneSourceSnapshot:       "sourceSnapShot",
				ReadOnlyClone:             false,
			},
			sa.SMB,
			"\\flex-vol\\trident_pvc_vol",
		},
		{
			"test_share",
			"flex-vol",
			&storage.VolumeConfig{
				Name:                      "volumeConfig",
				InternalName:              "trident_pvc_vol",
				CloneSourceVolumeInternal: "cloneSourceInternal",
				CloneSourceSnapshot:       "sourceSnapShot",
				ReadOnlyClone:             true,
			},
			sa.SMB,
			"\\test_share\\flex-vol\\cloneSourceInternal\\~snapshot\\sourceSnapShot",
		},
		{
			"",
			"flex-vol",
			&storage.VolumeConfig{
				Name:                      "volumeConfig",
				InternalName:              "trident_pvc_vol",
				CloneSourceVolumeInternal: "cloneSourceInternal",
				CloneSourceSnapshot:       "sourceSnapShot",
				ReadOnlyClone:             true,
			},
			sa.SMB,
			"\\flex-vol\\cloneSourceInternal\\~snapshot\\sourceSnapShot",
		},
		{
			"test_share",
			"flex-vol",
			&storage.VolumeConfig{
				Name:                      "pvc-vol",
				InternalName:              "trident_pvc_vol",
				CloneSourceVolumeInternal: "cloneSourceInternal",
				CloneSourceSnapshot:       "sourceSnapShot",
				ReadOnlyClone:             false,
			},
			sa.NFS,
			"/flex-vol/trident_pvc_vol",
		},
		{
			"test_share",
			"flex-vol",
			&storage.VolumeConfig{
				Name:                      "pvc-vol",
				InternalName:              "trident_pvc_vol",
				CloneSourceVolumeInternal: "cloneSourceInternal",
				CloneSourceSnapshot:       "sourceSnapShot",
				ReadOnlyClone:             true,
			},
			sa.NFS,
			"/flex-vol/cloneSourceInternal/.snapshot/sourceSnapShot",
		},
	}

	for _, test := range tests {
		t.Run(test.smbShare, func(t *testing.T) {
			result := ConstructOntapNASQTreeVolumePath(ctx, test.smbShare, "flex-vol", test.volConfig, test.protocol)
			assert.Equal(t, test.expectedPath, result, "unable to construct Ontap-NAS-QTree SMB volume path")
		})
	}
}

func TestEnsureNodeAccess(t *testing.T) {
	// Test 1 - Positive flow

	ctx := context.Background()
	ontapConfig := newOntapStorageDriverConfig()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().ExportPolicyExists(ctx, "trident-fakeUUID").AnyTimes().Return(true, nil)
	volInfo := &tridentmodels.VolumePublishInfo{
		BackendUUID: "fakeUUID",
	}

	err := ensureNodeAccess(ctx, volInfo, mockAPI, ontapConfig)

	assert.NoError(t, err)

	// Test 2 - Test When policy doesn't exists

	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockRestClient := newMockRestClient(t)
	mockRestClient.EXPECT().SvmGetByName(ctx, gomock.Any()).AnyTimes()
	mockRestClient.EXPECT().SvmList(ctx, gomock.Any()).AnyTimes()
	mockAPI.EXPECT().ExportPolicyExists(ctx, "trident-fakeUUID").AnyTimes().Return(false, nil)

	volInfo = &tridentmodels.VolumePublishInfo{
		BackendUUID: "fakeUUID",
	}

	err = ensureNodeAccess(ctx, volInfo, mockAPI, ontapConfig)

	assert.NoError(t, err)

	// Test-3, Negative Test when ExportPolicyExists returns error

	mockRestClient = newMockRestClient(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockRestClient.EXPECT().SvmGetByName(ctx, gomock.Any()).AnyTimes()
	mockRestClient.EXPECT().SvmList(ctx, gomock.Any()).AnyTimes()
	mockAPI.EXPECT().ExportPolicyExists(ctx, "trident-fakeUUID").AnyTimes().Return(false,
		fmt.Errorf("Error returned while checking policy"))

	volInfo = &tridentmodels.VolumePublishInfo{
		BackendUUID: "fakeUUID",
	}

	err = ensureNodeAccess(ctx, volInfo, mockAPI, ontapConfig)

	assert.Error(t, err)
}

func TestDeleteExportPolicy(t *testing.T) {
	// Test-1: Positive flow

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().ExportPolicyDestroy(ctx, gomock.Any()).Return(nil)

	err := deleteExportPolicy(ctx, "fakePolicy", mockAPI)

	assert.NoError(t, err)

	// Test-2: Error flow

	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().ExportPolicyDestroy(ctx, gomock.Any()).Return(fmt.Errorf("Error while destroying policy"))

	err = deleteExportPolicy(ctx, "fakePolicy", mockAPI)

	assert.Error(t, err)
}

func TestEnsureExportPolicyExists(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, gomock.Any()).Return(nil)

	err := ensureExportPolicyExists(ctx, "fakePolicy", mockAPI)

	assert.NoError(t, err)
}

func TestReconcileExportPolicyRules(t *testing.T) {
	ctx := context.Background()

	tt := []struct {
		name    string
		mocks   func(mockAPI *mockapi.MockOntapAPI, policyName, nasType string, desiredRules []string)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "PositiveFlow",
			mocks: func(mockAPI *mockapi.MockOntapAPI, policyName, nasType string, desiredRules []string) {
				ruleList := make(map[string]int)
				ruleList["0.0.0.1/0"] = 0
				ruleList["::/0"] = 1
				mockAPI.EXPECT().ExportRuleList(ctx, policyName).
					Return(ruleList, fmt.Errorf("fake error extracting rules"))
				mockAPI.EXPECT().ExportRuleCreate(ctx, policyName, desiredRules[0], nasType).Return(nil)
				mockAPI.EXPECT().ExportRuleDestroy(ctx, policyName, ruleList["0.0.0.1/0"]).Return(nil)
			},
			wantErr: assert.NoError,
		},
		{
			name: "ErrorCreatingExportRule",
			mocks: func(mockAPI *mockapi.MockOntapAPI, policyName, nasType string, desiredRules []string) {
				ruleList := make(map[string]int)
				ruleList["0.0.0.1/0"] = 0
				ruleList["::/0"] = 1
				mockAPI.EXPECT().ExportRuleList(ctx, policyName).Return(ruleList, nil)
				mockAPI.EXPECT().ExportRuleCreate(ctx, policyName, desiredRules[0],
					nasType).Return(fmt.Errorf("Error creating export rule"))
			},
			wantErr: assert.Error,
		},
		{
			name: "ErrorDestroyingExportRule",
			mocks: func(mockAPI *mockapi.MockOntapAPI, policyName, nasType string, desiredRules []string) {
				ruleList := make(map[string]int)
				ruleList["0.0.0.1/0"] = 0
				ruleList["::/0"] = 1
				mockAPI.EXPECT().ExportRuleList(ctx, policyName).Return(ruleList, nil)
				mockAPI.EXPECT().ExportRuleCreate(ctx, policyName, desiredRules[0], nasType).Return(nil)
				mockAPI.EXPECT().ExportRuleDestroy(ctx, policyName, ruleList["0.0.0.1/0"]).Return(fmt.Errorf(
					"Error destroying export rule"))
			},
			wantErr: assert.Error,
		},
		{
			name: "RuleAlreadyExistError",
			mocks: func(mockAPI *mockapi.MockOntapAPI, policyName, nasType string, desiredRules []string) {
				ruleList := make(map[string]int)
				ruleList["0.0.0.1/0"] = 0
				ruleList["::/0"] = 1
				mockAPI.EXPECT().ExportRuleList(ctx, policyName).Return(ruleList, nil)
				mockAPI.EXPECT().ExportRuleCreate(ctx, policyName, desiredRules[0], nasType).
					Return(errors.AlreadyExistsError("Rule already exists"))
				mockAPI.EXPECT().ExportRuleDestroy(ctx, policyName, ruleList["0.0.0.1/0"]).Return(nil)
			},
			wantErr: assert.NoError,
		},
		{
			name: "MatchZAPIRuleFormat",
			mocks: func(mockAPI *mockapi.MockOntapAPI, policyName, nasType string, desiredRules []string) {
				ruleList := make(map[string]int)
				ruleList["0.0.0.1/0,0.0.0.0/0"] = 0
				ruleList["::/0"] = 1
				ruleList["1.1.1.1/0"] = 2
				mockAPI.EXPECT().ExportRuleList(ctx, policyName).Return(ruleList, nil)
				mockAPI.EXPECT().ExportRuleDestroy(ctx, policyName, ruleList["1.1.1.1/0"]).Return(nil)
			},
			wantErr: assert.NoError,
		},
	}

	for _, tr := range tt {
		t.Run(tr.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
			commonConfig := &drivers.CommonStorageDriverConfig{
				DebugTraceFlags: map[string]bool{"method": true},
			}
			config := &drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: commonConfig,
			}
			config.NASType = sa.SMB
			desiredRules := []string{"0.0.0.0/0", "::/0"}
			tr.mocks(mockAPI, "dummyPolicy", config.NASType, desiredRules)

			err := reconcileExportPolicyRules(ctx, "dummyPolicy", desiredRules, mockAPI, config)
			if !tr.wantErr(t, err, "Unexpected Result") {
				return
			}
		})
	}
}

func TestIsDefaultAuthTypeOfType(t *testing.T) {
	response := api.IscsiInitiatorAuth{
		AuthType: "fakeAuthType",
	}

	actual := isDefaultAuthTypeOfType(response, "fakeAuthType")

	assert.True(t, actual)
}

func TestIsDefaultAuthTypeNone(t *testing.T) {
	// Test1: AuthType is "none"
	response := api.IscsiInitiatorAuth{
		AuthType: "none",
	}

	actual := IsDefaultAuthTypeNone(response)

	assert.True(t, actual)

	// Test2: AuthType is not "none"
	response = api.IscsiInitiatorAuth{
		AuthType: "chap",
	}

	actual = IsDefaultAuthTypeNone(response)

	assert.False(t, actual)

	// Test3: AuthType field empty
	response = api.IscsiInitiatorAuth{}

	actual = IsDefaultAuthTypeNone(response)

	assert.False(t, actual)
}

func TestIsDefaultAuthTypeCHAP(t *testing.T) {
	// Test1: AuthType is "chap"
	response := api.IscsiInitiatorAuth{
		AuthType: "CHAP",
	}

	actual := IsDefaultAuthTypeCHAP(response)

	assert.True(t, actual)

	// Test2: AuthType is not "chap"
	response = api.IscsiInitiatorAuth{
		AuthType: "none",
	}

	actual = IsDefaultAuthTypeCHAP(response)

	assert.False(t, actual)

	// Test3: AuthType field empty
	response = api.IscsiInitiatorAuth{}

	actual = IsDefaultAuthTypeCHAP(response)

	assert.False(t, actual)
}

func TestIsDefaultAuthTypeDeny(t *testing.T) {
	// Test1: AuthType is "chap"
	response := api.IscsiInitiatorAuth{
		AuthType: "deny",
	}

	actual := IsDefaultAuthTypeDeny(response)

	assert.True(t, actual)

	// Test2: AuthType is not "deny"
	response = api.IscsiInitiatorAuth{
		AuthType: "chap",
	}

	actual = IsDefaultAuthTypeDeny(response)

	assert.False(t, actual)

	// Test3: AuthType field empty
	response = api.IscsiInitiatorAuth{}

	actual = IsDefaultAuthTypeDeny(response)

	assert.False(t, actual)
}

func mockValidate(_ context.Context) error {
	return nil
}

func mockValidate_Error(_ context.Context) error {
	return fmt.Errorf("Error while validating")
}

func TestInitializeSANDriver(t *testing.T) {
	// Test-1: Positive flow
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		UseCHAP:                   true,
		IgroupName:                "",
		SVM:                       "testSVM",
		Username:                  "testUsername",
		Password:                  "testPassword",
		ChapUsername:              "chapUsername",
		ChapTargetUsername:        "chapTargetUsername",
		ChapInitiatorSecret:       "chapInitiatorSecret",
		ChapTargetInitiatorSecret: "chapTargetInitiatorSecret",
	}

	config.SANType = sa.ISCSI

	response := api.IscsiInitiatorAuth{
		AuthType: "none",
	}
	lun := []api.Lun{}
	backendUUID := "testBackendUUID"
	driverContext := tridentconfig.ContextCSI
	expectedIgroupName := getDefaultIgroupName(driverContext, backendUUID)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(response, nil)
	mockAPI.EXPECT().LunList(ctx, "*").Return(lun, nil)
	mockAPI.EXPECT().IscsiInitiatorSetDefaultAuth(ctx, "CHAP", config.ChapUsername, config.ChapInitiatorSecret,
		config.ChapTargetUsername, config.ChapTargetInitiatorSecret).Return(nil)

	err := InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.NoError(t, err)

	// Test-2: UseChap is false
	config.UseCHAP = false
	response.AuthType = "deny"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(response, nil)

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.Error(t, err)

	// Test-3: Testing error flow : IscsiInitiatorSetDefaultAuth returns error
	config.UseCHAP = true
	response.AuthType = "none"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(response, nil)
	mockAPI.EXPECT().LunList(ctx, "*").Return(lun, nil)
	mockAPI.EXPECT().IscsiInitiatorSetDefaultAuth(ctx, "CHAP", config.ChapUsername, config.ChapInitiatorSecret,
		config.ChapTargetUsername, config.ChapTargetInitiatorSecret).Return(fmt.Errorf("Error setting default auth"))

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.Error(t, err)

	// Test-4: Testing error flow : error with CHAP credentials
	config.UseCHAP = true
	response.AuthType = "deny"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(response, nil)

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.Error(t, err)

	// Test-5: Testing error flow : LunList returns error
	config.UseCHAP = true
	response.AuthType = "none"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(response, nil)
	mockAPI.EXPECT().LunList(ctx, "*").Return(lun, fmt.Errorf("error enumerating LUNs"))

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.Error(t, err)

	// Test-6: Testing error flow : Error enabling chap for exiting luns
	dummyLun := api.Lun{
		Comment:      "dummyLun",
		SerialNumber: "testSerialNumber",
	}
	config.UseCHAP = true
	response.AuthType = "none"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(response, nil)
	mockAPI.EXPECT().LunList(ctx, "*").Return([]api.Lun{dummyLun}, nil)

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.Error(t, err)

	// Test-7: Testing error flow : validate retuns error
	config.UseCHAP = true
	response.AuthType = "none"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate_Error, backendUUID)

	assert.Error(t, err)

	// Test-8: Testing error flow : LunList returns error
	config.UseCHAP = true
	response.AuthType = "none"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi",
		"linux").Return(fmt.Errorf("ensureIGroupExists returned error"))

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.Error(t, err)

	// Test-9: Testing error flow : IscsiInitiatorGetDefaultAuth returns error
	config.UseCHAP = true
	response.AuthType = "none"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(response, fmt.Errorf("error getting default auth"))

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.Error(t, err)

	// Test-10: Do not enable CHAP if any LUNs already exisit
	config.UseCHAP = true
	response.AuthType = "none"
	mockCtrl = gomock.NewController(t)
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IgroupCreate(ctx, expectedIgroupName, "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(response, nil)
	mockAPI.EXPECT().LunList(ctx, "*").Return([]api.Lun{dummyLun}, nil)

	err = InitializeSANDriver(ctx, driverContext, mockAPI, config, mockValidate, backendUUID)

	assert.Error(t, err)
	assert.Equal(t, "will not enable CHAP for SVM testSVM; 1 existing LUNs would lose access", err.Error())
}

func TestEMSHeartbeat(t *testing.T) {
	ctx := context.Background()
	mockAPI, driver := newMockOntapNASDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	driver.telemetry = &Telemetry{
		Plugin: driver.Name(),
		SVM:    "SVM1",
		Driver: driver,
		done:   make(chan struct{}),
	}
	driver.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
	driver.telemetry.TridentBackendUUID = BackendUUID
	hostname, _ := os.Hostname()
	message, _ := json.Marshal(driver.GetTelemetry())
	mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-nas", "1", false, "heartbeat", hostname, string(message), 1,
		"trident", 5).AnyTimes()

	EMSHeartbeat(ctx, driver)
}

func TestLunUnmapAllIgroups(t *testing.T) {
	// Test-1 : Positive flow
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	lunpath := "fakelunPath"
	igroups := []string{"iGroup1", "iGroup2"}
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, lunpath).Return([]string{"iGroup1"}, nil)
	mockAPI.EXPECT().LunUnmap(ctx, igroups[0], lunpath).Return(nil)

	err := LunUnmapAllIgroups(ctx, mockAPI, lunpath)

	assert.NoError(t, err)

	// Test-2 : Testing Error flow: LunListIgroupsMapped returns error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	lunpath = "fakelunPath"
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, lunpath).Return([]string{"iGroup1"},
		fmt.Errorf("Error in LunListIgroupsMapped"))

	err = LunUnmapAllIgroups(ctx, mockAPI, lunpath)

	assert.Error(t, err)

	// Test-3: Testing error flow: LunUnmap returns error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	lunpath = "fakelunPath"
	igroups = []string{"iGroup1", "iGroup2"}
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, lunpath).Return([]string{"iGroup1"}, nil)
	mockAPI.EXPECT().LunUnmap(ctx, igroups[0], lunpath).Return(fmt.Errorf("Error in LunUnmap"))

	err = LunUnmapAllIgroups(ctx, mockAPI, lunpath)

	assert.Error(t, err)
}

func TestGetVolumeSnapshot(t *testing.T) {
	// Test-1: Positive flow
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	snapConfig := &storage.SnapshotConfig{
		VolumeInternalName: "fakeVolInternalName",
		InternalName:       "fakeInternalName",
	}

	expectedSnapConfig := &storage.SnapshotConfig{
		VolumeInternalName: "fakeVolInternalName",
		InternalName:       "fakeInternalName",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}

	dummySnapshot := api.Snapshot{
		Name:       "fakeInternalName",
		CreateTime: "dummyTime",
	}
	mockAPI.EXPECT().LunSize(ctx, "fakeVolInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(dummySnapshot, nil)
	expectedSnap := &storage.Snapshot{
		Config:    expectedSnapConfig,
		Created:   "dummyTime",
		SizeBytes: 100,
		State:     "online",
	}

	snap, err := getVolumeSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.NoError(t, err, "Found error when expected none")
	assert.NotNil(t, snap, "Found no snap when expected one")
	assert.Equal(t, expectedSnap.Config.InternalName, snap.Config.InternalName, "Snaps do not match")

	// Test-2: Testing error flow: Snap not found
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	dummySnapshot = api.Snapshot{
		Name:       "wrongSnapshotName",
		CreateTime: "dummyTime",
	}
	mockAPI.EXPECT().LunSize(ctx, "fakeVolInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName).Return(
		dummySnapshot,
		errors.NotFoundError(fmt.Sprintf("snapshot %v not found for volume %v", snapConfig.InternalName,
			snapConfig.VolumeInternalName)))

	snap, err = getVolumeSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Nil(t, snap, "Found snap when expected none")
	assert.NoError(t, err, "Found error when expected none")

	// Test-3: Testing Error flow: LunSize returned error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().LunSize(ctx, "fakeVolInternalName").Return(100, fmt.Errorf("LunSize returned error"))

	_, err = getVolumeSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Error(t, err, "Found error when expected none")

	// Test-4: Testing Error flow: VolumeSnapshotList returned error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().LunSize(ctx, "fakeVolInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(dummySnapshot,
		fmt.Errorf("Error returned"))

	_, err = getVolumeSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Error(t, err, "Found error when expected none")

	// Test-5: Testing Error flow: LunSize returned Not found error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().LunSize(ctx, "fakeVolInternalName").Return(0,
		errors.NotFoundError("LunSize returned error"))

	_, err = getVolumeSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Error(t, err, "Expected LUN not found error, got none")
}

func TestGetVolumeSnapshotList(t *testing.T) {
	// Test-1: Positive flow
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}

	dummySnapshot := api.Snapshot{
		Name:       "dummySnap",
		CreateTime: "dummyTime",
	}
	snapshotsList := []api.Snapshot{dummySnapshot}
	mockAPI.EXPECT().LunSize(ctx, "fakeInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotList(ctx, volConfig.InternalName).Return(snapshotsList, nil)
	expected := 1
	snap, err := getVolumeSnapshotList(ctx, volConfig, config, mockAPI, mockAPI.LunSize)

	assert.NoError(t, err, "Found error when expected none")

	assert.Equal(t, expected, len(snap))

	// Test-2: Testing error flow : VolumeSnapshotList returned error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().LunSize(ctx, "fakeInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotList(ctx, volConfig.InternalName).Return(snapshotsList,
		fmt.Errorf("Error returned from VolumeSnapshotList"))

	_, err = getVolumeSnapshotList(ctx, volConfig, config, mockAPI, mockAPI.LunSize)

	assert.Error(t, err, "Found error when expected none")

	// Test-3: Testing Error flow: LunSize returned error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().LunSize(ctx, "fakeInternalName").Return(100, fmt.Errorf("LunSize returned error"))

	_, err = getVolumeSnapshotList(ctx, volConfig, config, mockAPI, mockAPI.LunSize)

	assert.Error(t, err, "Found error when expected none")
}

func TestGetDesiredExportPolicyRules(t *testing.T) {
	inputIPs := []string{
		"1.1.1.1", "2.2.2.2", "3.3.3.3",
	}

	inputCIDRs := []string{"0.0.0.0/0"}

	ctx := context.Background()

	config := &drivers.OntapStorageDriverConfig{
		AutoExportCIDRs: inputCIDRs,
	}

	node := tridentmodels.Node{
		IPs: inputIPs,
	}

	nodeList := []*tridentmodels.Node{&node}

	_, err := getDesiredExportPolicyRules(ctx, nodeList, config)

	assert.NoError(t, err, "Found error when expected none")
}

func TestReconcileNASNodeAccess(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	inputIPs := []string{
		"1.1.1.1",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AutoExportPolicy:          true,
	}

	node := tridentmodels.Node{
		IPs: inputIPs,
	}
	nodeList := []*tridentmodels.Node{&node}

	policyName := "fakePolicy"

	ruleMap := make(map[string]int)
	ruleMap["1.1.1.1"] = 1
	error := fmt.Errorf("Error returned")

	// Test1: Poitive flow
	mockAPI.EXPECT().ExportPolicyCreate(ctx, policyName).Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, policyName).Return(ruleMap, nil)
	mockAPI.EXPECT().ExportRuleDestroy(ctx, policyName, ruleMap["1.1.1.1"]).Return(nil)

	err := reconcileNASNodeAccess(ctx, nodeList, config, mockAPI, policyName)

	assert.NoError(t, err)

	// Test2: Error flow: Policy not found
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, policyName).AnyTimes().Return(error)

	err = reconcileNASNodeAccess(ctx, nodeList, config, mockAPI, policyName)

	assert.Error(t, err)

	// Test3: Error flow: unable to determine desired export policy rules
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	config.AutoExportCIDRs = []string{"192.168.1.0/35"}
	mockAPI.EXPECT().ExportPolicyCreate(ctx, policyName).Return(nil)

	err = reconcileNASNodeAccess(ctx, nodeList, config, mockAPI, policyName)

	assert.Error(t, err)

	// Test4: Error flow: unabled to reconcile export policy rules
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	config.AutoExportCIDRs = []string{}
	mockAPI.EXPECT().ExportPolicyCreate(ctx, policyName).Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, policyName).Return(ruleMap, nil)
	mockAPI.EXPECT().ExportRuleDestroy(ctx, policyName, ruleMap["1.1.1.1"]).AnyTimes().Return(error)

	err = reconcileNASNodeAccess(ctx, nodeList, config, mockAPI, policyName)

	assert.Error(t, err)
}

func TestReconcileSANNodeAccess(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	backendUUID := "1234"
	tridentUUID := "4321"

	// Test reconcile destroys unused igroups
	existingIgroups := []string{"netappdvp", "node1-" + tridentUUID, "node2-" + tridentUUID, "trident-" + backendUUID}
	nodesInUse := []string{"node2"}
	mockAPI.EXPECT().IgroupList(ctx).Return(existingIgroups, nil)
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, existingIgroups[1])
	mockAPI.EXPECT().IgroupDestroy(ctx, existingIgroups[1])
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, existingIgroups[3])
	mockAPI.EXPECT().IgroupDestroy(ctx, existingIgroups[3])

	err := reconcileSANNodeAccess(ctx, mockAPI, nodesInUse, backendUUID, tridentUUID)
	assert.NoError(t, err)
}

func TestFilterUnusedTridentIgroups(t *testing.T) {
	const backendUUID = "1234"
	const tridentUUID = "4321"

	tests := []struct {
		name     string
		igroups  []string
		nodes    []string
		expected []string
	}{
		{
			name:     "all nodes used with per-backend",
			igroups:  []string{"node1-" + tridentUUID, "node2-" + tridentUUID, "trident-" + backendUUID},
			nodes:    []string{"node1", "node2"},
			expected: []string{"trident-" + backendUUID},
		},
		{
			name:     "all nodes used without per-backend",
			igroups:  []string{"node1-" + tridentUUID, "node2-" + tridentUUID},
			nodes:    []string{"node1", "node2"},
			expected: []string{},
		},
		{
			name:     "no nodes used with per-backend",
			igroups:  []string{"node1-" + tridentUUID, "node2-" + tridentUUID, "trident-" + backendUUID},
			expected: []string{"node1-" + tridentUUID, "node2-" + tridentUUID, "trident-" + backendUUID},
		},
		{
			name:     "some nodes used with per-backend and non-Trident igroups",
			igroups:  []string{"netappdvp", "node1-" + tridentUUID, "node2-" + tridentUUID, "trident-" + backendUUID},
			nodes:    []string{"node2"},
			expected: []string{"node1-" + tridentUUID, "trident-" + backendUUID},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := filterUnusedTridentIgroups(test.igroups, test.nodes, backendUUID, tridentUUID)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGetPoolsForCreate(t *testing.T) {
	ctx := context.Background()

	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}
	backend := &storage.StorageBackend{}
	backend.SetName("dummybackend")
	backend.SetOnline(true)

	pool := storage.NewStoragePool(nil, "dummyPool")
	pool.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool.SetBackend(backend)

	pool1 := storage.NewStoragePool(nil, "dummyPool1")
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool1.SetBackend(backend)

	pool2 := storage.NewStoragePool(nil, "dummyPool2")
	pool2.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool2.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool2.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool2.SetBackend(backend)

	backend.SetStorage(map[string]storage.Pool{"pool": pool, "pool1": pool1, "pool2": pool2})
	physicalPools := map[string]storage.Pool{"dummyPool1": pool1, "dummyPool2": pool2}
	virtualPools := map[string]storage.Pool{"dummyPool": pool}
	storagePool := pool
	volAttributes := map[string]sa.Request{}

	_, err := getPoolsForCreate(ctx, volConfig, storagePool, volAttributes, physicalPools, virtualPools)

	assert.NoError(t, err)
}

func TestGetPoolsForCreate_NoMatchingPools(t *testing.T) {
	ctx := context.Background()

	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}
	backend := &storage.StorageBackend{}
	backend.SetName("dummybackend")
	backend.SetOnline(true)

	pool := storage.NewStoragePool(nil, "dummyPool")
	pool.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool.SetBackend(backend)

	backend.SetStorage(map[string]storage.Pool{"pool": pool})
	physicalPools := map[string]storage.Pool{}
	virtualPools := map[string]storage.Pool{"dummyPool": pool}
	storagePool := pool

	req1 := sa.NewStringRequest("dummy")
	req2 := sa.NewStringRequest("fake")
	volAttributes := map[string]sa.Request{"req1": req1, "req2": req2}

	_, err := getPoolsForCreate(ctx, volConfig, storagePool, volAttributes, physicalPools, virtualPools)

	assert.Error(t, err)
}

func TestGetVolumeOptsCommon_Thin(t *testing.T) {
	ctx := context.Background()
	volConfig := &storage.VolumeConfig{
		Name:              "fakeVolName",
		InternalName:      "fakeInternalName",
		SnapshotPolicy:    "fakeSnapPoliy",
		SnapshotReserve:   "fakeSnapReserve",
		UnixPermissions:   "fakePermissions",
		SnapshotDir:       "fakeSnapDir",
		ExportPolicy:      "fakeExportPolicy",
		SpaceReserve:      "fakeSpaceReserve",
		SecurityStyle:     "fakeSecurityStyle",
		SplitOnClone:      "fakeSplitOnClone",
		FileSystem:        "fakeFilesystem",
		Encryption:        "fakeEncryption",
		QosPolicy:         "fakeQoSPolicy",
		AdaptiveQosPolicy: "fakeAdaptiveQosPolicy",
	}

	// Provisioning type "thin"
	requests := map[string]sa.Request{
		sa.IOPS:             sa.NewIntRequest(40),
		sa.Snapshots:        sa.NewBoolRequest(true),
		sa.ProvisioningType: sa.NewStringRequest("thin"),
		sa.Encryption:       sa.NewBoolRequest(true),
	}

	opts := getVolumeOptsCommon(ctx, volConfig, requests)

	assert.NotEqual(t, 0, len(opts), "No Volume Options returned")
}

func TestGetVolumeOptsCommon_Thick(t *testing.T) {
	ctx := context.Background()
	volConfig := &storage.VolumeConfig{
		Name:              "fakeVolName",
		InternalName:      "fakeInternalName",
		SnapshotPolicy:    "fakeSnapPoliy",
		SnapshotReserve:   "fakeSnapReserve",
		UnixPermissions:   "fakePermissions",
		SnapshotDir:       "fakeSnapDir",
		ExportPolicy:      "fakeExportPolicy",
		SpaceReserve:      "fakeSpaceReserve",
		SecurityStyle:     "fakeSecurityStyle",
		SplitOnClone:      "fakeSplitOnClone",
		FileSystem:        "fakeFilesystem",
		Encryption:        "fakeEncryption",
		QosPolicy:         "fakeQoSPolicy",
		AdaptiveQosPolicy: "fakeAdaptiveQosPolicy",
	}

	// Provisioning type "thick"
	requests := map[string]sa.Request{
		sa.IOPS:             sa.NewIntRequest(40),
		sa.Snapshots:        sa.NewBoolRequest(true),
		sa.ProvisioningType: sa.NewStringRequest("thick"),
		sa.Encryption:       sa.NewBoolRequest(true),
	}

	opts := getVolumeOptsCommon(ctx, volConfig, requests)

	assert.NotEqual(t, 0, len(opts), "No Volume Options returned")
}

func TestGetVolumeOptsCommon_InvalidProvisioning(t *testing.T) {
	ctx := context.Background()
	volConfig := &storage.VolumeConfig{
		Name:              "fakeVolName",
		InternalName:      "fakeInternalName",
		SnapshotPolicy:    "fakeSnapPoliy",
		SnapshotReserve:   "fakeSnapReserve",
		UnixPermissions:   "fakePermissions",
		SnapshotDir:       "fakeSnapDir",
		ExportPolicy:      "fakeExportPolicy",
		SpaceReserve:      "fakeSpaceReserve",
		SecurityStyle:     "fakeSecurityStyle",
		SplitOnClone:      "fakeSplitOnClone",
		FileSystem:        "fakeFilesystem",
		Encryption:        "fakeEncryption",
		QosPolicy:         "fakeQoSPolicy",
		AdaptiveQosPolicy: "fakeAdaptiveQosPolicy",
	}

	// Provisioning type "InvalidProvisioningString"
	requests := map[string]sa.Request{
		sa.IOPS:             sa.NewIntRequest(40),
		sa.Snapshots:        sa.NewBoolRequest(true),
		sa.ProvisioningType: sa.NewStringRequest("InvalidProvisioning"),
		sa.Encryption:       sa.NewBoolRequest(true),
	}

	opts := getVolumeOptsCommon(ctx, volConfig, requests)

	assert.NotEqual(t, 0, len(opts), "No Volume Options returned")
}

func TestGetVolumeOptsCommon_InvalidProvisioningType(t *testing.T) {
	ctx := context.Background()
	volConfig := &storage.VolumeConfig{
		Name:              "fakeVolName",
		InternalName:      "fakeInternalName",
		SnapshotPolicy:    "fakeSnapPoliy",
		SnapshotReserve:   "fakeSnapReserve",
		UnixPermissions:   "fakePermissions",
		SnapshotDir:       "fakeSnapDir",
		ExportPolicy:      "fakeExportPolicy",
		SpaceReserve:      "fakeSpaceReserve",
		SecurityStyle:     "fakeSecurityStyle",
		SplitOnClone:      "fakeSplitOnClone",
		FileSystem:        "fakeFilesystem",
		Encryption:        "fakeEncryption",
		QosPolicy:         "fakeQoSPolicy",
		AdaptiveQosPolicy: "fakeAdaptiveQosPolicy",
	}

	// Provisioning type "InvalidProvisioningType"
	requests := map[string]sa.Request{
		sa.IOPS:             sa.NewIntRequest(40),
		sa.Snapshots:        sa.NewBoolRequest(true),
		sa.Encryption:       sa.NewBoolRequest(true),
		sa.ProvisioningType: sa.NewIntRequest(5555),
	}

	opts := getVolumeOptsCommon(ctx, volConfig, requests)

	assert.NotEqual(t, 0, len(opts), "No Volume Options returned")
}

func TestGetVolumeOptsCommon_Encryption_False(t *testing.T) {
	ctx := context.Background()
	volConfig := &storage.VolumeConfig{
		Name:              "fakeVolName",
		InternalName:      "fakeInternalName",
		SnapshotPolicy:    "fakeSnapPoliy",
		SnapshotReserve:   "fakeSnapReserve",
		UnixPermissions:   "fakePermissions",
		SnapshotDir:       "fakeSnapDir",
		ExportPolicy:      "fakeExportPolicy",
		SpaceReserve:      "fakeSpaceReserve",
		SecurityStyle:     "fakeSecurityStyle",
		SplitOnClone:      "fakeSplitOnClone",
		FileSystem:        "fakeFilesystem",
		Encryption:        "fakeEncryption",
		QosPolicy:         "fakeQoSPolicy",
		AdaptiveQosPolicy: "fakeAdaptiveQosPolicy",
	}

	// "Encryption: "false"
	requests := map[string]sa.Request{
		sa.IOPS:             sa.NewIntRequest(40),
		sa.Snapshots:        sa.NewBoolRequest(true),
		sa.Encryption:       sa.NewBoolRequest(false),
		sa.ProvisioningType: sa.NewStringRequest("thin"),
	}

	opts := getVolumeOptsCommon(ctx, volConfig, requests)

	assert.NotEqual(t, 0, len(opts), "No Volume Options returned")
}

func TestGetVolumeOptsCommon_InvalidEncryptionType(t *testing.T) {
	ctx := context.Background()
	volConfig := &storage.VolumeConfig{
		Name:              "fakeVolName",
		InternalName:      "fakeInternalName",
		SnapshotPolicy:    "fakeSnapPoliy",
		SnapshotReserve:   "fakeSnapReserve",
		UnixPermissions:   "fakePermissions",
		SnapshotDir:       "fakeSnapDir",
		ExportPolicy:      "fakeExportPolicy",
		SpaceReserve:      "fakeSpaceReserve",
		SecurityStyle:     "fakeSecurityStyle",
		SplitOnClone:      "fakeSplitOnClone",
		FileSystem:        "fakeFilesystem",
		Encryption:        "fakeEncryption",
		QosPolicy:         "fakeQoSPolicy",
		AdaptiveQosPolicy: "fakeAdaptiveQosPolicy",
	}

	// Encryption: "InvalidEncryptionType"
	requests := map[string]sa.Request{
		sa.IOPS:             sa.NewIntRequest(40),
		sa.Snapshots:        sa.NewBoolRequest(true),
		sa.Encryption:       sa.NewIntRequest(5555),
		sa.ProvisioningType: sa.NewStringRequest("thin"),
	}

	opts := getVolumeOptsCommon(ctx, volConfig, requests)

	assert.NotEqual(t, 0, len(opts))
}

func TestGetVolumeOptsCommon_DifferentSnapshotDir(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name                string
		inputSnapshotDir    string
		expectedSnapshotDir string
	}{
		{"Default snapshotDir", "", ""},
		{"Valid uppercase snapshotDir", "TRUE", "true"},
		{"Valid lowercase snapshotDir", "false", "false"},
		{"Invalid snapshotDir", "fakeSnapDir", "fakeSnapDir"},
	}

	volConfig := &storage.VolumeConfig{
		Name:              "fakeVolName",
		InternalName:      "fakeInternalName",
		SnapshotPolicy:    "fakeSnapPoliy",
		SnapshotReserve:   "fakeSnapReserve",
		UnixPermissions:   "fakePermissions",
		ExportPolicy:      "fakeExportPolicy",
		SpaceReserve:      "fakeSpaceReserve",
		SecurityStyle:     "fakeSecurityStyle",
		SplitOnClone:      "fakeSplitOnClone",
		FileSystem:        "fakeFilesystem",
		Encryption:        "fakeEncryption",
		QosPolicy:         "fakeQoSPolicy",
		AdaptiveQosPolicy: "fakeAdaptiveQosPolicy",
	}

	// "Encryption: "false"
	requests := map[string]sa.Request{
		sa.IOPS:             sa.NewIntRequest(40),
		sa.Snapshots:        sa.NewBoolRequest(true),
		sa.Encryption:       sa.NewBoolRequest(false),
		sa.ProvisioningType: sa.NewStringRequest("thin"),
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			volConfig.SnapshotDir = test.inputSnapshotDir
			opts := getVolumeOptsCommon(ctx, volConfig, requests)
			assert.NotEqual(t, 0, len(opts))
			assert.Equal(t, test.expectedSnapshotDir, opts["snapshotDir"])
		})
	}
}

func TestGetVolumeOptsCommon_NoVolumeOptsReturned(t *testing.T) {
	ctx := context.Background()
	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}

	// Invalid value of Encryption and ProvisioningType
	requests := map[string]sa.Request{
		sa.IOPS:             sa.NewIntRequest(40),
		sa.Snapshots:        sa.NewBoolRequest(true),
		sa.Encryption:       sa.NewIntRequest(5555),
		sa.ProvisioningType: sa.NewStringRequest("invalid"),
	}

	opts := getVolumeOptsCommon(ctx, volConfig, requests)

	assert.Equal(t, 0, len(opts))
}

func TestTelemetryString(t *testing.T) {
	mockAPI, driver := newMockOntapNASDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	driver.telemetry = &Telemetry{
		Plugin: driver.Name(),
		SVM:    "SVM1",
		Driver: driver,
		done:   make(chan struct{}),
	}
	// Check the output
	expected := fmt.Sprintf("%s", driver.telemetry)

	// Call the String() method
	result := driver.telemetry.String()

	assert.Equal(t, expected, result)
}

func TestGoString(t *testing.T) {
	mockAPI, driver := newMockOntapNASDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	driver.telemetry = &Telemetry{
		Plugin: driver.Name(),
		SVM:    "SVM1",
		Driver: driver,
		done:   make(chan struct{}),
	}
	// Check the output
	expected := fmt.Sprintf("%s", driver.telemetry)

	// Call the String() method
	result := driver.telemetry.GoString()

	assert.Equal(t, expected, result)
}

func mockVolumeExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func mockVolumeSize(_ context.Context, _ string) (uint64, error) {
	return 1024, nil
}

func mockVolumeExistsError(_ context.Context, _ string) (bool, error) {
	return true, fmt.Errorf("VolumeExistsError")
}

func mockVolumeSizeError(_ context.Context, _ string) (uint64, error) {
	return 1024, fmt.Errorf("VolumeSizeError")
}

func mockVolumeSizeLarger(_ context.Context, _ string) (uint64, error) {
	return 10000, nil
}

func mockVolumeInfo(_ context.Context, name string) (*api.Volume, error) {
	return &api.Volume{
		Name:            name,
		SnapshotPolicy:  "fakePolicy",
		SnapshotReserve: 0,
	}, nil
}

func mockVolumeInfoWithSnapshotReserve(ctx context.Context, name string) (*api.Volume, error) {
	return &api.Volume{
		Name:            name,
		SnapshotPolicy:  "fakePolicy",
		SnapshotReserve: 90,
	}, nil
}

func TestResizeValidation(t *testing.T) {
	// Test1: Positive flow
	ctx := context.Background()
	name := "test"
	sizeBytes := 1024

	volConfig := &storage.VolumeConfig{
		Name:         name,
		InternalName: name + "internal",
		Size:         "1024",
	}

	// Test1: No change, volume size and new size are the same value
	val, err := resizeValidation(ctx, volConfig, uint64(sizeBytes), mockVolumeExists, mockVolumeSize, mockVolumeInfo)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), val)

	// Test2: Error flow: volume does not exist
	_, err = resizeValidation(ctx, volConfig, uint64(sizeBytes), mockVolumeExistsError, mockVolumeSize, mockVolumeInfo)
	assert.Error(t, err)

	// Test3: Error flow: Volume size error
	_, err = resizeValidation(ctx, volConfig, uint64(sizeBytes), mockVolumeExists, mockVolumeSizeError, mockVolumeInfo)
	assert.Error(t, err)

	// Test4: Error flow: Requested Volume size is less than the previous size
	_, err = resizeValidation(ctx, volConfig, uint64(sizeBytes-100), mockVolumeExists, mockVolumeSizeLarger,
		mockVolumeInfo)
	assert.Error(t, err)
	ok, _ := errors.HasUnsupportedCapacityRangeError(err)
	assert.Equal(t, true, ok)

	// Test5: Positive flow: Volume should resize
	newSize, err := resizeValidation(ctx, volConfig, uint64(sizeBytes*10), mockVolumeExists, mockVolumeSizeLarger,
		mockVolumeInfoWithSnapshotReserve)
	assert.NoError(t, err)
	assert.Equal(t, uint64(sizeBytes*100), newSize)
}

func TestGetISCSITargetInfo(t *testing.T) {
	// Test1: Positive flow
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		SVM:                       "testSVM",
	}

	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("TestiSCSINodeName", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, config.SVM).Return([]string{"TestiSCSIInterface1"}, nil)

	nodeName, _, err := GetISCSITargetInfo(ctx, mockAPI, config)

	assert.Equal(t, "TestiSCSINodeName", nodeName)
	assert.NoError(t, err)

	// Test2: Error flow : could not get SVM iSCSI node name
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("TestiSCSINodeName",
		fmt.Errorf("IscsiNodeGetNameRequest returned error"))

	_, _, err = GetISCSITargetInfo(ctx, mockAPI, config)

	assert.Error(t, err)

	// Test3: Error flow : could not get SVM iSCSI interface
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("TestiSCSINodeName", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, config.SVM).Return([]string{"TestiSCSIInterface1"},
		fmt.Errorf("IscsiNodeGetNameRequest returned error"))

	_, _, err = GetISCSITargetInfo(ctx, mockAPI, config)

	assert.Error(t, err)

	// Test4: Error flow : Nil interface
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("TestiSCSINodeName", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, config.SVM).Return(nil, nil)

	_, _, err = GetISCSITargetInfo(ctx, mockAPI, config)

	assert.Error(t, err)
}

func TestValidateBidirectionalChapCredentials(t *testing.T) {
	defaultAuth := api.IscsiInitiatorAuth{
		SVMName:          "testSVM",
		AuthType:         "none",
		ChapOutboundUser: "",
		ChapUser:         "",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		SVM:                       "testSVM",
		ChapInitiatorSecret:       "fakeChapInitiatorSecret",
		ChapUsername:              "fakeChapUsername",
		ChapTargetUsername:        "fakeChapTargetUsername",
		ChapTargetInitiatorSecret: "fakeChapTargetInitiatorSecret",
	}

	credentials, err := ValidateBidirectionalChapCredentials(defaultAuth, config)

	assert.Equal(t, config.ChapInitiatorSecret, credentials.ChapInitiatorSecret)
	assert.Equal(t, config.ChapUsername, credentials.ChapUsername)
	assert.Equal(t, config.ChapTargetUsername, credentials.ChapTargetUsername)
	assert.Equal(t, config.ChapTargetInitiatorSecret, credentials.ChapTargetInitiatorSecret)
	assert.NoError(t, err)
}

func TestValidateBidirectionalChapCredentials_MissingValues(t *testing.T) {
	defaultAuth := api.IscsiInitiatorAuth{
		SVMName:          "testSVM",
		AuthType:         "none",
		ChapOutboundUser: "",
		ChapUser:         "",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		SVM:                       "testSVM",
		ChapInitiatorSecret:       "",
		ChapUsername:              "",
		ChapTargetUsername:        "",
		ChapTargetInitiatorSecret: "",
	}

	_, err := ValidateBidirectionalChapCredentials(defaultAuth, config)

	assert.Error(t, err)
}

func TestValidateBidirectionalChapCredentials_AuthCHAP(t *testing.T) {
	defaultAuth := api.IscsiInitiatorAuth{
		SVMName:          "testSVM",
		AuthType:         "CHAP",
		ChapOutboundUser: "",
		ChapUser:         "",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		SVM:                       "testSVM",
		ChapInitiatorSecret:       "fakeChapInitiatorSecret",
		ChapUsername:              "fakeChapUsername",
		ChapTargetUsername:        "fakeChapTargetUsername",
		ChapTargetInitiatorSecret: "fakeChapTargetInitiatorSecret",
	}

	_, err := ValidateBidirectionalChapCredentials(defaultAuth, config)

	assert.Error(t, err)
}

func TestValidateBidirectionalChapCredentials_AuthDeny(t *testing.T) {
	defaultAuth := api.IscsiInitiatorAuth{
		SVMName:          "testSVM",
		AuthType:         "deny",
		ChapOutboundUser: "",
		ChapUser:         "",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		SVM:                       "testSVM",
		ChapInitiatorSecret:       "fakeChapInitiatorSecret",
		ChapUsername:              "fakeChapUsername",
		ChapTargetUsername:        "fakeChapTargetUsername",
		ChapTargetInitiatorSecret: "fakeChapTargetInitiatorSecret",
	}

	_, err := ValidateBidirectionalChapCredentials(defaultAuth, config)

	assert.Error(t, err)
}

func TestValidateBidirectionalChapCredentials_Error(t *testing.T) {
	defaultAuth := api.IscsiInitiatorAuth{
		SVMName:          "testSVM",
		AuthType:         "fake",
		ChapOutboundUser: "",
		ChapUser:         "",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		SVM:                       "testSVM",
		ChapInitiatorSecret:       "fakeChapInitiatorSecret",
		ChapUsername:              "fakeChapUsername",
		ChapTargetUsername:        "fakeChapTargetUsername",
		ChapTargetInitiatorSecret: "fakeChapTargetInitiatorSecret",
	}

	_, err := ValidateBidirectionalChapCredentials(defaultAuth, config)

	assert.Error(t, err)
}

func TestValidateBidirectionalChapCredentials_DifferntCHAPUsers(t *testing.T) {
	defaultAuth := api.IscsiInitiatorAuth{
		SVMName:          "testSVM",
		AuthType:         "CHAP",
		ChapOutboundUser: "fakeOutbound",
		ChapUser:         "fake",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		SVM:                       "testSVM",
		ChapInitiatorSecret:       "fakeChapInitiatorSecret",
		ChapUsername:              "fakeChapUsername",
		ChapTargetUsername:        "fakeChapTargetUsername",
		ChapTargetInitiatorSecret: "fakeChapTargetInitiatorSecret",
	}

	_, err := ValidateBidirectionalChapCredentials(defaultAuth, config)

	assert.Error(t, err)
}

func TestGetInternalVolumeNameCommon(t *testing.T) {
	ctx := context.Background()
	// Test-1 UsingPassthroughStore == true
	tridentconfig.UsingPassthroughStore = true
	storagePrefix := "trident"
	name := "pvc_123456789"
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		StoragePrefix:   &storagePrefix,
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}
	expected := "tridentpvc_123456789"
	volConfig := &storage.VolumeConfig{Name: name}
	pool := storage.NewStoragePool(nil, "dummyPool")

	out := getInternalVolumeNameCommon(ctx, config, volConfig, pool)

	assert.Equal(t, expected, out)

	// Test-2 UsingPassthroughStore == false
	tridentconfig.UsingPassthroughStore = false
	expected = "trident_pvc_123456789"

	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)

	assert.Equal(t, expected, out)

	// Test-3 UsingPassthroughStore == false and valid name template
	tridentconfig.UsingPassthroughStore = false
	expected = "pool_dev_test_cluster_1"
	pool = getValidOntapNASPool()
	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-4 UsingPassthroughStore == false and invalid name template
	tridentconfig.UsingPassthroughStore = false
	// getInternalVolumeNameCommon returns an internal volume name if nameTemplate generation fails.
	expected = "trident_pvc_123456789"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			// "Namespac" is an invalid field
			NameTemplate: "pool_{{.labels.Cluster}}_{{.volume.Namespac}}_{{.volume.RequestName}}",
		},
	)
	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-5 UsingPassthroughStore == false and valid name template with multiple underscore
	tridentconfig.UsingPassthroughStore = false
	expected = "pool"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_________{{.labels.Cluster}}_______{{.volume.Namespace}}______{{.volume." +
				"RequestName}}_____",
		},
	)
	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-6 UsingPassthroughStore == false and valid name template with special character
	tridentconfig.UsingPassthroughStore = false
	expected = "pool"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool$%^&*{{.labels.Cluster}}_______{{.volume.Namespace}}______{{.volume." +
				"RequestName}}_____",
		},
	)
	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-7 UsingPassthroughStore == false and valid name template with label exist in pool
	tridentconfig.UsingPassthroughStore = false
	// "dev_test_cluster_1" is the pool label set in function getValidOntapNASPool
	expected = "pool_dev_test_cluster_1"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_{{.labels.clusterName}}_{{.volume.Namespace}}_{{.volume.RequestName}}",
		},
	)
	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-8 UsingPassthroughStore == false and valid name template with volume namespace
	tridentconfig.UsingPassthroughStore = false
	volConfig.Namespace = "trident"
	expected = "pool_dev_test_cluster_1_trident"
	pool = getValidOntapNASPool()
	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-9 UsingPassthroughStore == false and valid name template with volume requestName
	tridentconfig.UsingPassthroughStore = false
	volConfig.Namespace = "trident"
	volConfig.RequestName = "volumeRequestName"
	expected = "pool_dev_test_cluster_1_trident_volumeRequestName"
	pool = getValidOntapNASPool()
	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-10 invalid name template
	expected = "trident_pvc_123456789"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_{{.labels.clusterName}}_{{.volume.Namespace}}_{{.volume.RequestName",
		},
	)
	out = getInternalVolumeNameCommon(ctx, config, volConfig, pool)
	assert.Equal(t, expected, out)
}

func TestEnsureUniquenessInNameTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "name template contains .volume.Name",
			input:    "test_{{.volume.Name}}",
			expected: "test_{{.volume.Name}}",
		},
		{
			name:     "name template empty",
			input:    "",
			expected: "",
		},
		{
			name:     "name template does not contain .volume.Name",
			input:    "test",
			expected: "test_{{slice .volume.Name 4 9}}",
		},
		{
			name:     "name template has slice function",
			input:    "test_{{slice .volume.Name 4 9}}",
			expected: "test_{{slice .volume.Name 4 9}}",
		},
		{
			name:     "name template contains .volume.Name outside a curly brackets ",
			input:    "test.volume.Name",
			expected: "test.volume.Name_{{slice .volume.Name 4 9}}",
		},
		{
			name:     "name template contains .volume.Name outside a curly brackets",
			input:    "test_{{test}}.volume.Name{{test}}",
			expected: "test_{{test}}.volume.Name{{test}}_{{slice .volume.Name 4 9}}",
		},
		{
			name:  "name template contains .volume.Namespace, does not contain .volume.Name",
			input: "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}",
			expected: "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_" +
				"{{slice .volume.Name 4 9}}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := ensureUniquenessInNameTemplate(test.input)
			if output != test.expected {
				t.Errorf("expected %s, got %s", test.expected, output)
			}
		})
	}
}

func MockModifyVolumeExportPolicy(ctx context.Context, volName, policyName string) error {
	return nil
}

func TestAddUniqueIscsiIGroupName(t *testing.T) {
	tests := []struct {
		message         string
		IscsiIgroup     string
		IscsiAccessInfo string
		ExpectedOutput  string
	}{
		{
			"Unique IscsiIgroupName", "fakeigroupName3", "fakeigroupName1,fakeigroupName2",
			"fakeigroupName1,fakeigroupName2,fakeigroupName3",
		},
		{
			"Duplicate IscsiIgroupName", "fakeigroupName2", "fakeigroupName1,fakeigroupName2,fakeigroupName3",
			"fakeigroupName1,fakeigroupName2,fakeigroupName3",
		},
		{"Empty VolumePublishInfo", "fakeigroupName2", "", "fakeigroupName2"},
	}

	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			volumeAccessInfo := tridentmodels.VolumeAccessInfo{
				IscsiAccessInfo: tridentmodels.IscsiAccessInfo{
					IscsiIgroup: test.IscsiAccessInfo,
				},
			}

			publishInfo := &tridentmodels.VolumePublishInfo{
				VolumeAccessInfo: volumeAccessInfo,
			}
			addUniqueIscsiIGroupName(publishInfo, test.IscsiIgroup)
			assert.Equal(t, publishInfo.IscsiIgroup, test.ExpectedOutput)
		})
	}
}

func TestPublishLun(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	lunPath := "fakeLunPath"
	igroupName := "fakeigroupName"
	iSCSINodeName := "fakeiSCSINodeName"
	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8"}

	node := tridentmodels.Node{
		IPs: ips,
	}
	nodeList := []*tridentmodels.Node{&node}

	dummyLun := &api.Lun{
		Comment:      "dummyLun",
		SerialNumber: "testSerialNumber",
	}

	dummyLunNoSerial := &api.Lun{
		Comment:      "dummyLun",
		SerialNumber: "",
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		DriverContext:   "csi",
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		SVM:                       "testSVM",
		AutoExportPolicy:          true,
		UseCHAP:                   true,
	}

	config.SANType = sa.ISCSI

	publishInfo := &tridentmodels.VolumePublishInfo{
		BackendUUID: "fakeBackendUUID",
		Localhost:   false,
		Unmanaged:   true,
		Nodes:       nodeList,
		HostIQN:     []string{"host_iqn"},
	}
	// Test1 - Positive flow
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("fstype", nil)
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return("formatOptions", nil)
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, igroupName, publishInfo.HostIQN[0])
	mockAPI.EXPECT().EnsureLunMapped(ctx, igroupName, lunPath).Return(1111, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, igroupName, lunPath).Return([]string{"Node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, ips, []string{"Node1"}).Return([]string{}, nil)

	err := PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.NoError(t, err)

	// Test2 - Error Path: No hostIQN passed
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	publishInfo.Localhost = false
	publishInfo.HostIQN = []string{}

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.Error(t, err)

	// Test 3 - LunGetFSType returns error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	publishInfo.HostIQN = []string{"host_iqn"}
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("", fmt.Errorf("LunGetFSType returned error"))
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return("formatOptions", nil)
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, igroupName, publishInfo.HostIQN[0])
	mockAPI.EXPECT().EnsureLunMapped(ctx, igroupName, lunPath).Return(1111, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, igroupName, lunPath).Return([]string{"Node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, ips, []string{"Node1"}).Return([]string{}, nil)

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.NoError(t, err)

	// Test 4 - LunGetFSType returns error
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	publishInfo.HostIQN = []string{"host_iqn"}
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("fsType", nil)
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return("", fmt.Errorf("LunGetAttribute returned error"))
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, igroupName, publishInfo.HostIQN[0])
	mockAPI.EXPECT().EnsureLunMapped(ctx, igroupName, lunPath).Return(1111, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, igroupName, lunPath).Return([]string{"Node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, ips, []string{"Node1"}).Return([]string{}, nil)

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.NoError(t, err)

	// Test 5 - No target node found
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	publishInfo.HostIQN = []string{"host_iqn"}
	publishInfo.HostName = "fakeHostName"
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("", fmt.Errorf("LunGetFSType returned error"))
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return("formatOptions", nil)
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLun, nil)

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.Error(t, err)

	// Test 6 - EnsureIgroupAdded returns error
	publishInfo = &tridentmodels.VolumePublishInfo{
		BackendUUID: "fakeBackendUUID",
		Localhost:   false,
		Unmanaged:   false,
		Nodes:       nodeList,
		HostIQN:     []string{"host_iqn"},
	}
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("fstype", nil)
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return("formatOptions", nil)
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, igroupName,
		gomock.Any()).Return(fmt.Errorf("EnsureIgroupAdded returned error"))

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.Error(t, err)

	// Test 7 - EnsureLunMapped returns error
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("fstype", nil)
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return("formatOptions", nil)
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureLunMapped(ctx, igroupName, lunPath).Return(1111,
		fmt.Errorf("EnsureLunMapped returned error"))
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, igroupName, gomock.Any()).Return(nil)

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.Error(t, err)

	// Test 8 - LunGetByName returns error
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("fstype", nil)
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return("formatOptions", nil)
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLun, fmt.Errorf("LunGetByName returned error"))

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.Error(t, err)

	// Test 9 - LunGetByName returns nil but Serial Number is empty
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("fstype", nil)
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return("formatOptions", nil)
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLunNoSerial, nil)

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)

	assert.Error(t, err)

	// Test 10 - Checking whether the correct value of formatOptions is updated or not
	publishInfo = &tridentmodels.VolumePublishInfo{
		BackendUUID: "fakeBackendUUID",
		Localhost:   false,
		Unmanaged:   true,
		Nodes:       nodeList,
		HostIQN:     []string{"host_iqn"},
	}
	tempFormatOptions := "-b 4096 -T stride=256"
	mockAPI.EXPECT().LunGetFSType(ctx, lunPath).Return("fstype", nil)
	mockAPI.EXPECT().LunGetAttribute(ctx, lunPath, "formatOptions").Return(tempFormatOptions, nil)
	mockAPI.EXPECT().LunGetByName(ctx, lunPath).Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, igroupName, publishInfo.HostIQN[0])
	mockAPI.EXPECT().EnsureLunMapped(ctx, igroupName, lunPath).Return(1111, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, igroupName, lunPath).Return([]string{"Node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, ips, []string{"Node1"}).Return([]string{}, nil)

	err = PublishLUN(ctx, mockAPI, config, ips, publishInfo, lunPath, igroupName, iSCSINodeName)
	assert.NoError(t, err)
	assert.Equal(t, tempFormatOptions, publishInfo.FormatOptions)
}

func TestValidateSANDriver(t *testing.T) {
	ctx := context.Background()

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8"}
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		DriverContext:   "csi",
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AutoExportPolicy:          true,
		UseCHAP:                   true,
		DataLIF:                   "1.1.1.1",
	}

	err := ValidateSANDriver(ctx, config, ips)
	assert.NoError(t, err)

	// Test 2:  IP not found
	ips = []string{"2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8"}

	err = ValidateSANDriver(ctx, config, ips)

	assert.NoError(t, err)
}

func TestValidateSANDriver_IPNotFound(t *testing.T) {
	ctx := context.Background()

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		DriverContext:   "csi",
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AutoExportPolicy:          true,
		UseCHAP:                   true,
		DataLIF:                   "1.1.1.1",
	}

	err := ValidateSANDriver(ctx, config, ips)

	assert.NoError(t, err)
}

func TestValidateSANDriver_BackendIgroupDeprecation(t *testing.T) {
	ctx := context.Background()

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		DriverContext:   "csi",
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AutoExportPolicy:          true,
		UseCHAP:                   true,
		DataLIF:                   "1.1.1.1",
	}

	err := ValidateSANDriver(ctx, config, ips)

	assert.NoError(t, err)
}

func TestValidateNASDriver(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		DriverContext:   "csi",
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}
	// Test 1 - true value of LUKS
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "true"

	err := ValidateNASDriver(ctx, mockAPI, config)

	assert.Error(t, err)

	// Test 2 - Invalid value of LUKS
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "fake"

	err = ValidateNASDriver(ctx, mockAPI, config)

	assert.Error(t, err)

	// Test 3 - false value of LUKS
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "false"
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return(nil, fmt.Errorf("error returned"))

	err = ValidateNASDriver(ctx, mockAPI, config)

	assert.Error(t, err)

	// Test 4: NASType = SMB
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "false"
	config.NASType = sa.SMB
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "cifs").AnyTimes().Return(nil, nil)

	err = ValidateNASDriver(ctx, mockAPI, config)

	assert.Error(t, err)

	// Test 5 - no data LIF returned
	config.NASType = sa.NFS
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "false"
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return([]string{}, nil)

	err = ValidateNASDriver(ctx, mockAPI, config)

	assert.Error(t, err)

	// Test 6 - Positive flow
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "false"
	dataLIF := make([]string, 0)
	dataLIF = append(dataLIF, "5.5.5.5")
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return(dataLIF, nil)

	err = ValidateNASDriver(ctx, mockAPI, config)

	assert.NoError(t, err)

	// Test 7 - Config.DataLIF empty
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "false"
	config.DataLIF = ""
	dataLIF = make([]string, 0)
	dataLIF = append(dataLIF, "5.5.5.5")
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return(dataLIF, nil)

	err = ValidateNASDriver(ctx, mockAPI, config)

	assert.NoError(t, err)

	// Test 8 - CIDR validation
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "false"
	config.DataLIF = ""
	dataLIF = make([]string, 0)
	dataLIF = append(dataLIF, "5.5.5.5")
	config.AutoExportCIDRs = []string{"192.168.0.0***24"}
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return(dataLIF, nil)

	err = ValidateNASDriver(ctx, mockAPI, config)

	assert.Error(t, err)

	// Test 9 - IPV6 check
	config.OntapStorageDriverConfigDefaults.LUKSEncryption = "false"
	config.DataLIF = ""
	dataLIF = make([]string, 0)
	dataLIF = append(dataLIF, "4444:5555:6666:7777")
	config.AutoExportCIDRs = []string{"192.168.0.0/24"}
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return(dataLIF, nil)

	err = ValidateNASDriver(ctx, mockAPI, config)

	assert.NoError(t, err)
}

func TestCheckVolumePoolSizeLimits(t *testing.T) {
	ctx := context.Background()
	requestedSize := uint64(1073741824) // 1Gi

	// Returns because LimitVolumePoolSize not specified.
	config := &drivers.OntapStorageDriverConfig{
		LimitVolumePoolSize: "",
	}
	shouldLimit, sizeLimit, err := CheckVolumePoolSizeLimits(ctx, requestedSize, config)
	assert.False(t, shouldLimit, "expected should limit to be false")
	assert.Zero(t, sizeLimit, "expected zero size limit")
	assert.Nil(t, err, "expected nil error")

	// Errors when LimitVolumePoolSize is not empty but invalid and cannot be parsed.
	config = &drivers.OntapStorageDriverConfig{
		LimitVolumePoolSize: "Gi",
	}
	shouldLimit, sizeLimit, err = CheckVolumePoolSizeLimits(ctx, requestedSize, config)
	assert.False(t, shouldLimit, "expected should limit to be false")
	assert.Zero(t, sizeLimit, "expected zero size limit")
	assert.NotNil(t, err, "expected non-nil error")

	// Errors when the requested volume is larger than LimitVolumePoolSize.
	requestedSize = uint64(2000000000)
	config = &drivers.OntapStorageDriverConfig{
		LimitVolumePoolSize: "1Gi",
	}
	shouldLimit, sizeLimit, err = CheckVolumePoolSizeLimits(ctx, requestedSize, config)
	assert.True(t, shouldLimit, "expected should limit to be true")
	assert.Equal(t, sizeLimit, uint64(1073741824), "expected size limit of 1Gi")
	assert.NotNil(t, err, "expected non-nil error")

	requestedSize = uint64(1000000000)
	config = &drivers.OntapStorageDriverConfig{
		LimitVolumePoolSize: "1Gi",
	}
	shouldLimit, sizeLimit, err = CheckVolumePoolSizeLimits(ctx, requestedSize, config)
	assert.True(t, shouldLimit, "expected should limit to be true")
	assert.Equal(t, sizeLimit, uint64(1073741824), "expected size limit of 1Gi")
	assert.Nil(t, err, "expected nil error")
}

func TestGetSnapshotReserve(t *testing.T) {
	// snapshot reserve is not passed
	snapshotPolicy := "fakePolicy"
	snapshotReserve := ""

	_, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)

	assert.NoError(t, err)
}

func TestGetSnapshotReserve_ExplicitlyPassed(t *testing.T) {
	// snapshot reserve is passed
	snapshotPolicy := "fakePolicy"
	snapshotReserve := "10"
	expected := 10

	got, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)

	assert.Equal(t, expected, got)
	assert.NoError(t, err)
}

func TestGetSnapshotReserve_PolicyNotSet(t *testing.T) {
	// snapshotPolicy is not set
	snapshotPolicy := ""
	snapshotReserve := ""
	expected := 0

	got, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)

	assert.Equal(t, expected, got)
	assert.NoError(t, err)
}

func TestGetStorageBackendSpecsCommon(t *testing.T) {
	// Test1: Physical pools provided
	backend := &storage.StorageBackend{}
	backend.SetName("dummybackend")
	backend.SetOnline(true)
	backend.SetStorage(make(map[string]storage.Pool))
	pool1 := storage.NewStoragePool(nil, "dummyPool1")
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)

	pool2 := storage.NewStoragePool(nil, "dummyPool2")
	pool2.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool2.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool2.Attributes()[sa.Replication] = sa.NewBoolOffer(false)

	physicalPools := map[string]storage.Pool{"dummyPool1": pool1, "dummyPool2": pool2}
	virtualPools := map[string]storage.Pool{}

	err := getStorageBackendSpecsCommon(backend, physicalPools, virtualPools, "dummybackend")

	assert.NoError(t, err)

	// Test2: Virtual pools provided
	virtualPools = map[string]storage.Pool{"dummyPool1": pool1, "dummyPool2": pool2}
	physicalPools = map[string]storage.Pool{}

	err = getStorageBackendSpecsCommon(backend, physicalPools, virtualPools, "dummybackend")

	assert.NoError(t, err)
}

func TestCloneFlexvol(t *testing.T) {
	type parameters struct {
		configureOntapAPI         func(ontapAPI *mockapi.MockOntapAPI)
		cloneVolumeConfig         storage.VolumeConfig
		storageDriverConfig       drivers.OntapStorageDriverConfig
		split                     bool
		expectError               bool
		validateCloneVolumeConfig func(t *testing.T, cloneVolumeConfig storage.VolumeConfig)
	}

	const internalName = "dummy"
	const cloneSourceVolumeInternal = "fakeSource"
	const cloneSourceSnapshotInternal = "fakeSnap"
	const label = "fakeLabel"

	qosPolicyGroup := api.QosPolicyGroup{
		Name: "fakePolicyGroup",
		Kind: api.QosPolicyGroupKind,
	}
	storageDriverConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DebugTraceFlags:   map[string]bool{"method": true},
			StorageDriverName: tridentconfig.OntapNASStorageDriverName,
		},
	}
	storageDriverConfigNVMe := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DebugTraceFlags:   map[string]bool{"method": true},
			StorageDriverName: tridentconfig.OntapNASStorageDriverName,
		},
		OntapStorageDriverPool: drivers.OntapStorageDriverPool{SANType: sa.NVMe},
	}

	cloneVolumeConfig := storage.VolumeConfig{
		InternalName:                internalName,
		CloneSourceVolumeInternal:   cloneSourceVolumeInternal,
		CloneSourceSnapshotInternal: cloneSourceSnapshotInternal,
	}
	cloneVolumeConfigNoSnapshot := storage.VolumeConfig{
		InternalName:                internalName,
		CloneSourceVolumeInternal:   cloneSourceVolumeInternal,
		CloneSourceSnapshotInternal: "",
	}
	ctx := context.Background()

	tests := map[string]parameters{
		"Error case: volumeExists returns error": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(false, fmt.Errorf("volumeExists returned error"))
			},
			cloneVolumeConfig:   cloneVolumeConfig,
			storageDriverConfig: storageDriverConfig,
			split:               false,
			expectError:         true,
		},
		"Error case: volume already exists": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(true, nil)
			},
			cloneVolumeConfig:   cloneVolumeConfig,
			storageDriverConfig: storageDriverConfig,
			split:               false,
			expectError:         true,
		},
		"No specific snapshot was requested": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(false, nil)
				mockAPI.EXPECT().VolumeSnapshotCreate(ctx, time.Now().UTC().Format(storage.SnapshotNameFormat),
					"fakeSource").Return(fmt.Errorf("VolumeSnapshotCreate returned error"))
			},
			cloneVolumeConfig:   cloneVolumeConfigNoSnapshot,
			storageDriverConfig: storageDriverConfig,
			split:               false,
			expectError:         true,
			validateCloneVolumeConfig: func(t *testing.T, cloneVolumeConfig storage.VolumeConfig) {
				assert.Empty(t, cloneVolumeConfig.CloneSourceSnapshotInternal)
				assert.Empty(t, cloneVolumeConfig.CloneSourceSnapshot)
			},
		},
		"Creating clone returns error": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(false, nil)
				mockAPI.EXPECT().VolumeCloneCreate(
					ctx, internalName, cloneSourceVolumeInternal, cloneSourceSnapshotInternal, false,
				).Return(fmt.Errorf("error creating clone"))
			},
			cloneVolumeConfig:   cloneVolumeConfig,
			storageDriverConfig: storageDriverConfig,
			split:               false,
			expectError:         true,
		},
		"NVMe clone creation returns error": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(false, nil)
				mockAPI.EXPECT().VolumeCloneCreate(
					ctx, internalName, cloneSourceVolumeInternal, cloneSourceSnapshotInternal, false,
				).Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, internalName, gomock.Any(), gomock.Any(),
					maxFlexvolCloneWait).Return("", fmt.Errorf("error waiting for NVMe clone"))
			},
			cloneVolumeConfig:   cloneVolumeConfig,
			storageDriverConfig: storageDriverConfigNVMe,
			split:               false,
			expectError:         true,
		},
		"VolumeSetComment returns error": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(false, nil)
				mockAPI.EXPECT().VolumeCloneCreate(
					ctx, internalName, cloneSourceVolumeInternal, cloneSourceSnapshotInternal, false,
				).Return(nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, internalName, internalName, label).Return(fmt.Errorf("error creating clone"))
				mockAPI.EXPECT().VolumeWaitForStates(ctx, internalName, gomock.Any(), gomock.Any(),
					maxFlexvolCloneWait).Return("online", nil)
			},
			cloneVolumeConfig:   cloneVolumeConfig,
			storageDriverConfig: storageDriverConfig,
			split:               false,
			expectError:         true,
		},
		"VolumeMount returns error": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(false, nil)
				mockAPI.EXPECT().VolumeCloneCreate(
					ctx, internalName, cloneSourceVolumeInternal, cloneSourceSnapshotInternal, false,
				).Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, internalName, gomock.Any(), gomock.Any(),
					maxFlexvolCloneWait).Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, internalName, internalName, label).Return(nil)
				mockAPI.EXPECT().VolumeMount(ctx, internalName, "/"+internalName).Return(fmt.Errorf("error mounting volume"))
			},
			cloneVolumeConfig:   cloneVolumeConfig,
			storageDriverConfig: storageDriverConfig,
			split:               false,
			expectError:         true,
		},
		"Error setting QoS Policy": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(false, nil)
				mockAPI.EXPECT().VolumeCloneCreate(
					ctx, internalName, cloneSourceVolumeInternal, cloneSourceSnapshotInternal, false,
				).Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, internalName, gomock.Any(), gomock.Any(),
					maxFlexvolCloneWait).Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, internalName, internalName, label).Return(nil)
				mockAPI.EXPECT().VolumeMount(ctx, internalName, "/"+internalName).Return(nil)
				mockAPI.EXPECT().VolumeSetQosPolicyGroupName(
					ctx, internalName, qosPolicyGroup,
				).Return(fmt.Errorf("error setting qos policy"))
			},
			cloneVolumeConfig:   cloneVolumeConfig,
			storageDriverConfig: storageDriverConfig,
			split:               false,
			expectError:         true,
		},
		"Error splitting clone": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, internalName).Return(false, nil)
				mockAPI.EXPECT().VolumeCloneCreate(
					ctx, internalName, cloneSourceVolumeInternal, cloneSourceSnapshotInternal, false,
				).Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, internalName, gomock.Any(), gomock.Any(),
					maxFlexvolCloneWait).Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, internalName, internalName, label).Return(nil)
				mockAPI.EXPECT().VolumeMount(ctx, internalName, "/"+internalName).Return(nil)
				mockAPI.EXPECT().VolumeSetQosPolicyGroupName(ctx, internalName, qosPolicyGroup).Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, internalName).Return(fmt.Errorf("error splitting clone"))
			},
			cloneVolumeConfig:   cloneVolumeConfig,
			storageDriverConfig: storageDriverConfig,
			split:               true,
			expectError:         true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

			if params.configureOntapAPI != nil {
				params.configureOntapAPI(mockAPI)
			}

			err := cloneFlexvol(
				ctx, &params.cloneVolumeConfig, label, params.split, &params.storageDriverConfig, mockAPI, qosPolicyGroup,
			)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
			}

			if params.validateCloneVolumeConfig != nil {
				params.validateCloneVolumeConfig(t, params.cloneVolumeConfig)
			}
		})
	}
}

func TestCreateFlexvolSnapshot(t *testing.T) {
	ctx := context.Background()
	name := "fakeVolumeInternalName"

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags:   map[string]bool{"method": true},
		StorageDriverName: tridentconfig.OntapNASStorageDriverName,
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "fakeInternalName",
		VolumeInternalName: "fakeVolumeInternalName",
	}

	// Test-1: Error flow: error while finding volume
	mockAPI.EXPECT().VolumeExists(ctx, name).Return(false, fmt.Errorf("Volume doesn't exist"))

	snapshot, err := createFlexvolSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Nil(t, snapshot, "Expected no snapshot")
	assert.Error(t, err)

	// Test2: Error case: Volume doesn't exist
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().VolumeExists(ctx, name).Return(false, nil)

	snapshot, err = createFlexvolSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Nil(t, snapshot, "Expected no snapshot")
	assert.Error(t, err)

	// Test3: Error case: error reading volume size
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().VolumeExists(ctx, name).Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "fakeVolumeInternalName").Return(0, fmt.Errorf("Error reading volume size"))

	snapshot, err = createFlexvolSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Nil(t, snapshot, "Expected no snapshot")
	assert.Error(t, err)

	// Test4: Error case: error creating snapshot
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().VolumeExists(ctx, name).Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "fakeVolumeInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(fmt.Errorf("Error creating snapshot"))

	snapshot, err = createFlexvolSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Nil(t, snapshot, "Expected no snapshot")
	assert.Error(t, err)

	// Test5: Error case: error getting snapshot list
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().VolumeExists(ctx, name).Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "fakeVolumeInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		snapConfig.InternalName, snapConfig.VolumeInternalName).Return(api.Snapshot{},
		fmt.Errorf("Error getting snapshot list"))

	snapshot, err = createFlexvolSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Nil(t, snapshot, "Expected no snapshot")
	assert.Error(t, err)

	// Test6: Error case: Snapshot not found
	dummySnap := api.Snapshot{
		Name: "dummy",
	}
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().VolumeExists(ctx, name).Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "fakeVolumeInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		snapConfig.InternalName, snapConfig.VolumeInternalName).Return(
		dummySnap,
		errors.NotFoundError(fmt.Sprintf("snapshot %v not found for volume %v", snapConfig.InternalName,
			snapConfig.VolumeInternalName)))

	snapshot, err = createFlexvolSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.Nil(t, snapshot, "Expected no snapshot")
	assert.Error(t, err)

	// Test7: Positive case
	dummySnap = api.Snapshot{
		Name: "fakeInternalName",
	}
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().VolumeExists(ctx, name).Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "fakeVolumeInternalName").Return(100, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		snapConfig.InternalName, snapConfig.VolumeInternalName).Return(dummySnap, nil)

	snapshot, err = createFlexvolSnapshot(ctx, snapConfig, config, mockAPI, mockAPI.LunSize)

	assert.NotNil(t, snapshot, "Expected snapshot")
	assert.NoError(t, err)
}

func TestIsFlexvolRW(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	name := "fakeVolume"
	flexvol := &api.Volume{
		AccessType: VolTypeRW,
	}

	// Test1- Positive case
	mockAPI.EXPECT().VolumeInfo(ctx, name).Return(flexvol, nil)
	expected := true

	VolRW, err := isFlexvolRW(ctx, mockAPI, name)

	assert.Equal(t, expected, VolRW)
	assert.NoError(t, err)

	// Test2- Error case
	mockAPI.EXPECT().VolumeInfo(ctx, name).Return(flexvol, fmt.Errorf("Error returned"))
	expected = false

	VolRW, err = isFlexvolRW(ctx, mockAPI, name)

	assert.Equal(t, expected, VolRW)
	assert.Error(t, err)

	// Test3- Access type other than VolTypeRW
	flexvol.AccessType = VolTypeLS
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().VolumeInfo(ctx, name).Return(flexvol, nil)
	expected = false

	VolRW, err = isFlexvolRW(ctx, mockAPI, name)

	assert.Equal(t, expected, VolRW)
	assert.NoError(t, err)
}

func TestGetVolumeSize(t *testing.T) {
	sizeBytes := uint64(0)
	poolDefaultSizeBytes := "20971520"
	expected := uint64(MinimumVolumeSizeBytes)

	// Test1- Getting default size
	size := GetVolumeSize(uint64(sizeBytes), poolDefaultSizeBytes)

	assert.Equal(t, expected, size)

	// Test2- size less than MinimumVolumeSizeBytes
	sizeBytes = 209715

	size = GetVolumeSize(uint64(sizeBytes), poolDefaultSizeBytes)

	assert.Equal(t, expected, size)

	// Test2- Positive case
	sizeBytes = MinimumVolumeSizeBytes

	size = GetVolumeSize(uint64(sizeBytes), poolDefaultSizeBytes)

	assert.Equal(t, sizeBytes, size)
}

func TestGetDefaultIgroupName(t *testing.T) {
	backendUUID := "fakeBackendUUID"

	// Test1 - "csi" context
	driverContext := tridentconfig.DriverContext("csi")
	expected := "trident-fakeBackendUUID"

	result := getDefaultIgroupName(driverContext, backendUUID)

	assert.Equal(t, expected, result)

	// Test2 - "docker" context
	driverContext = tridentconfig.DriverContext("docker")
	expected = "netappdvp"

	result = getDefaultIgroupName(driverContext, backendUUID)

	assert.Equal(t, expected, result)
}

func TestDiscoverBackendAggrNamesCommon(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{}, nil)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	storageDriver := newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName,
		false, nil, mockAPI)

	aggr, err := discoverBackendAggrNamesCommon(ctx, storageDriver)

	assert.Nil(t, aggr, "Expected no aggregate")
	assert.Error(t, err)

	// Test2 - aggregates is available to SVM
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{}, nil).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME},
		nil)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	storageDriver = newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName,
		false, nil, mockAPI)
	expected := ONTAPTEST_VSERVER_AGGR_NAME

	aggr, err = discoverBackendAggrNamesCommon(ctx, storageDriver)

	assert.Equal(t, expected, aggr[0])
	assert.NoError(t, err)

	// Test2 - aggregates is not available to SVM
	mockAPI = mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{}, nil).Return([]string{"aggr1"}, nil)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	storageDriver = newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName,
		false, nil, mockAPI)

	aggr, err = discoverBackendAggrNamesCommon(ctx, storageDriver)

	assert.Nil(t, aggr, "Expected no aggregate")
	assert.Error(t, err)
}

func mockCloneSplitStart(_ context.Context, _ string) error {
	return nil
}

func mockCloneSplitStart_error(_ context.Context, _ string) error {
	return fmt.Errorf("CloneSplitStart returned error")
}

func TestSplitVolumeFromBusySnapshot(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}
	snapConfig := &storage.SnapshotConfig{
		VolumeInternalName: "fakeVolInternalName",
		InternalName:       "fakeInternalName",
	}

	// Test1: Error flow: Error returned by VolumeListBySnapshotParent
	mockAPI.EXPECT().VolumeListBySnapshotParent(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(api.VolumeNameList{},
		fmt.Errorf("Error returned by VolumeListBySnapshotParent"))

	err := SplitVolumeFromBusySnapshot(ctx, snapConfig, config, mockAPI, mockCloneSplitStart)

	assert.Error(t, err)

	// Test2: Error flow : No volumes returned
	mockAPI.EXPECT().VolumeListBySnapshotParent(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(api.VolumeNameList{}, nil)

	err = SplitVolumeFromBusySnapshot(ctx, snapConfig, config, mockAPI, mockCloneSplitStart)

	assert.NoError(t, err)

	// Test3 : Error flow: splitting clone returns error
	mockAPI.EXPECT().VolumeListBySnapshotParent(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(api.VolumeNameList{"vol1"}, nil)

	err = SplitVolumeFromBusySnapshot(ctx, snapConfig, config, mockAPI, mockCloneSplitStart_error)

	assert.Error(t, err)

	// Test4 : Positive flow
	mockAPI.EXPECT().VolumeListBySnapshotParent(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(api.VolumeNameList{"vol1"}, nil)

	err = SplitVolumeFromBusySnapshot(ctx, snapConfig, config, mockAPI, mockCloneSplitStart)

	assert.NoError(t, err)
}

func TestSplitVolumeFromBusySnapshotWithDelay(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}
	snapConfig := &storage.SnapshotConfig{
		VolumeInternalName: "fakeVolInternalName",
		InternalName:       "fakeInternalName",
	}

	// Test 1: No snap found
	cloneSplitTimers := make(map[string]time.Time)
	SplitVolumeFromBusySnapshotWithDelay(ctx, snapConfig, config, mockAPI, mockCloneSplitStart, cloneSplitTimers)

	// Test 2: time difference is < 0
	cloneSplitTimers[snapConfig.ID()] = time.Now().Add(10 * time.Millisecond)
	SplitVolumeFromBusySnapshotWithDelay(ctx, snapConfig, config, mockAPI, mockCloneSplitStart, cloneSplitTimers)

	time.Sleep(50 * time.Millisecond)
	// Test 3: time difference < config.CloneSplitDelay, which is defaulted to 10 seconds.
	SplitVolumeFromBusySnapshotWithDelay(ctx, snapConfig, config, mockAPI, mockCloneSplitStart, cloneSplitTimers)

	// Test 4: SplitVolumeFromBusySnapshot returning error
	// Add the time for first delete in past so that SplitVolume is called.
	cloneSplitTimers[snapConfig.ID()] = time.Now().Add(-10 * time.Second)
	mockAPI.EXPECT().VolumeListBySnapshotParent(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName).Return(api.VolumeNameList{},
		fmt.Errorf("error returned by VolumeListBySnapshotParent"))
	SplitVolumeFromBusySnapshotWithDelay(ctx, snapConfig, config, mockAPI, mockCloneSplitStart, cloneSplitTimers)
}

func TestGetVolumeExternalCommon(t *testing.T) {
	volume := api.Volume{
		Name:            "fakeVolume",
		Size:            "100",
		SnapshotPolicy:  "dummyPolicy",
		ExportPolicy:    "dummyPolicy",
		UnixPermissions: "777",
		Aggregates:      []string{"aggr1"},
	}
	svmName := "SVM1"

	// Test-1 : prefix present
	storagePrefix := "fake"

	volExternal := getVolumeExternalCommon(volume, storagePrefix, svmName)

	assert.NotNil(t, volExternal, "Expected external volume; found none")

	// Test-2 : No prefix present
	storagePrefix = ""

	volExternal = getVolumeExternalCommon(volume, storagePrefix, svmName)

	assert.NotNil(t, volExternal, "Expected external volume; found none")

	// Test-3 : More than one aggregate present
	storagePrefix = "fake"
	volume.Aggregates = []string{"aggr1", "aggr2"}

	volExternal = getVolumeExternalCommon(volume, storagePrefix, svmName)

	assert.NotNil(t, volExternal, "Expected external volume; found none")
}

func TestGetVserverAggrAttributes(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	storageDriver := newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName,
		false, nil, mockAPI)

	pool1 := storage.NewStoragePool(nil, "dummyPool1")
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)

	pool2 := storage.NewStoragePool(nil, "dummyPool2")
	pool2.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool2.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool2.Attributes()[sa.Replication] = sa.NewBoolOffer(false)

	poolsAttributeMap := make(map[string]map[string]sa.Offer)
	poolsAttributeMap["pool1"] = pool1.Attributes()
	poolsAttributeMap["pool2"] = pool2.Attributes()

	// Test-1 : Error while getting SVM aggregates
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{}, fmt.Errorf("Error returned"))

	err := getVserverAggrAttributes(ctx, storageDriver, &poolsAttributeMap)

	assert.Error(t, err)

	// Test-2 : matching pool not found
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{"dummyPool1": "hdd"}, nil)

	err = getVserverAggrAttributes(ctx, storageDriver, &poolsAttributeMap)

	assert.NoError(t, err)

	// Test-3 : Get the storage attributes corresponding to the aggregate type
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{"pool1": "hdd"}, nil)

	err = getVserverAggrAttributes(ctx, storageDriver, &poolsAttributeMap)

	assert.NoError(t, err)

	// Test-4 : Get the storage attributes corresponding to the aggregate of unknown type
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{"pool1": "fake"}, nil)

	err = getVserverAggrAttributes(ctx, storageDriver, &poolsAttributeMap)

	assert.NoError(t, err)
}

func TestInitializeStoragePoolsCommon(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	storageDriver := newTestOntapSANDriver(vserverAdminHost, "443", vserverAggrName,
		false, nil, mockAPI)
	storageDriver.Config.Aggregate = ""
	pool1 := storage.NewStoragePool(nil, "dummyPool1")
	pool1.Attributes()[sa.Media] = sa.NewStringOffer("hdd")

	backendName := "dummyBackend"
	storageDriver.Config.Region = "dummyRegion"
	storageDriver.Config.Zone = "dummyZone"
	CommonConfigDefault := &drivers.CommonStorageDriverConfigDefaults{
		Size:         "10000",
		NameTemplate: "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}",
	}
	defaults := &drivers.OntapStorageDriverConfigDefaults{
		SpaceAllocation:                   "fake",
		SpaceReserve:                      "fakeSpaceReserve",
		SnapshotPolicy:                    "fakeSnapshotPolicy",
		SnapshotReserve:                   "fakeSnapshotReserve",
		SplitOnClone:                      "false",
		UnixPermissions:                   "777",
		SnapshotDir:                       "TRUE",
		ExportPolicy:                      "fakeExportPolicy",
		SecurityStyle:                     "fakeSecurityStyle",
		FileSystemType:                    "fakeFileSystem",
		Encryption:                        "true",
		LUKSEncryption:                    "false",
		TieringPolicy:                     "fakeTieringPolicy",
		QosPolicy:                         "fakeQosPolicy",
		AdaptiveQosPolicy:                 "fakeAdaptiveQosPolicy",
		CommonStorageDriverConfigDefaults: *CommonConfigDefault,
	}
	storageDriver.Config.Storage = []drivers.OntapStorageDriverPool{
		{
			Region: "fakeRegion",
			Zone:   "fakeZone",
			SupportedTopologies: []map[string]string{
				{
					"topology.kubernetes.io/region": "us_east_1",
					"topology.kubernetes.io/zone":   "us_east_1a",
				},
			},
			OntapStorageDriverConfigDefaults: *defaults,
			NASType:                          sa.NFS,
		},
	}

	// Test1 - Positive flow
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"dummyPool1"}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{"dummyPool1": "hdd"}, nil)

	physicalPool, virtualPool, err := InitializeStoragePoolsCommon(ctx, storageDriver, pool1.Attributes(), backendName)

	assert.NotNil(t, physicalPool, "Physical Pool not found when expected")
	assert.NotNil(t, virtualPool, "Virtual Pool not found when expected")
	// Ensure snapshotDir is a lower case value
	assert.Equal(t, "true", virtualPool["dummyBackend_pool_0"].InternalAttributes()[SnapshotDir])
	assert.NoError(t, err)

	// Test2 - Invalid value of encryption
	defaults = &drivers.OntapStorageDriverConfigDefaults{
		SpaceAllocation:                   "fake",
		SpaceReserve:                      "fakeSpaceReserve",
		SnapshotPolicy:                    "fakeSnapshotPolicy",
		SnapshotReserve:                   "fakeSnapshotReserve",
		SplitOnClone:                      "false",
		UnixPermissions:                   "777",
		SnapshotDir:                       "fakeSnapshotDir",
		ExportPolicy:                      "fakeExportPolicy",
		SecurityStyle:                     "fakeSecurityStyle",
		FileSystemType:                    "fakeFileSystem",
		Encryption:                        "fakeValue",
		LUKSEncryption:                    "false",
		TieringPolicy:                     "fakeTieringPolicy",
		QosPolicy:                         "fakeQosPolicy",
		AdaptiveQosPolicy:                 "fakeAdaptiveQosPolicy",
		CommonStorageDriverConfigDefaults: *CommonConfigDefault,
	}
	storageDriver.Config.Storage = []drivers.OntapStorageDriverPool{
		{
			Region: "fakeRegion",
			Zone:   "fakeZone",
			SupportedTopologies: []map[string]string{
				{
					"topology.kubernetes.io/region": "us_east_1",
					"topology.kubernetes.io/zone":   "us_east_1a",
				},
			},
			OntapStorageDriverConfigDefaults: *defaults,
		},
	}
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"dummyPool1"}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{"dummyPool1": "hdd"}, nil)

	physicalPool, virtualPool, err = InitializeStoragePoolsCommon(ctx, storageDriver, pool1.Attributes(), backendName)

	assert.Nil(t, physicalPool, "Physical Pool not expected but found")
	assert.Nil(t, virtualPool, "Virtual pool not exepcted but found")
	assert.Error(t, err)

	// Test3 - Invalid value of snapshotDir
	defaults = &drivers.OntapStorageDriverConfigDefaults{
		SpaceAllocation:                   "fake",
		SpaceReserve:                      "fakeSpaceReserve",
		SnapshotPolicy:                    "fakeSnapshotPolicy",
		SnapshotReserve:                   "fakeSnapshotReserve",
		SplitOnClone:                      "false",
		UnixPermissions:                   "777",
		SnapshotDir:                       "FaLsE",
		ExportPolicy:                      "fakeExportPolicy",
		SecurityStyle:                     "fakeSecurityStyle",
		FileSystemType:                    "fakeFileSystem",
		Encryption:                        "true",
		LUKSEncryption:                    "false",
		TieringPolicy:                     "fakeTieringPolicy",
		QosPolicy:                         "fakeQosPolicy",
		AdaptiveQosPolicy:                 "fakeAdaptiveQosPolicy",
		CommonStorageDriverConfigDefaults: *CommonConfigDefault,
	}
	storageDriver.Config.Storage = []drivers.OntapStorageDriverPool{
		{
			Region: "fakeRegion",
			Zone:   "fakeZone",
			SupportedTopologies: []map[string]string{
				{
					"topology.kubernetes.io/region": "us_east_1",
					"topology.kubernetes.io/zone":   "us_east_1a",
				},
			},
			OntapStorageDriverConfigDefaults: *defaults,
		},
	}
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"dummyPool1"}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{"dummyPool1": "hdd"}, nil)

	physicalPool, virtualPool, err = InitializeStoragePoolsCommon(ctx, storageDriver, pool1.Attributes(), backendName)

	assert.Nil(t, physicalPool, "Physical Pool not expected but found")
	assert.Nil(t, virtualPool, "Virtual pool not exepcted but found")
	assert.Error(t, err)

	// Test 4 - Checking whether the formatOptions are correctly updated or not.
	expectedVirtualPoolFormatOptions := "-b 4096 -E stride=256,stripe_width=16"
	expectedPhysicalPoolFormatOptions := "-F -K"
	storageDriver.Config.OntapStorageDriverPool.FormatOptions = expectedPhysicalPoolFormatOptions
	defaults = &drivers.OntapStorageDriverConfigDefaults{
		SpaceAllocation:                   "fake",
		SpaceReserve:                      "fakeSpaceReserve",
		SnapshotPolicy:                    "fakeSnapshotPolicy",
		SnapshotReserve:                   "fakeSnapshotReserve",
		SplitOnClone:                      "false",
		UnixPermissions:                   "777",
		SnapshotDir:                       "TRUE",
		ExportPolicy:                      "fakeExportPolicy",
		SecurityStyle:                     "fakeSecurityStyle",
		FileSystemType:                    "fakeFileSystem",
		Encryption:                        "true",
		LUKSEncryption:                    "false",
		TieringPolicy:                     "fakeTieringPolicy",
		QosPolicy:                         "fakeQosPolicy",
		AdaptiveQosPolicy:                 "fakeAdaptiveQosPolicy",
		FormatOptions:                     expectedVirtualPoolFormatOptions,
		CommonStorageDriverConfigDefaults: *CommonConfigDefault,
	}
	storageDriver.Config.Storage = []drivers.OntapStorageDriverPool{
		{
			Region: "fakeRegion",
			Zone:   "fakeZone",
			SupportedTopologies: []map[string]string{
				{
					"topology.kubernetes.io/region": "us_east_1",
					"topology.kubernetes.io/zone":   "us_east_1a",
				},
			},
			OntapStorageDriverConfigDefaults: *defaults,
			NASType:                          sa.NFS,
		},
	}
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"dummyPool1"}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{"dummyPool1": "hdd"}, nil)

	physicalPool, virtualPool, err = InitializeStoragePoolsCommon(ctx, storageDriver, pool1.Attributes(), backendName)

	assert.NotNil(t, physicalPool, "Physical Pool not found when expected")
	assert.NotNil(t, virtualPool, "Virtual Pool not found when expected")
	assert.NoError(t, err)
	for _, phyPool := range physicalPool {
		assert.Equal(t, expectedPhysicalPoolFormatOptions, phyPool.InternalAttributes()[FormatOptions])
	}
	for _, vPool := range virtualPool {
		assert.Equal(t, expectedVirtualPoolFormatOptions, vPool.InternalAttributes()[FormatOptions])
	}

	// Test 5 - Check for SANType
	storageDriver.Config.Storage = []drivers.OntapStorageDriverPool{
		{
			SANType: sa.ISCSI,
		},
	}

	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"dummyPool1"}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(map[string]string{"dummyPool1": "hdd"}, nil)

	physicalPool, virtualPool, err = InitializeStoragePoolsCommon(ctx, storageDriver, pool1.Attributes(), backendName)

	assert.Nil(t, physicalPool, "Physical Pool not expected but found")
	assert.Nil(t, virtualPool, "Virtual pool not exepcted but found")
	assert.Error(t, err)
}

func TestValidateDataLIF(t *testing.T) {
	ctx := context.Background()
	dataLIFs := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "::1", "127.0.0.1"}

	// Test1 : Invalid dataLIF passed
	dataLIF := "invalidHostName"

	addresses, err := ValidateDataLIF(ctx, dataLIF, dataLIFs)

	assert.Nil(t, addresses, "Unexepected response received")
	assert.Error(t, err)

	// Test2 : Valid dataLIF passed
	dataLIF = "localhost"

	addresses, err = ValidateDataLIF(ctx, dataLIF, dataLIFs)

	assert.NotNil(t, addresses, "Unexepected response received")
	assert.NoError(t, err)
}

func TestValidateStoragePrefixEconomy(t *testing.T) {
	// Test1: Valid storage prefix
	storagePrefix := "this-is-a-valid-prefix"

	err := ValidateStoragePrefixEconomy(storagePrefix)

	assert.NoError(t, err)

	// Test2: Invalid storage prefix
	storagePrefix = "this is an invalid prefix"

	err = ValidateStoragePrefixEconomy(storagePrefix)

	assert.Error(t, err)
}

func TestCheckAggregateLimitsForFlexvol(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	flexVol := "fakeFlexVol"
	requestedSizeInt := 10000
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}
	volInfo := &api.Volume{
		Aggregates:   []string{},
		SpaceReserve: "10",
	}
	// Test1: Aggregate not found
	mockAPI.EXPECT().VolumeInfo(ctx, flexVol).Return(volInfo, nil)

	err := checkAggregateLimitsForFlexvol(ctx, flexVol, uint64(requestedSizeInt), *config, mockAPI)

	assert.Error(t, err)

	// Test2: VolInfo not found
	mockAPI.EXPECT().VolumeInfo(ctx, flexVol).Return(volInfo, fmt.Errorf("Error returned while getting volume info"))

	err = checkAggregateLimitsForFlexvol(ctx, flexVol, uint64(requestedSizeInt), *config, mockAPI)

	assert.Error(t, err)

	// Test3: Positive flow
	volInfo.Aggregates = []string{"fakeAggregate1"}
	mockAPI.EXPECT().VolumeInfo(ctx, flexVol).Return(volInfo, nil)

	err = checkAggregateLimitsForFlexvol(ctx, flexVol, uint64(requestedSizeInt), *config, mockAPI)

	assert.NoError(t, err)
}

func TestPopulateConfigurationDefaults(t *testing.T) {
	ctx := context.Background()

	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}

	// Test : Check for SANType case
	config.SANType = "ISCSI"
	err := PopulateConfigurationDefaults(ctx, config)
	assert.NoError(t, err)

	// Test1 - Positive flow with NASType SMB
	config.NASType = sa.SMB
	config.Size = "10000"

	err = PopulateConfigurationDefaults(ctx, config)

	assert.NoError(t, err)

	// Test2 - Positive flow with NASType NFS
	config.NASType = sa.NFS
	config.Size = "10000"

	err = PopulateConfigurationDefaults(ctx, config)

	assert.NoError(t, err)

	// Test3 - Invalid value of Size
	config.Size = "XYZ"

	err = PopulateConfigurationDefaults(ctx, config)

	assert.Error(t, err)

	// Test4 - Snapshot policy other than "none"
	config.Size = "10000"
	config.SnapshotPolicy = "dummy"

	err = PopulateConfigurationDefaults(ctx, config)

	assert.Equal(t, DefaultSnapshotReserve, config.SnapshotReserve)
	assert.NoError(t, err)

	// Test5 - AutoExportPolicy is set
	config.SnapshotPolicy = "none"
	config.AutoExportPolicy = true
	config.DriverContext = "csi"
	expected := "<automatic>"

	err = PopulateConfigurationDefaults(ctx, config)

	assert.Equal(t, expected, config.ExportPolicy)
	assert.NoError(t, err)

	// Test6 - Driver context other than "csi"
	config.SnapshotPolicy = "none"
	config.AutoExportPolicy = false
	config.DriverContext = "docker"
	expected = "-o nfsvers=3"

	err = PopulateConfigurationDefaults(ctx, config)

	assert.Equal(t, expected, config.NfsMountOptions)
	assert.NoError(t, err)

	// Test7 - Invalid value of Split on Clone
	config.DriverContext = "csi"
	config.SplitOnClone = "xyz"

	err = PopulateConfigurationDefaults(ctx, config)

	assert.Error(t, err)

	// Test 8 - Invalid value for cloneSplitDelay
	config.SplitOnClone = "true"
	config.CloneSplitDelay = "-123"
	err = PopulateConfigurationDefaults(ctx, config)
	assert.Error(t, err)

	config.SplitOnClone = ""
	config.CloneSplitDelay = ""

	// Test 9a - ext3 / verifying that the correct formatOptions are applied or not
	config.FileSystemType = "ext3"
	err = PopulateConfigurationDefaults(ctx, config)
	assert.NoError(t, err)
	assert.Equal(t, DefaultExt3FormatOptions, config.FormatOptions)

	// Test 9b - ext4 / verifying that the correct formatOptions are applied or not

	config.FileSystemType = "ext4"
	err = PopulateConfigurationDefaults(ctx, config)
	assert.NoError(t, err)
	assert.Equal(t, DefaultExt4FormatOptions, config.FormatOptions)

	// Test 9c - xfs / verifying that the correct formatOptions are applied or not
	config.FileSystemType = "xfs"
	err = PopulateConfigurationDefaults(ctx, config)
	assert.NoError(t, err)
	assert.Equal(t, DefaultXfsFormatOptions, config.FormatOptions)

	// test 10 - Invalid value for denyNewVolumePools
	config.SplitOnClone = "false"
	config.CloneSplitDelay = "123"
	config.DenyNewVolumePools = "asdf"
	err = PopulateConfigurationDefaults(ctx, config)
	assert.Error(t, err)
}

func TestNewOntapTelemetry(t *testing.T) {
	ctx := context.Background()
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	storageDriver := newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName, tridentconfig.DriverContext("CSI"),
		false, nil)

	// Test-1 : Valid value of UsageHeartBeat
	storageDriver.Config.UsageHeartbeat = "3.0"

	telemetry := NewOntapTelemetry(ctx, storageDriver)

	assert.Equal(t, storageDriver.Name(), telemetry.Plugin)

	// Test-2 : Invalid value of UsageHeartBeat
	storageDriver.Config.UsageHeartbeat = "XYZ"

	telemetry = NewOntapTelemetry(ctx, storageDriver)

	assert.Equal(t, storageDriver.Name(), telemetry.Plugin)
}

func MockGetVolumeInfo(_ context.Context, volName string) (volume *api.Volume, err error) {
	volume = &api.Volume{
		Name:            volName,
		SnapshotPolicy:  "fakePolicy",
		SnapshotReserve: 10,
	}
	return volume, nil
}

func TestGetSnapshotReserveFromOntap(t *testing.T) {
	ctx := context.Background()
	volName := "fakeVolName"
	expected := 10
	// Test1: Positive flow
	snapshotReserveInt, err := getSnapshotReserveFromOntap(ctx, volName, MockGetVolumeInfo)

	assert.Equal(t, expected, snapshotReserveInt)
	assert.NoError(t, err)
}

func TestGetISCSIDataLIFsForReportingNodes(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}
	lunPath := "dummyLunPath"
	iGroupName := "dummyIgroup"
	unmanagedimport := false

	// Test 1: No ips passed in
	ips = []string{}

	reportedNodes, err := getISCSIDataLIFsForReportingNodes(ctx, mockAPI, ips, lunPath, iGroupName, unmanagedimport)

	assert.Nil(t, reportedNodes)
	assert.Error(t, err)

	// Test2: unmanagedImport = false and reportedNodes = {}
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, iGroupName, lunPath).Return([]string{}, nil)
	ips = []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}

	reportedNodes, err = getISCSIDataLIFsForReportingNodes(ctx, mockAPI, ips, lunPath, iGroupName, unmanagedimport)

	assert.Nil(t, reportedNodes)
	assert.Error(t, err)

	// Test3: No dataLIFs reported by GetSLMDataLifs
	reportedNodes = []string{"fakeNode1"}
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, iGroupName, lunPath).Return(reportedNodes, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, ips, reportedNodes).Return([]string{"fakeNode1"},
		fmt.Errorf("Error getting DataLIFs"))
	ips = []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}

	reportedNodes, err = getISCSIDataLIFsForReportingNodes(ctx, mockAPI, ips, lunPath, iGroupName, unmanagedimport)

	assert.Nil(t, reportedNodes)
	assert.Error(t, err)
}

func TestInitializeOntapConfig(t *testing.T) {
	ctx := context.Background()
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}
	config.UseCHAP = true

	driverContext := "csi"
	configJSON := []byte{}

	backendSecret := map[string]string{"dummy": "fake"}

	// Test1: Invalid JSON
	configReturned, err := InitializeOntapConfig(ctx, tridentconfig.DriverContext(driverContext), string(configJSON),
		commonConfig, backendSecret)

	assert.Nil(t, configReturned)
	assert.Error(t, err)

	// Test2:  Invalid secrect
	configJSON, _ = json.Marshal(config)
	configReturned, err = InitializeOntapConfig(ctx, tridentconfig.DriverContext(driverContext), string(configJSON),
		commonConfig, backendSecret)

	assert.Nil(t, configReturned)
	assert.Error(t, err)

	// Test3:  Invalid config.Size
	backendSecret = map[string]string{
		"chapusername":              "fake",
		"chapinitiatorsecret":       "fake",
		"chaptargetusername":        "fake",
		"chaptargetinitiatorsecret": "fake",
	}
	config.ClientPrivateKey = "fake"
	config.Size = "InvalidSize"
	configJSON, _ = json.Marshal(config)

	configReturned, err = InitializeOntapConfig(ctx, tridentconfig.DriverContext(driverContext), string(configJSON),
		commonConfig, backendSecret)

	assert.Nil(t, configReturned, "Unexpected output received")
	assert.Error(t, err)
}

func TestCalculateFlexvolEconomySizeBytes(t *testing.T) {
	ctx := context.Background()
	flexvol := "fakeName"
	volAttr := &api.Volume{
		SnapshotReserve:   1,
		SnapshotSpaceUsed: 2000,
	}
	newLunOrQtreeSizeBytes := 10000
	totalDiskLimitBytes := 50000

	// Test 1- usableSpaceSnapReserve is less than usableSpaceWithSnapshots
	expected := uint64(totalDiskLimitBytes + newLunOrQtreeSizeBytes + volAttr.SnapshotSpaceUsed)
	flexvolBytes := calculateFlexvolEconomySizeBytes(ctx, flexvol, volAttr, uint64(newLunOrQtreeSizeBytes),
		uint64(totalDiskLimitBytes))

	assert.Equal(t, expected, flexvolBytes)

	// Test 2- usableSpaceSnapReserve is more than usableSpaceWithSnapshots
	volAttr.SnapshotReserve = 70
	expected = uint64((float64)(newLunOrQtreeSizeBytes+totalDiskLimitBytes) / (1.0 - (float64(volAttr.SnapshotReserve) / 100.0)))

	flexvolBytes = calculateFlexvolEconomySizeBytes(ctx, flexvol, volAttr, uint64(newLunOrQtreeSizeBytes),
		uint64(totalDiskLimitBytes))

	assert.Equal(t, expected, flexvolBytes)
}

func TestLunUnmapIgroup_FailsToListLunMapInfo(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	lunPath := "fakeLunPath"
	igroupName := "fakeigroupName"
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, fmt.Errorf("ontap api error"))

	err := LunUnmapIgroup(ctx, mockAPI, igroupName, lunPath)
	assert.Error(t, err)
}

func TestLunUnmapIgroup_ListsLunMapInfoForUnknownLunID(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	lunPath := "fakeLunPath"
	igroupName := "fakeigroupName"

	// If LUN ID (first return param here) is negative, it means that it hasn't been published anywhere.
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(-1, nil)

	err := LunUnmapIgroup(ctx, mockAPI, igroupName, lunPath)
	assert.NoError(t, err)
}

func TestLunUnmapIgroup_FailsToUnmapLunFromIgroup(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	lunPath := "fakeLunPath"
	igroupName := "fakeigroupName"

	// If LUN ID (first return param here) is negative, it means that it hasn't been published anywhere.
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(2, nil)
	mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(fmt.Errorf("ontap api error"))

	err := LunUnmapIgroup(ctx, mockAPI, igroupName, lunPath)
	assert.Error(t, err)
}

func TestLunUnmapIgroup_Succeeds(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	lunPath := "fakeLunPath"
	igroupName := "fakeigroupName"

	// If LUN ID (first return param here) is negative, it means that it hasn't been published anywhere.
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(2, nil)
	mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)

	err := LunUnmapIgroup(ctx, mockAPI, igroupName, lunPath)
	assert.NoError(t, err)
}

func TestDestroyUnmappedIgroup_FailsToListLUNsMappedToIgroup(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	igroupName := "fakeigroupName"

	// If LUN ID (first return param here) is negative, it means that it hasn't been published anywhere.
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, fmt.Errorf("ontap api error"))

	err := DestroyUnmappedIgroup(ctx, mockAPI, igroupName)
	assert.Error(t, err)
}

func TestDestroyUnmappedIgroup_FailsToDestroyEmptyIgroup(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	igroupName := "fakeigroupName"

	// If LUN ID (first return param here) is negative, it means that it hasn't been published anywhere.
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, nil)
	mockAPI.EXPECT().IgroupDestroy(ctx, igroupName).Return(fmt.Errorf("ontap api error"))

	err := DestroyUnmappedIgroup(ctx, mockAPI, igroupName)
	assert.Error(t, err)
}

func TestDestroyUnmappedIgroup_Succeeds(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	igroupName := "fakeigroupName"

	// If LUN ID (first return param here) is negative, it means that it hasn't been published anywhere.
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, nil)
	mockAPI.EXPECT().IgroupDestroy(ctx, igroupName).Return(nil)

	err := DestroyUnmappedIgroup(ctx, mockAPI, igroupName)
	assert.NoError(t, err)
}

func TestEnableSANPublishEnforcement_DoesNotEnableForUnmangedImport(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	volName := "websterj_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
	internalVolName := "pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
	lunPath := fmt.Sprintf("/vol/myBucket/storagePrefix_vol1_%s", internalVolName)
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:         volName,
			InternalName: internalVolName,
			AccessInfo: tridentmodels.VolumeAccessInfo{
				PublishEnforcement: false,
				IscsiAccessInfo: tridentmodels.IscsiAccessInfo{
					IscsiLunNumber: 1,
				},
			},
			ImportNotManaged: true,
		},
	}

	err := EnableSANPublishEnforcement(ctx, mockAPI, volume.Config, lunPath)
	assert.NoError(t, err)
	assert.False(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.NotEqual(t, -1, volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestEnableSANPublishEnforcement_FailsToUnmapLunFromAllIgroups(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	volName := "websterj_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
	internalVolName := "pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
	lunPath := fmt.Sprintf("/vol/myBucket/storagePrefix_vol1_%s", internalVolName)
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:         volName,
			InternalName: internalVolName,
			AccessInfo: tridentmodels.VolumeAccessInfo{
				PublishEnforcement: false,
				IscsiAccessInfo: tridentmodels.IscsiAccessInfo{
					IscsiLunNumber: 1,
				},
			},
			ImportNotManaged: false,
		},
	}
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return(nil, fmt.Errorf("ontap api error"))

	err := EnableSANPublishEnforcement(ctx, mockAPI, volume.Config, lunPath)
	assert.Error(t, err)
	assert.False(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.NotEqual(t, -1, volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestEnableSANPublishEnforcement_Succeeds(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	volName := "websterj_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
	internalVolName := "pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
	lunPath := fmt.Sprintf("/vol/myBucket/storagePrefix_vol1_%s", internalVolName)
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:         volName,
			InternalName: internalVolName,
			AccessInfo: tridentmodels.VolumeAccessInfo{
				PublishEnforcement: false,
				IscsiAccessInfo: tridentmodels.IscsiAccessInfo{
					IscsiLunNumber: 1,
				},
			},
			ImportNotManaged: false,
		},
	}
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return(nil, nil)

	err := EnableSANPublishEnforcement(ctx, mockAPI, volume.Config, lunPath)
	assert.NoError(t, err)
	assert.True(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.Equal(t, int32(-1), volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestGetNodeSpecificIgroup(t *testing.T) {
	tests := map[string]struct {
		node, uuid string
		truncate   bool
	}{
		"get igroup name does not truncate with short node names": {
			node:     "node12345678910.my.fqdn",
			uuid:     "b11ad8a0-f182-420f-b00e-d82ce9d80962",
			truncate: false,
			// node12345678910.my.fqdn-b11ad8a0-f182-420f-b00e-d82ce9d80962
		},
		"get igroup name does not truncate when generated igroup name < max igroup length": {
			node:     "node12345678910.my.fqdn-12345678910-12345678910-1234567891",
			uuid:     "b11ad8a0-f182-420f-b00e-d82ce9d80962",
			truncate: false,
			// node12345678910.my.fqdn-12345678910-12345678910-1234567891-b11ad8a0-f182-420f-b00e-d82ce9d80962
		},
		"get igroup name does not truncate when generated igroup name = max igroup length": {
			node:     "node12345678910.my.fqdn-12345678910-12345678910-12345678910",
			uuid:     "b11ad8a0-f182-420f-b00e-d82ce9d80962",
			truncate: false,
			// node12345678910.my.fqdn-12345678910-12345678910-12345678910-b11ad8a0-f182-420f-b00e-d82ce9d80962
		},
		"get igroup name does truncate when node is < max igroup length and generated igroup name > max igroup length": {
			node:     "node12345678910.my.fqdn-12345678910-12345678910-12345678910-12345678910-12345678910-12345678910",
			uuid:     "b11ad8a0-f182-420f-b00e-d82ce9d80962",
			truncate: true,
			// node12345678910.my.fqdn-12345678910-12345678910-12345678910-b11ad8a0-f182-420f-b00e-d82ce9d80962
		},
		"get igroup name does truncate when node is > max igroup length and generated igroup name > max igroup length": {
			node:     "node12345678910.my.fqdn-12345678910-12345678910-12345678910-12345678910-12345678910-123456789101",
			uuid:     "b11ad8a0-f182-420f-b00e-d82ce9d80962",
			truncate: true,
			// node12345678910.my.fqdn-12345678910-12345678910-12345678910-b11ad8a0-f182-420f-b00e-d82ce9d80962
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			igroup := getNodeSpecificIgroupName(test.node, test.uuid)
			assert.LessOrEqual(t, len(igroup), MaximumIgroupNameLength)
			assert.Contains(t, igroup, test.uuid)
			if test.truncate {
				assert.NotContains(t, igroup, test.node)
			} else {
				assert.Contains(t, igroup, test.node)
			}
		})
	}
}

func TestRemoveIgroupFromList(t *testing.T) {
	tests := []struct {
		message             string
		Igroup              string
		IscsiIgroupNameList string
		ExpectedOutput      string
	}{
		{
			"remove Igroup", "fakeigroupName2", "fakeigroupName1,fakeigroupName2",
			"fakeigroupName1",
		},
		{
			"IscsiIgroupNameList size 1", "fakeigroupName1", "fakeigroupName1",
			"",
		},
		{
			"Empty IscsiIgroupName", "", "fakeigroupName1,fakeigroupName2",
			"fakeigroupName1,fakeigroupName2",
		},
		{
			"remove Igroup from middle of string", "fakeigroupName2", "fakeigroupName1,fakeigroupName2,fakeigroupName3",
			"fakeigroupName1,fakeigroupName3",
		},
		{"Empty IscsiIgroupNameList", "fakeigroupName2", "", ""},
	}

	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			IgroupList := removeIgroupFromIscsiIgroupList(test.IscsiIgroupNameList, test.Igroup)
			assert.Equal(t, test.ExpectedOutput, IgroupList)
		})
	}
}

func TestGetSVMState(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	var derivedPoolsNil []string
	var aggrsNil []string
	dataLIFs := []string{"1.2.3.4"}
	derivedPools := []string{"aggr1"}
	existingPools := []string{"aggr1"}

	// Format for every test:
	// Calls needed.
	// NewLine
	// Test itself.

	// Test 1
	// a: API.GetSVMState returns error
	mockAPI.EXPECT().GetSVMState(ctx).Return("TestStateInvalid", fmt.Errorf("GetSVMState returned error"))

	state, code := getSVMState(ctx, mockAPI, sa.ISCSI, derivedPoolsNil, aggrsNil...)
	assert.Equal(t, StateReasonSVMUnreachable, state, "state returned should be TestStateUnknown")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should not be pool change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")

	// Test 2
	// a: SVM is stopped
	mockAPI.EXPECT().GetSVMState(ctx).Return(models.SvmStateStopped, nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, derivedPoolsNil, aggrsNil...)
	assert.Equal(t, StateReasonSVMStopped, state, "state should be SVM stopped")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should not be pool change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")

	mockAPI.EXPECT().GetSVMState(ctx).Return(models.SvmStateRunning, nil).AnyTimes()

	// Test 3
	// a: client.GetSVMAggregateNames returns error
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(derivedPoolsNil, fmt.Errorf("API call returned error")).Times(1)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.14.1", nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, derivedPoolsNil, aggrsNil...)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should not be pool change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")

	// b: client.GetSVMAggregateNames returns empty list
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(nil, nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, []string{"aggr1"}, aggrsNil...)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.True(t, code.Contains(storage.BackendStatePoolsChange), "Should be a pool change")
	assert.Equal(t, StateReasonNoAggregates, state, "Reason should be StateReasonNoAggregates")

	// c: client.GetSVMAggregateNames returns non-empty list, config.Aggregate is missing
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"aggr1", "aggr2"}, nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, []string{"aggr1"}, []string{"aggr3"}...)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.True(t, code.Contains(storage.BackendStatePoolsChange), "Should be a pool change")
	assert.Equal(t, StateReasonMissingAggregate, state, "Reason should be equal to StateReasonMissingAggregate")

	// d: client.GetSVMAggregateNames returns non-empty list, config.Aggregate is present, also no change in pool
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"aggr1"}, nil).Times(1)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.14.1", nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, []string{"aggr1"}, []string{"aggr1"}...)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, "", state, "Reason should be empty")

	// e: client.GetSVMAggregateNames returns non-empty list, config.Aggregate is "", also no change in pool
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"aggr1"}, nil).Times(1)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.14.1", nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, []string{"aggr1"}, "")
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, "", state, "Reason should be empty")

	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(derivedPools, nil).AnyTimes()

	// Test 4
	// a: error getting data LIFs
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, fmt.Errorf("API call returned error")).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, existingPools)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, StateReasonDataLIFsDown, state, "Reason should be equal to StateReasonDataLIFsDown")

	// b: no data LIFs in up state
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(nil, nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, existingPools)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, StateReasonDataLIFsDown, state, "Reason should be equal to StateReasonDataLIFsDown")

	// c: all well
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.14.1", nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, existingPools)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, "", state, "Reason should be empty")

	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil).AnyTimes()

	// d: error getting FC data LIFs
	mockAPI.EXPECT().NetFcpInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs,
		fmt.Errorf("API call returned error")).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.FCP, existingPools)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, StateReasonDataLIFsDown, state, "Reason should be equal to StateReasonDataLIFsDown")

	// e: no FC data LIFs in up state
	mockAPI.EXPECT().NetFcpInterfaceGetDataLIFs(ctx, gomock.Any()).Return(nil, nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.FCP, existingPools)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, StateReasonDataLIFsDown, state, "Reason should be equal to StateReasonDataLIFsDown")

	// f: all well with FC.
	mockAPI.EXPECT().NetFcpInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.14.1", nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.FCP, existingPools)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, "", state, "Reason should be empty")

	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil).AnyTimes()

	// Test 5
	// a: current ONTAP version > cached ONTAP version
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.15.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, existingPools)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.True(t, code.Contains(storage.BackendStateAPIVersionChange), "Should reflect change in ONTAP version")

	// b: current ONTAP version = cached ONTAP version
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.14.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, existingPools)
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should reflect NO change in ONTAP version")

	// c: current ONTAP version < cached ONTAP version
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.13.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, existingPools)
	assert.True(t, code.Contains(storage.BackendStateAPIVersionChange), "Should reflect change in ONTAP version")

	// d: error in fetching the api version
	mockAPI.EXPECT().APIVersion(ctx, true).Return("", fmt.Errorf("API call returned error")).Times(1)

	state, code = getSVMState(ctx, mockAPI, sa.ISCSI, existingPools)
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")

	// Creating a ZAPI mock client, to test the same scenario as above, just specific to ZAPI.
	// e: current ONTAP version > cached ONTAP version
	mockZapiClient := mockapi.NewMockZapiClientInterface(mockCtrl)
	mockONTAPZAPI, _ := api.NewOntapAPIZAPIFromZapiClientInterface(mockZapiClient)
	mockZapiClient.EXPECT().GetSVMState(gomock.Any()).Return(models.SvmStateRunning, nil)
	mockZapiClient.EXPECT().SVMGetAggregateNames().Return(derivedPools, nil)
	mockZapiClient.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil)
	mockZapiClient.EXPECT().SystemGetOntapiVersion(ctx, true).Return("1.241", nil)
	mockZapiClient.EXPECT().SystemGetOntapiVersion(ctx, false).Return("1.251", nil)

	state, code = getSVMState(ctx, mockONTAPZAPI, sa.ISCSI, existingPools)
	assert.True(t, code.Contains(storage.BackendStateAPIVersionChange), "Should reflect change in ONTAP version")

	// f: current ONTAP version < cached ONTAP version
	mockZapiClient.EXPECT().GetSVMState(gomock.Any()).Return(models.SvmStateRunning, nil)
	mockZapiClient.EXPECT().SVMGetAggregateNames().Return(derivedPools, nil)
	mockZapiClient.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil)
	mockZapiClient.EXPECT().SystemGetOntapiVersion(ctx, false).Return("1.241", nil)
	mockZapiClient.EXPECT().SystemGetOntapiVersion(ctx, true).Return("1.251", nil)

	state, code = getSVMState(ctx, mockONTAPZAPI, sa.ISCSI, existingPools)
	assert.True(t, code.Contains(storage.BackendStateAPIVersionChange), "Should reflect change in ONTAP version")

	// g: current ONTAP version = cached ONTAP version
	mockZapiClient.EXPECT().GetSVMState(gomock.Any()).Return(models.SvmStateRunning, nil)
	mockZapiClient.EXPECT().SVMGetAggregateNames().Return(derivedPools, nil)
	mockZapiClient.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil)
	mockZapiClient.EXPECT().SystemGetOntapiVersion(ctx, false).Return("1.251", nil)
	mockZapiClient.EXPECT().SystemGetOntapiVersion(ctx, true).Return("1.251", nil)

	state, code = getSVMState(ctx, mockONTAPZAPI, sa.ISCSI, existingPools)
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should reflect NO change in ONTAP version")
}

func TestConstructLabelsFromConfigs(t *testing.T) {
	ctx := context.Background()

	pool := storage.NewStoragePool(nil, "dummyPool")
	volConfig := &storage.VolumeConfig{Name: "testVol"}
	storagePrefix := "trident"
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		StoragePrefix:   &storagePrefix,
	}

	_, err := ConstructLabelsFromConfigs(ctx, pool, volConfig, commonConfig, api.MaxNASLabelLength)

	assert.Nil(t, err)
}

func TestConstructLabelsFromConfigs_LablelTemplate(t *testing.T) {
	ctx := context.Background()

	pool1 := storage.NewStoragePool(nil, "dummyPool")
	pool1.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"template": `{{.volume.Name}}`,
	})
	volConfig := &storage.VolumeConfig{
		Name: "testVol",
	}
	storagePrefix := "trident"
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		StoragePrefix:   &storagePrefix,
	}

	// Test 1: Positive test
	expectedLabel := "{\"provisioning\":{\"template\":\"testVol\"}}"
	label, err := ConstructLabelsFromConfigs(ctx, pool1, volConfig, commonConfig, api.MaxSANLabelLength)

	// Test 2: Template has invalid volume field. The template execution failed.
	assert.Nil(t, err)
	assert.Equal(t, expectedLabel, label)

	pool2 := storage.NewStoragePool(nil, "dummyPool")
	pool2.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"template": `pool_{{.volume.Name}}_{{.volume.InvalidFeild}}`,
	})

	expectedLabel = "{\"provisioning\":{\"template\":\"pool_{{.volume.Name}}_{{.volume.InvalidFeild}}\"}}"
	label, err = ConstructLabelsFromConfigs(ctx, pool2, volConfig, commonConfig, api.MaxNASLabelLength)

	assert.Nil(t, err)
	assert.Equal(t, expectedLabel, label)

	// Test 3: The template is missing curly bracket. Template execution fail and label contain string in
	pool3 := storage.NewStoragePool(nil, "dummyPool")
	pool3.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"template": `{{.volume.Name}}_{{.volume.InvalidFeild`,
	})

	expectedLabel = "{\"provisioning\":{\"template\":\"{{.volume.Name}}_{{.volume.InvalidFeild\"}}"
	label, err = ConstructLabelsFromConfigs(ctx, pool3, volConfig, commonConfig, api.MaxNASLabelLength)

	assert.Nil(t, err)
	assert.Equal(t, expectedLabel, label)

	pool4 := storage.NewStoragePool(nil, "dummyPool")
	pool4.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"template": `pool_{{.volume.Name}}_{{.volume.Namespace}}_tridentSanVolumethisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
			"V88bESTQlRIWRSS40sx9ND8P9yPf0LV8jPofiqtTp2iIXgotGh83zZ1HEeFlMGxZlIcOiPdoi07cJ" +
			"bQBuHvTRNX6pHRKUXaIrjEpygM4SpaqHYdZ8O1k2meeugg7eXu4dPhqetI3Sip3W4v9QuFkh1YBaI"`,
	})
	volConfig = &storage.VolumeConfig{
		Name:      "testVol",
		Namespace: "Trident",
	}
	storagePrefix = "trident"
	commonConfig = &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		StoragePrefix:   &storagePrefix,
	}

	expectedLabel = "{\"provisioning\":{\"template\":\"testVol\"}}"
	label, err = ConstructLabelsFromConfigs(ctx, pool4, volConfig, commonConfig, api.MaxSANLabelLength)

	assert.Error(t, err)
}

func TestValidateStoragePools_Tempatizedlabels_virtualPool(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	storageDriver := newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)

	physicalPools := map[string]storage.Pool{}
	virtualPools := map[string]storage.Pool{"test": getValidOntapNASPool()}

	cases := []struct {
		Name          string
		labelValue    string
		expectedError bool
	}{
		{"stringLabel", "1_cluster_pool", false},
		{"IntLabel", "1234", false},
		{"specialCharachterLabel", "1_cluster_pool&$#", false},
		{"lableValueEmpty", "", false},
		{"ValidTemplate", "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}", false},
		{"InvalidTemplateLabel", "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName", true},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf(c.Name), func(t *testing.T) {
			virtualPools["test"].Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
				"template": c.labelValue,
			})

			storageDriver.virtualPools = virtualPools
			storageDriver.physicalPools = physicalPools

			err := ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)
			if !c.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateStoragePools_Tempatizedlabels_physicalPool(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	storageDriver := newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName,
		tridentconfig.DriverContext("CSI"), false, nil)

	virtualPools := map[string]storage.Pool{}
	physicalPools := map[string]storage.Pool{"test": getValidOntapNASPool()}

	cases := []struct {
		Name          string
		labelValue    string
		expectedError bool
	}{
		{"stringLabel", "1_cluster_pool", false},
		{"IntLabel", "1234", false},
		{"specialCharachterLabel", "1_cluster_pool&$#", false},
		{"ValidTemplate", "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}", false},
		{"InvalidTemplateLabel", "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName", true},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("test%v", i), func(t *testing.T) {
			physicalPools["test"].Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
				"template": c.labelValue,
			})

			storageDriver.virtualPools = virtualPools
			storageDriver.physicalPools = physicalPools

			err := ValidateStoragePools(context.Background(), physicalPools, virtualPools, storageDriver, 0)
			if !c.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestGetVolumeNameFromTemplate(t *testing.T) {
	ctx := context.Background()
	tridentconfig.UsingPassthroughStore = false

	storagePrefix := "trident"
	name := "pvc_123456789"
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		StoragePrefix:   &storagePrefix,
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}

	volConfig := &storage.VolumeConfig{Name: name}

	expected := "pool_dev_test_cluster_1"
	pool := getValidOntapNASPool()
	out, err := GetVolumeNameFromTemplate(ctx, config, volConfig, pool)

	assert.NoError(t, err, "Error is not nil, expected no error")
	assert.Equal(t, expected, out)
}

func TestGetVolumeNameFromTemplate_invalidNameTemplate(t *testing.T) {
	ctx := context.Background()
	tridentconfig.UsingPassthroughStore = false

	storagePrefix := "trident"
	name := "pvc-123456"
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		StoragePrefix:   &storagePrefix,
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}

	volConfig := &storage.VolumeConfig{Name: name}

	expected := ""
	pool := getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_{{.volume.Invalid}}",
		},
	)
	out, err := GetVolumeNameFromTemplate(ctx, config, volConfig, pool)

	assert.Error(t, err, "Error is nil, expected an error")
	assert.Equal(t, expected, out)

	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_{{.volume.Name",
		},
	)
	out, err = GetVolumeNameFromTemplate(ctx, config, volConfig, pool)

	assert.Error(t, err, "Error is nil, expected an error")
	assert.Equal(t, expected, out)
}

func TestGetVolumeNameFromTemplateWithLabel(t *testing.T) {
	ctx := context.Background()
	tridentconfig.UsingPassthroughStore = false

	storagePrefix := "trident"
	name := "pvc_123456789"
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		StoragePrefix:   &storagePrefix,
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}
	expected := "tridentFake"
	volConfig := &storage.VolumeConfig{Name: name, Namespace: "trident", RequestName: "pvc"}

	expected = "pool_pvc_123456789_trident_trident_pvc"
	pool := getValidOntapNASPool()
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    `{{.volume.Name}}_{{.volume.Namespace}}`,
	})
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_{{.labels.template}}_{{.volume.Namespace}}_{{.volume.RequestName}}",
		},
	)
	out, err := GetVolumeNameFromTemplate(ctx, config, volConfig, pool)

	assert.NoError(t, err, "Error is not nil, expected no error")
	assert.Equal(t, expected, out)
}

func TestGetVolumeNameFromTemplate_NameStartWithDigit(t *testing.T) {
	ctx := context.Background()
	tridentconfig.UsingPassthroughStore = false

	storagePrefix := "trident"
	name := "pvc_123456789"
	commonConfig := &drivers.CommonStorageDriverConfig{
		DebugTraceFlags: map[string]bool{"method": true},
		StoragePrefix:   &storagePrefix,
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}

	volConfig := &storage.VolumeConfig{Name: name, Namespace: "trident", RequestName: "pvc-nas"}

	// Test 1: Name template starts with a digit.
	pool := getValidOntapNASPool()

	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "1234_{{.volume.RequestName}}",
		},
	)
	out, err := GetVolumeNameFromTemplate(ctx, config, volConfig, pool)

	assert.Error(t, err, "Error is nil, volume name should not start with number")

	// Test 2: Generated name start with digit.
	pool = getValidOntapNASPool()
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cluster": "1_cluster",
	})
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "{{.labels.cluster}}",
		},
	)
	out, err = GetVolumeNameFromTemplate(ctx, config, volConfig, pool)

	assert.Error(t, err, "Error is nil, volume name should not start with number")

	// Test 3: user defined template is empty. The pvc UUID start with digit
	pool = getValidOntapNASPool()
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cluster": "1_cluster",
	})
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "{{.labels.NotExist}}_{{slice .volume.Name 4 9}}",
		},
	)
	out, err = GetVolumeNameFromTemplate(ctx, config, volConfig, pool)

	expected := "tridentFake"
	assert.Error(t, err, "Error is nil, volume name should not start with number")

	// Test 4: user defined template is empty. The pvc UUID start with a letter
	volConfig = &storage.VolumeConfig{Name: "pvc-a1b2c3", RequestName: "pvc-nas"}
	pool = getValidOntapNASPool()
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cluster": "1_cluster",
	})
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "{{.labels.NotExist}}_{{slice .volume.Name 4 9}}",
		},
	)
	out, err = GetVolumeNameFromTemplate(ctx, config, volConfig, pool)

	expected = "a1b2c"
	assert.NoError(t, err, "Error is not nil, expected no error")
	assert.Equal(t, expected, out)
}

func TestConstructPoolForLabels(t *testing.T) {
	nameTemplate := "testTemplate"
	labels := map[string]string{
		"label1": "value1",
		"label2": "value2",
	}

	pool := ConstructPoolForLabels(nameTemplate, labels)

	assert.Equal(t, nameTemplate, pool.InternalAttributes()[NameTemplate])
	assert.Equal(t, sa.NewLabelOffer(labels), pool.Attributes()["labels"])

	pool = ConstructPoolForLabels("", labels)
	assert.Equal(t, sa.NewLabelOffer(labels), pool.Attributes()["labels"])

	pool = ConstructPoolForLabels("", nil)
	assert.Equal(t, sa.NewLabelOffer(nil), pool.Attributes()["labels"])
}

func TestSubtractUintFromSizeString(t *testing.T) {
	units := []string{"", "MB", "MiB", "GB", "GiB"}
	count := 100
	maxValue := int64(1000000000)

	// Test for invalid size string
	_, err := subtractUintFromSizeString("not a size", 0)
	assert.ErrorContains(t, err, "invalid size")

	// Fuzz tests
	for range count {
		sizeValue := rand.Int63n(maxValue + 1)
		size := strconv.FormatInt(sizeValue, 10) + units[rand.Intn(len(units))]
		val := uint64(rand.Int63n(maxValue))

		sizeBytesString, err := capacity.ToBytes(size)
		assert.NoError(t, err)
		sizeBytes, _ := strconv.ParseUint(sizeBytesString, 10, 64)

		newSizeBytesString, err := subtractUintFromSizeString(size, val)

		if val > sizeBytes {
			assert.ErrorContains(t, err, "too large")
		} else {
			assert.NoError(t, err)

			expected := strconv.FormatUint(sizeBytes-val, 10)
			assert.Equal(t, expected, newSizeBytesString)
		}
	}
}

func TestIncrementWithLUKSMetadataIfLUKSEnabled(t *testing.T) {
	size := uint64(1000000000)

	actual := incrementWithLUKSMetadataIfLUKSEnabled(context.Background(), size, "true")
	assert.Greater(t, actual, size)

	actual = incrementWithLUKSMetadataIfLUKSEnabled(context.Background(), size, "false")
	assert.Equal(t, actual, size)

	actual = incrementWithLUKSMetadataIfLUKSEnabled(context.Background(), size, "blue")
	assert.Equal(t, actual, size)
}

func TestDecrementWithLUKSMetadataIfLUKSEnabled(t *testing.T) {
	size := uint64(1000000000)

	actual := decrementWithLUKSMetadataIfLUKSEnabled(context.Background(), size, "true")
	assert.Less(t, actual, size)

	actual = decrementWithLUKSMetadataIfLUKSEnabled(context.Background(), size, "false")
	assert.Equal(t, actual, size)

	actual = decrementWithLUKSMetadataIfLUKSEnabled(context.Background(), size, "blue")
	assert.Equal(t, actual, size)
}

func TestDeleteAutomaticSnapshot(t *testing.T) {
	type parameters struct {
		volConfig                             storage.VolumeConfig
		volDeleteError                        error
		expectAPISnapshotDeleteFunctionCalled bool
		snapshotDeleteError                   error
	}

	volConfig := getVolumeConfig()

	volConfigWithInternalSnapshot := getVolumeConfig()
	volConfigWithInternalSnapshot.CloneSourceSnapshotInternal = "snapshot-internal"

	tests := map[string]parameters{
		"volume config without internal snapshot": {
			volConfig:                             volConfig,
			expectAPISnapshotDeleteFunctionCalled: false,
		},
		"volume config with internal snapshot": {
			volConfig:                             volConfigWithInternalSnapshot,
			expectAPISnapshotDeleteFunctionCalled: true,
		},
		"volume config with internal snapshot: volume delete error": {
			volConfig:                             volConfigWithInternalSnapshot,
			expectAPISnapshotDeleteFunctionCalled: false,
			volDeleteError:                        fmt.Errorf("volume delete error"),
		},
		"volume config with internal snapshot: snapshot delete error": {
			volConfig:                             volConfigWithInternalSnapshot,
			expectAPISnapshotDeleteFunctionCalled: true,
			snapshotDeleteError:                   fmt.Errorf("snapshot delete error"),
		},
		"volume config with internal snapshot: snapshot not found": {
			volConfig:                             volConfigWithInternalSnapshot,
			expectAPISnapshotDeleteFunctionCalled: true,
			snapshotDeleteError:                   api.NotFoundError("snapshot not found"),
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			snapshotDeleteAPIFunctionCalled := false
			mockAPISnapshotDeleteFunction := func(ctx context.Context, s, s2 string) error {
				snapshotDeleteAPIFunctionCalled = true
				return params.snapshotDeleteError
			}
			_, storageDriver := newMockOntapNASDriver(t)

			deleteAutomaticSnapshot(context.Background(), storageDriver, params.volDeleteError,
				&params.volConfig, mockAPISnapshotDeleteFunction)

			assert.Equal(t, params.expectAPISnapshotDeleteFunctionCalled, snapshotDeleteAPIFunctionCalled)
		})
	}
}
