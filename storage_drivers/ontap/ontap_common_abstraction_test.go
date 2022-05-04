// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	mock_api "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/stretchr/testify/assert"
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

	apiErr := api.GetErrorAbstraction(ctx, nil, snapListResponseErr)
	assert.NotNil(t, apiErr)
	assert.Equal(t, "API error: nil response", apiErr.Error())

	apiErr = api.GetErrorAbstraction(ctx, APIResponsePassed, nil)
	assert.Nil(t, apiErr)

	apiErr = api.GetErrorAbstraction(ctx, APIResponsePassed, nil)
	assert.Nil(t, apiErr)

	apiErr = api.GetErrorAbstraction(ctx, APIResponseFailed, nil)
	assert.NotNil(t, apiErr)
	assert.Equal(t, "API error: &{ failed reason errno}", apiErr.Error())
}

func TestZapiGetSVMAggregateSpace(t *testing.T) {
	ctx := context.Background()

	aggr := "aggr1"

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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
	assert.Equal(t, "could not determine aggregate space, cannot check aggregate provisioning limits for aggr1", err.Error())
	assert.Equal(t, 0, len(result))

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

func TestRestGetSVMAggregateSpace(t *testing.T) {
	ctx := context.Background()

	aggr := "aggr1"

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

func newMockOntapAPI(t *testing.T) *mock_api.MockOntapAPI {
	mockCtrl := gomock.NewController(t)
	mockOntapAPI := mock_api.NewMockOntapAPI(mockCtrl)
	return mockOntapAPI
}

func TestCheckAggregateLimitsAbstraction(t *testing.T) {
	ctx := context.Background()

	aggr := "aggr1"
	ontapConfig := *newTestOntapDriverConfig(ONTAPTEST_LOCALHOST, "443", aggr)
	ontapConfig.SVM = "svm"
	ontapConfig.LimitAggregateUsage = "95%"
	ontapConfig.Aggregate = aggr

	spaceReserve := "none"
	requestedSizeInt := 1

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  nil result
	mockOntapAPI := newMockOntapAPI(t)
	mockOntapAPI.EXPECT().GetSVMAggregateSpace(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, aggregate string) ([]api.SVMAggregateSpace, error) {
			return nil, nil
		},
	).AnyTimes()

	err := checkAggregateLimitsAbstraction(ctx, aggr, spaceReserve, uint64(requestedSizeInt), ontapConfig, mockOntapAPI)
	assert.Equal(t, "could not find aggregate, cannot check aggregate provisioning limits for aggr1", err.Error())

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  empty result
	mockOntapAPI = newMockOntapAPI(t)
	mockOntapAPI.EXPECT().GetSVMAggregateSpace(gomock.Any(), aggr).DoAndReturn(
		func(ctx context.Context, aggregate string) ([]api.SVMAggregateSpace, error) {
			result := []api.SVMAggregateSpace{}
			return result, nil
		},
	).AnyTimes()

	err = checkAggregateLimitsAbstraction(ctx, aggr, spaceReserve, uint64(requestedSizeInt), ontapConfig, mockOntapAPI)
	assert.Equal(t, "could not find aggregate, cannot check aggregate provisioning limits for aggr1", err.Error())

	//////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

	err = checkAggregateLimitsAbstraction(ctx, aggr, spaceReserve, uint64(requestedSizeInt), ontapConfig, mockOntapAPI)
	assert.Nil(t, err)
}
