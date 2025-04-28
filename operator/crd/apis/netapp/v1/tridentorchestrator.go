// Copyright 2024 NetApp, Inc. All Rights Reserved.

package v1

import "github.com/netapp/trident/pkg/collection"

type AppStatus string

const (
	AppStatusNotInstalled AppStatus = ""             // default
	AppStatusInstalling   AppStatus = "Installing"   // Set only on controlling CR
	AppStatusInstalled    AppStatus = "Installed"    // Set only on controlling CR
	AppStatusUninstalling AppStatus = "Uninstalling" // Set only on controlling CR
	AppStatusUninstalled  AppStatus = "Uninstalled"  // Set only on controlling CR
	AppStatusFailed       AppStatus = "Failed"       // Set only on controlling CR
	AppStatusUpdating     AppStatus = "Updating"     // Set only on controlling CR
	AppStatusError        AppStatus = "Error"        // Should not be set on controlling CR
)

const MaxNumberOfTridentOrchestrators int = 1

func (o *TridentOrchestrator) HasTridentInstallationFailed() bool {
	if o.Status.Status == string(AppStatusFailed) || o.Status.Status == string(AppStatusError) {
		return true
	}
	return false
}

func (o *TridentOrchestrator) IsTridentOperationInProgress() bool {
	if o.Status.Status != string(AppStatusInstalled) && o.Status.Status != string(AppStatusUninstalled) &&
		!o.HasTridentInstallationFailed() {
		return true
	}
	return false
}

func (in *TridentOrchestrator) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentOrchestrator) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentOrchestrator) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentOrchestrator) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
