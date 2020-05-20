/*
 * Copyright (c) 2020 NetApp, Inc. All Rights Reserved.
 */

package cmd

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetCRDMapFromBundle(t *testing.T) {
	log.Debug("Running TestGetCRDMapFromBundle...")

	CRDBundle := createCRDBundle(CRDnames)

	crdMap := getCRDMapFromBundle(CRDBundle)

	assert.Equal(t, len(CRDnames), len(crdMap), "Mismatch between number of CRD names (" +
		"count %v) and CRDs in the map (count %v)", len(CRDnames), len(crdMap))

	var missingCRDs []string
	for _, crdName := range CRDnames {
		if _, ok := crdMap[crdName]; !ok {
			missingCRDs = append(missingCRDs, crdName)
		}
	}

	if len(missingCRDs) > 0 {
		assert.Fail(t, "CRD map missing CRDs: %v", missingCRDs)
	}
}

func TestValidateCRDsPass(t *testing.T) {
	log.Debug("Running TestValidateCRDsPass...")

	CRDBundle := createCRDBundle(CRDnames)

	crdMap := getCRDMapFromBundle(CRDBundle)

	assert.Nil(t, validateCRDs(crdMap), "CRD validation failed")
}

func TestValidateCRDsMissingCRDsFail(t *testing.T) {
	log.Debug("Running TestValidateCRDsMissingCRDsFail...")

	CRDBundle := createCRDBundle(CRDnames[1:len(CRDnames)-1])

	crdMap := getCRDMapFromBundle(CRDBundle)

	assert.NotNil(t, validateCRDs(crdMap), "CRD validation should fail")
}

func TestValidateCRDsExtraCRDsFail(t *testing.T) {
	log.Debug("Running TestValidateCRDsExtraCRDsFail...")

	newCRDNames := append(CRDnames, "xyz.trident.netapp.io")

	CRDBundle := createCRDBundle(newCRDNames)

	crdMap := getCRDMapFromBundle(CRDBundle)

	assert.NotNil(t, validateCRDs(crdMap), "CRD validation should fail")
}

func createCRDBundle(crdNames []string) string {
	var crdBundle string
	for i, crdName := range crdNames {
		crdBundle = crdBundle + fmt.Sprintf(CRDTemplate, crdName)

		if i != len(crdNames)-1 {
			crdBundle = crdBundle + "\n---"
		}
	}

	return crdBundle
}

var CRDTemplate = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: %v
spec:
  group: trident.netapp.io`
