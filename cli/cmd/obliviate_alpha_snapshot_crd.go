// Copyright 2020 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	obliviateCmd.AddCommand(obliviateAlphaSnapshotCRDCmd)
	obliviateAlphaSnapshotCRDCmd.Flags().StringVar(&configPath, "k8s-config-path", kubeConfigPath(),
		"Path to KubeConfig file.")
}

var obliviateAlphaSnapshotCRDCmd = &cobra.Command{
	Use:              "alpha-snapshot-crd",
	Short:            "Delete kubernetes snapshot CRDs that are v1alpha1",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	RunE: func(cmd *cobra.Command, args []string) error {

		var err error
		initLogging()

		if OperatingMode == ModeTunnel {
			command := []string{"obliviate", "alpha-snapshot-crd"}
			TunnelCommand(command)
			return nil
		} else {
			forceObliviate = true // Don't require confirmation for this command
			if err = initClients(); err != nil {
				return err
			}
			return deleteAlphaSnapshotCRDs()
		}
	},
}

func deleteAlphaSnapshotCRDs() error {

	crdNames := []string{
		"volumesnapshotclasses.snapshot.storage.k8s.io",
		"volumesnapshotcontents.snapshot.storage.k8s.io",
		"volumesnapshots.snapshot.storage.k8s.io",
	}

	for _, crdName := range crdNames {

		logFields := log.Fields{"CRD": crdName}

		// See if CRD exists
		exists, err := k8sClient.CheckCRDExists(crdName)
		if err != nil {
			return err
		}
		if !exists {
			log.WithFields(logFields).Info("CRD not present.")
			continue
		}

		// Get the CRD and check version
		crd, err := k8sClient.GetCRD(crdName)
		if err != nil {
			return err
		}
		alpha := false
		if strings.ToLower(crd.Spec.Version) == "v1alpha1" {
			alpha = true
		}
		for _, version := range crd.Spec.Versions {
			if strings.ToLower(version.Name) == "v1alpha1" {
				alpha = true
			}
		}

		if !alpha {
			log.WithFields(logFields).Info("CRD is not alpha")
			continue
		}

		log.WithFields(logFields).Debug("Deleting CRD.")

		// Try deleting CRD
		if err = k8sClient.DeleteCRD(crdName); err != nil {
			log.WithFields(logFields).Errorf("Could not delete CRD; %v", err)
			return err
		}

		// Check if CRD still exists (mostly likely pinned by finalizer)
		if exists, err = k8sClient.CheckCRDExists(crdName); err != nil {
			return err
		} else if !exists {
			log.WithFields(logFields).Info("CRD deleted.")
			continue
		}

		log.WithFields(logFields).Debug("CRD still present, must remove finalizers.")

		// Remove finalizer
		if err = k8sClient.RemoveFinalizerFromCRD(crdName); err != nil {
			log.WithFields(logFields).Errorf("Could not remove finalizer from CRD; %v", err)
			return err
		} else {
			log.WithFields(logFields).Debug("Removed finalizer from CRD.")
		}

		// Check if removing the finalizer allowed the CRD to be deleted
		if exists, err = k8sClient.CheckCRDExists(crdName); err != nil {
			return err
		} else if !exists {
			log.WithFields(logFields).Info("CRD deleted after removing finalizers.")
			continue
		} else {
			log.WithFields(logFields).Error("CRD still not deleted after removing finalizers.")
		}
	}

	return nil
}
