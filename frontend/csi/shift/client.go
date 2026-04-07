// Copyright 2025 NetApp, Inc. All Rights Reserved.

package shift

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
)

// JobStatus represents the status of a Shift job.
type JobStatus string

const (
	JobStatusSuccess JobStatus = "success"
	JobStatusRunning JobStatus = "running"
	JobStatusFailed  JobStatus = "failed"
)

// Request is the payload that will be sent to the Shift REST endpoint.
type Request struct {
	PVCName       string `json:"pvcName"`
	PVCNamespace  string `json:"pvcNamespace"`
	BackendUUID   string `json:"backendUUID"`
	ManagementLIF string `json:"managementLIF"`
	SVM           string `json:"svm"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	DiskPath      string `json:"diskPath"`
	NFSServer     string `json:"nfsServer"`
	NFSPath       string `json:"nfsPath"`
	VMID          string `json:"vmId"`
	VMUUID        string `json:"vmUuid"`
}

// Response is the payload returned by the Shift REST endpoint.
type Response struct {
	Status     JobStatus `json:"status"`
	VolumeName string    `json:"volumeName"`
	Message    string    `json:"message"`
	JobID      string    `json:"jobId"`
}

// Client is the interface for invoking Shift jobs.
type Client interface {
	InvokeShiftJob(ctx context.Context, shiftCfg *storage.ShiftConfig, pvcName, pvcNamespace string) (*Response, error)
}

// dryRunClient is a log-only stub used for testing the integration wiring.
// It logs every field that would be sent to the Shift endpoint and returns
// a "running" status so the PVC stays Pending and Kubernetes retries.
// TODO: Replace with real httpClient once the Shift endpoint is available.
type dryRunClient struct{}

// NewClient returns a dry-run Shift client that only logs.
// Swap this to newHTTPClient() when ready to call the real endpoint.
func NewClient() Client {
	return &dryRunClient{}
}

func (c *dryRunClient) InvokeShiftJob(
	ctx context.Context, shiftCfg *storage.ShiftConfig, pvcName, pvcNamespace string,
) (*Response, error) {

	Logc(ctx).Info("========== SHIFT DRY-RUN: START ==========")
	Logc(ctx).WithFields(LogFields{
		"pvcName":      pvcName,
		"pvcNamespace": pvcNamespace,
	}).Info("SHIFT DRY-RUN: PVC identification")

	Logc(ctx).WithFields(LogFields{
		"endpoint":      shiftCfg.Endpoint,
		"backendUUID":   shiftCfg.BackendUUID,
		"managementLIF": shiftCfg.ManagementLIF,
		"svm":           shiftCfg.SVM,
		"hasUsername":    shiftCfg.Username != "",
		"hasPassword":   shiftCfg.Password != "",
	}).Info("SHIFT DRY-RUN: ONTAP credentials (from TBC secret)")

	Logc(ctx).WithFields(LogFields{
		"diskPath":  shiftCfg.DiskPath,
		"nfsServer": shiftCfg.NFSServer,
		"nfsPath":   shiftCfg.NFSPath,
		"vmID":      shiftCfg.VMID,
		"vmUUID":    shiftCfg.VMUUID,
	}).Info("SHIFT DRY-RUN: MTV VM metadata (from PVC annotations)")

	Logc(ctx).Info("SHIFT DRY-RUN: Would POST to endpoint: " + shiftCfg.Endpoint)
	Logc(ctx).Info("SHIFT DRY-RUN: Returning 'running' status so PVC stays Pending (dry-run mode).")
	Logc(ctx).Info("========== SHIFT DRY-RUN: END ==========")

	return &Response{
		Status:  JobStatusRunning,
		JobID:   "dry-run-job-001",
		Message: "dry-run mode: Shift endpoint not called",
	}, nil
}

// TODO: Uncomment and wire this in when the real Shift endpoint is available.
//
// import (
//     "bytes"
//     "crypto/tls"
//     "encoding/json"
//     "fmt"
//     "io"
//     "net/http"
//     "time"
// )
//
// type httpClient struct {
//     client *http.Client
// }
//
// func newHTTPClient() Client {
//     return &httpClient{
//         client: &http.Client{
//             Timeout: 30 * time.Second,
//             Transport: &http.Transport{
//                 TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
//             },
//         },
//     }
// }
//
// func (c *httpClient) InvokeShiftJob(
//     ctx context.Context, shiftCfg *storage.ShiftConfig, pvcName, pvcNamespace string,
// ) (*Response, error) {
//     reqBody := &Request{
//         PVCName:       pvcName,
//         PVCNamespace:  pvcNamespace,
//         BackendUUID:   shiftCfg.BackendUUID,
//         ManagementLIF: shiftCfg.ManagementLIF,
//         SVM:           shiftCfg.SVM,
//         Username:      shiftCfg.Username,
//         Password:      shiftCfg.Password,
//         DiskPath:      shiftCfg.DiskPath,
//         NFSServer:     shiftCfg.NFSServer,
//         NFSPath:       shiftCfg.NFSPath,
//         VMID:          shiftCfg.VMID,
//         VMUUID:        shiftCfg.VMUUID,
//     }
//
//     payload, err := json.Marshal(reqBody)
//     if err != nil {
//         return nil, fmt.Errorf("shift: failed to marshal request: %v", err)
//     }
//
//     Logc(ctx).WithFields(LogFields{
//         "endpoint": shiftCfg.Endpoint,
//         "pvcName":  pvcName,
//     }).Info("Shift: invoking Shift REST endpoint.")
//
//     httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, shiftCfg.Endpoint, bytes.NewReader(payload))
//     if err != nil {
//         return nil, fmt.Errorf("shift: failed to create HTTP request: %v", err)
//     }
//     httpReq.Header.Set("Content-Type", "application/json")
//
//     httpResp, err := c.client.Do(httpReq)
//     if err != nil {
//         return nil, fmt.Errorf("shift: HTTP request failed: %v", err)
//     }
//     defer httpResp.Body.Close()
//
//     body, err := io.ReadAll(httpResp.Body)
//     if err != nil {
//         return nil, fmt.Errorf("shift: failed to read response body: %v", err)
//     }
//
//     if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
//         return nil, fmt.Errorf("shift: endpoint returned HTTP %d: %s", httpResp.StatusCode, string(body))
//     }
//
//     var resp Response
//     if err = json.Unmarshal(body, &resp); err != nil {
//         return nil, fmt.Errorf("shift: failed to unmarshal response: %v", err)
//     }
//
//     Logc(ctx).WithFields(LogFields{
//         "jobStatus":  resp.Status,
//         "volumeName": resp.VolumeName,
//         "jobID":      resp.JobID,
//     }).Info("Shift: parsed Shift response.")
//
//     return &resp, nil
// }
