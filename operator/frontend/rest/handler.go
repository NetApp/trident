// Copyright 2024 NetApp, Inc. All Rights Reserved.

package rest

import (
	"encoding/json"
	"net/http"

	cliapi "github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/operator/clients"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
)

type OperatorHandler struct {
	oClient clients.OperatorCRDClientInterface
}

func NewOperatorHandler(clientFactory *clients.Clients) *OperatorHandler {
	return &OperatorHandler{oClient: clients.NewOperatorCRDClient(clientFactory.CRDClient)}
}

func (h *OperatorHandler) GetStatus(w http.ResponseWriter, _ *http.Request) {
	httpStatusCode := http.StatusOK

	operatorStatus, err := h.getStatus()
	if err != nil {
		httpStatusCode = http.StatusInternalServerError
		operatorStatus.Status = string(cliapi.OperatorPhaseError)
		operatorStatus.ErrorMessage = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatusCode)
	_ = json.NewEncoder(w).Encode(operatorStatus)
}

func (h *OperatorHandler) getStatus() (cliapi.OperatorStatus, error) {
	var torcCR *operatorV1.TridentOrchestrator

	configuratorsInstalled := true
	configuratorsProcessing := false
	status := cliapi.OperatorStatus{
		TorcStatus:  make(map[string]cliapi.CRStatus, operatorV1.MaxNumberOfTridentOrchestrators),
		TconfStatus: make(map[string]cliapi.CRStatus),
	}

	torcCRList, err := h.oClient.GetTorcCRList()
	if err != nil {
		return status, err
	}

	// Ideally, there will be none (tridentctl installation) or a single TorcCR present.
	if len(torcCRList.Items) > operatorV1.MaxNumberOfTridentOrchestrators {
		status.Status = string(cliapi.OperatorPhaseUnknown)
		status.ErrorMessage = "More than one Trident Orchestrator CR found"
		return status, nil
	}

	if len(torcCRList.Items) == 1 {
		torcCR = &torcCRList.Items[0]
		status.TorcStatus[torcCR.Name] = cliapi.CRStatus{Status: torcCR.Status.Status, Message: torcCR.Status.Message}
	}

	tconfCRList, err := h.oClient.GetTconfCRList()
	if err != nil {
		return status, err
	}

	for _, tconfCR := range tconfCRList.Items {
		status.TconfStatus[tconfCR.Name] = cliapi.CRStatus{
			Status:  tconfCR.Status.LastOperationStatus,
			Message: tconfCR.Status.Message,
		}

		if tconfCR.Status.LastOperationStatus == string(operatorV1.Processing) ||
			tconfCR.Status.LastOperationStatus == "" {
			configuratorsProcessing = true
		}
		if tconfCR.Status.LastOperationStatus == string(operatorV1.Failed) {
			configuratorsInstalled = false
		}
	}

	if torcCR != nil && torcCR.IsTridentOperationInProgress() {
		status.Status = string(cliapi.OperatorPhaseProcessing)
		return status, nil
	}

	if torcCR != nil && torcCR.HasTridentInstallationFailed() {
		status.Status = string(cliapi.OperatorPhaseFailed)
		return status, nil
	}

	// We consider that Trident is installed beyond this point.

	// If all the TconfCRs are processed successfully, we should report operator status as "Done".
	if configuratorsInstalled {
		status.Status = string(cliapi.OperatorPhaseDone)
	}

	// If any one of the TconfCR is in processing state, we should report operator status as "Processing".
	if configuratorsProcessing {
		status.Status = string(cliapi.OperatorPhaseProcessing)
	}

	// If no TconfCR is in processing state and at least one of the TconfCR failed, we should report operator
	// status as "Failed".
	if !configuratorsProcessing && !configuratorsInstalled {
		status.Status = string(cliapi.OperatorPhaseFailed)
	}

	return status, nil
}
