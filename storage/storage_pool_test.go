// Copyright 2020 NetApp, Inc. All Rights Reserved.

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	sa "github.com/netapp/trident/storage_attribute"
)

func TestGetLabelsJSONNoCharacterLimitSuccess(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 0)

	assert.Nil(t, err, "Error is not nil")
	// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}} is 67 characters
	assert.Equal(t, `{"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}`, label,
		"Label is not set correctly")
}

func TestGetLabelsJSONNoLabelSuccess(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(nil)

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 1023)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, label, "", "Label is set")
}

func TestGetLabelsJSONEmptyLabelSuccess(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(map[string]string{})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 1023)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, label, "", "Label is set")
}

func TestGetLabelsJSONLessThanLimitSuccess(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 1023)

	assert.Nil(t, err, "Error is not nil")
	// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}} is 67 characters
	assert.Equal(t, `{"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}`, label,
		"Label is not set correctly")
}

func TestGetLabelsJSONExactLimitSuccess(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(map[string]string{
		"labelName1": "labelValue1",
		"labelName2": "labelValue2",
	})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 72)

	assert.Nil(t, err, "Error is not nil")
	// {"provisioning":{"labelName1":"labelValue1","labelName2":"labelValue2"}} is72 characters
	assert.Equal(t, `{"provisioning":{"labelName1":"labelValue1","labelName2":"labelValue2"}}`, label,
		"Label is not set correctly")
}

func TestGetLabelsJSONExceedsCharacterLimitFail(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})

	// {"provisioning":{"clusterName":"dev-test-cluster-1"}} is 53 characters
	// {"provisioning":{"cloud":"anf"}} is 32 characters
	_, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 31)

	assert.NotNil(t, err, "Error is nil")
	assert.Contains(t, err.Error(), "exceeds the character limit", "character limit exceeded "+
		"error not raised")
}

func TestAllowLabelOverwriteInternalTrue(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 0)

	assert.Nil(t, err)
	assert.NotEmpty(t, label)

	allowLabelOverwrite := AllowPoolLabelOverwrite("provisioning", label)

	assert.True(t, allowLabelOverwrite, "Not allowed to overwrite internal label")
}

func TestAllowLabelOverwriteEmptyFalse(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(map[string]string{})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 0)

	assert.Nil(t, err)
	assert.Empty(t, label)

	allowLabelOverwrite := AllowPoolLabelOverwrite("provisioning", label)

	assert.False(t, allowLabelOverwrite, "Allowed to overwrite empty label")
}

func TestAllowLabelOverwriteExternalValidJSONFalse(t *testing.T) {
	pool := Pool{}
	pool.Attributes = make(map[string]sa.Offer)
	pool.Attributes["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud": "insights",
	})

	label, err := pool.GetLabelsJSON(context.TODO(), "cloudinsights", 0)

	assert.Nil(t, err)
	assert.NotEmpty(t, label)

	allowLabelOverwrite := AllowPoolLabelOverwrite("provisioning", label)

	assert.False(t, allowLabelOverwrite, "Allowed to overwrite external label")
}

func TestAllowLabelOverwriteExternalFreeFormFalse(t *testing.T) {
	allowLabelOverwrite := AllowPoolLabelOverwrite("provisioning", "Cloud Insights")

	assert.False(t, allowLabelOverwrite, "Allowed to overwrite external label")
}
