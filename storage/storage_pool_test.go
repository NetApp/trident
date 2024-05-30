// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storage

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	sa "github.com/netapp/trident/storage_attribute"
)

func TestGetLabelsJSONNoCharacterLimitSuccess(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 0)

	assert.Nil(t, err, "Error is not nil")
	// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}} is 67 characters
	assert.Equal(t, `{"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}`, label,
		"Label is not set correctly")
}

func TestGetTemplatizedLabelsJSONNoCharacterLimitSuccess(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    `{{.volume.Name}}_{{.volume.Namespace}}`,
	})

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label, err := pool.GetTemplatizedLabelsJSON(context.TODO(), "provisioning", 200, templateData)

	assert.Nil(t, err, "Error is not nil")
	// {"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}} is 67 characters
	assert.Equal(t, `{"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1","template":"newVolume_testNamespace"}}`, label,
		"Label is not set correctly")
}

func TestGetTemplatizedLabelsJSONNoLabelSuccess(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(nil) // Label is nil

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label, err := pool.GetTemplatizedLabelsJSON(context.TODO(), "provisioning", 1023, templateData)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, label, "", "Label is set")
}

func TestGetTemplatizedLabelsJSONEmptyLabelSuccess(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{}) // Label is empty

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label, err := pool.GetTemplatizedLabelsJSON(context.TODO(), "provisioning", 1023, templateData)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, label, "", "Label is set")
}

func TestGetTemplatizedLabelsJSONExceedsCharacterLimitFail(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    `{{.volume.Name}}_{{.volume.Namespace}}`,
	})

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	_, err := pool.GetTemplatizedLabelsJSON(context.TODO(), "provisioning", 20, templateData)

	assert.NotNil(t, err, "Error is nil")
	assert.Contains(t, err.Error(), "exceeds the character limit", "character limit exceeded error not raised")
}

func TestGetTemplatizedLabelsJSONLabelLimitZero(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    `{{.volume.Name}}_{{.volume.Namespace}}`,
	})

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	expectedLabel := "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\",\"template\":\"newVolume_testNamespace\"}}"
	labels, err := pool.GetTemplatizedLabelsJSON(context.TODO(), "provisioning", 0, templateData)

	assert.Equal(t, expectedLabel, labels)

	assert.NoError(t, err, "Error is not nil")
}

func TestGetTemplatizedLabelsJSONTemplateExecuteFail(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    `{{.volume.Name}}_{{.volume.InvalidFeild}}`,
	})

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label, err := pool.GetTemplatizedLabelsJSON(context.TODO(), "provisioning", 1023, templateData)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, `{"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1","template":"{{.volume.Name}}_{{.volume.InvalidFeild}}"}}`, label,
		"Label is not set correctly")
}

func TestGetTemplatizedLabelsJSONPoolAttributesNil(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = nil

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label, err := pool.GetTemplatizedLabelsJSON(context.TODO(), "provisioning", 1023, templateData)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, label, "", "Label is set")
}

func TestGetLabelMapFromTemplate(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    `{{.volume.Name}}_{{.volume.Namespace}}`,
	})

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label := pool.GetLabelMapFromTemplate(context.TODO(), templateData)

	expectedLabel := map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    `newVolume_testNamespace`,
	}
	assert.Equal(t, expectedLabel, label, "Label is not set correctly")
}

func TestGetLabelMapFromTemplate_NoTemplate(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label := pool.GetLabelMapFromTemplate(context.TODO(), templateData)

	expectedLabel := map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	}
	assert.Equal(t, expectedLabel, label, "Label is not set correctly")
}

func TestGetLabelMapFromTemplate_NoLabelInPool(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))

	// Test 1: label map is nil
	pool.Attributes()["labels"] = sa.NewLabelOffer(nil)

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label := pool.GetLabelMapFromTemplate(context.TODO(), templateData)

	expectedLabel := map[string]string{}
	assert.Equal(t, expectedLabel, label, "Label is not set correctly")

	// Test 2: label is nil
	pool.Attributes()["labels"] = nil
	label = pool.GetLabelMapFromTemplate(context.TODO(), templateData)

	expectedLabel = map[string]string{}
	assert.Equal(t, expectedLabel, label, "Label is not set correctly")

	// Test 3: label is empty
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{})
	label = pool.GetLabelMapFromTemplate(context.TODO(), templateData)

	expectedLabel = map[string]string{}
	assert.Equal(t, expectedLabel, label, "Label is not set correctly")
}

func TestGetLabelMapFromTemplate_InvalidTemplate(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    `{{.volume.Name}}_{{.volume.Namespa}}`,
	})

	volume := VolumeConfig{Name: "newVolume", Namespace: "testNamespace"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	label := pool.GetLabelMapFromTemplate(context.TODO(), templateData)

	expectedLabel := map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"template":    "{{.volume.Name}}_{{.volume.Namespa}}",
	}
	assert.Equal(t, expectedLabel, label, "Label is not set correctly")
}

func TestGetLabelsJSONNoLabelSuccess(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(nil)

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 1023)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, label, "", "Label is set")
}

func TestGetLabelsJSONEmptyLabelSuccess(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 1023)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, label, "", "Label is set")
}

func TestGetLabelsJSONLessThanLimitSuccess(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
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
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
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
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
	})

	// {"provisioning":{"clusterName":"dev-test-cluster-1"}} is 53 characters
	// {"provisioning":{"cloud":"anf"}} is 32 characters
	_, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 31)

	assert.NotNil(t, err, "Error is nil")
	assert.Contains(t, err.Error(), "exceeds the character limit", "character limit exceeded error not raised")
}

func TestGetLabels(t *testing.T) {
	labels := map[string]string{
		"cloud":       "anf",
		"clusterName": "dev-test-cluster-1",
		"abc.io/tier": "warm",
	}

	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(labels)

	expectedLabels := map[string]string{
		"prefix/cloud":       "anf",
		"prefix/clusterName": "dev-test-cluster-1",
		"abc.io/tier":        "warm",
	}

	assert.True(t, cmp.Equal(expectedLabels, pool.GetLabels(context.TODO(), "prefix/")))
}

func TestGetLabelsNone(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))

	expectedLabels := map[string]string{}

	assert.True(t, cmp.Equal(expectedLabels, pool.GetLabels(context.TODO(), "prefix_")))
}

func TestAllowLabelOverwriteInternalTrue(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
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
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{})

	label, err := pool.GetLabelsJSON(context.TODO(), "provisioning", 0)

	assert.Nil(t, err)
	assert.Empty(t, label)

	allowLabelOverwrite := AllowPoolLabelOverwrite("provisioning", label)

	assert.False(t, allowLabelOverwrite, "Allowed to overwrite empty label")
}

func TestAllowLabelOverwriteExternalValidJSONFalse(t *testing.T) {
	pool := StoragePool{}
	pool.SetAttributes(make(map[string]sa.Offer))
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
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

func TestUpdateProvisionLabelsReplaceFirstSuccess(t *testing.T) {
	originalLabels := []string{`{"provisioning":{"labelName1":"labelValue1","labelName2":"labelValue2"}}`, "foo", "bar"}

	newLabels := UpdateProvisioningLabels(
		`{"provisioning":{"labelName3":"labelValue3","labelName4":"labelValue4"}}`, originalLabels)

	assert.Equal(t, []string{"foo", "bar", `{"provisioning":{"labelName3":"labelValue3","labelName4":"labelValue4"}}`},
		newLabels, "Label is not updated correctly")
}

func TestUpdateProvisionLabelsReplaceMiddleSuccess(t *testing.T) {
	originalLabels := []string{"foo", `{"provisioning":{"labelName1":"labelValue1","labelName2":"labelValue2"}}`, "bar"}

	newLabels := UpdateProvisioningLabels(
		`{"provisioning":{"labelName3":"labelValue3","labelName4":"labelValue4"}}`, originalLabels)

	assert.Equal(t, []string{"foo", "bar", `{"provisioning":{"labelName3":"labelValue3","labelName4":"labelValue4"}}`},
		newLabels, "Label is not updated correctly")
}

func TestUpdateProvisionLabelsReplaceLastSuccess(t *testing.T) {
	originalLabels := []string{"foo", "bar", `{"provisioning":{"labelName1":"labelValue1","labelName2":"labelValue2"}}`}

	newLabels := UpdateProvisioningLabels(
		`{"provisioning":{"labelName3":"labelValue3","labelName4":"labelValue4"}}`, originalLabels)

	assert.Equal(t, []string{"foo", "bar", `{"provisioning":{"labelName3":"labelValue3","labelName4":"labelValue4"}}`},
		newLabels, "Label is not updated correctly")
}

func TestUpdateProvisionNotFoundSuccess(t *testing.T) {
	originalLabels := []string{"foo", "bar"}

	newLabels := UpdateProvisioningLabels(
		`{"provisioning":{"labelName3":"labelValue3","labelName4":"labelValue4"}}`, originalLabels)

	assert.Equal(t, []string{"foo", "bar", `{"provisioning":{"labelName3":"labelValue3","labelName4":"labelValue4"}}`},
		newLabels, "Label is not set correctly")
}

func TestDeleteProvisioningLabelsFoundFirstSuccess(t *testing.T) {
	newLabels := DeleteProvisioningLabels([]string{
		`{"provisioning":{"labelName1":"labelValue1","labelName2":"labelValue2"}}`, "foo", "bar",
	})

	assert.Equal(t, []string{"foo", "bar"}, newLabels, "Label is not deleted correctly")
}

func TestDeleteProvisioningLabelsFoundMiddleSuccess(t *testing.T) {
	newLabels := DeleteProvisioningLabels([]string{
		"foo", `{"provisioning":{"labelName1":"labelValue1","labelName2":"labelValue2"}}`, "bar",
	})

	assert.Equal(t, []string{"foo", "bar"}, newLabels, "Label is not deleted correctly")
}

func TestDeleteProvisioningLabelsFoundLastSuccess(t *testing.T) {
	newLabels := DeleteProvisioningLabels([]string{
		"foo", "bar", `{"provisioning":{"labelName1":"labelValue1","labelName2":"labelValue2"}}`,
	})

	assert.Equal(t, []string{"foo", "bar"}, newLabels, "Label is not deleted correctly")
}

func TestDeleteProvisioningLabelsNotFoundSuccess(t *testing.T) {
	newLabels := DeleteProvisioningLabels([]string{"foo", "bar"})

	assert.Equal(t, []string{"foo", "bar"}, newLabels, "Label is not left as is")
}
