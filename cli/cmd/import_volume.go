// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
)

var (
	importFilename   string
	importBase64Data string
	importNoManage   bool
)

func init() {
	importCmd.AddCommand(importVolumeCmd)
	importVolumeCmd.Flags().StringVarP(&importFilename, "filename", "f", "", "Path to YAML or JSON PVC file")
	importVolumeCmd.Flags().BoolVarP(&importNoManage, "no-manage", "", false, "Create PV/PVC only, don't assume volume lifecycle management")
	importVolumeCmd.Flags().StringVarP(&importBase64Data, "base64", "", "", "Base64 encoding")
	if err := importVolumeCmd.Flags().MarkHidden("base64"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

var importVolumeCmd = &cobra.Command{
	Use:   "volume <backendName> <volumeName>",
	Short: "Import an existing volume to Trident",
	Long: `Import an existing volume to Trident

To import an existing volume, specify the name of the Trident backend
containing the volume, as well as the name that uniquely identifies
the volume on the storage (i.e. ONTAP FlexVol, Element Volume, CVS
Volume path').`,
	Aliases: []string{"v"},
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		pvcDataJSON, err := getPVCData(importFilename, importBase64Data)
		if err != nil {
			return err
		}

		if OperatingMode == ModeTunnel {
			command := []string{"import", "volume", "--base64", base64.StdEncoding.EncodeToString(pvcDataJSON)}
			if importNoManage {
				command = append(command, "--no-manage")
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return volumeImport(args[0], args[1], importNoManage, pvcDataJSON)
		}
	},
}

func getPVCData(filename, b64Data string) ([]byte, error) {
	var err error
	var rawData []byte

	if b64Data == "" && filename == "" {
		return nil, errors.New("no PVC input file was specified")
	}

	// Read from file or stdin or b64 data
	if b64Data != "" {
		rawData, err = base64.StdEncoding.DecodeString(b64Data)
	} else if filename == "-" {
		rawData, err = ioutil.ReadAll(os.Stdin)
	} else {
		rawData, err = ioutil.ReadFile(filename)
	}
	if err != nil {
		return nil, err
	}

	// Ensure the file is valid JSON/YAML, and return JSON
	jsonData, err := yaml.YAMLToJSON(rawData)
	if err != nil {
		return nil, err
	}

	return jsonData, nil
}

func volumeImport(backendName, internalVolumeName string, noManage bool, pvcDataJSON []byte) error {
	request := &storage.ImportVolumeRequest{
		Backend:      backendName,
		InternalName: internalVolumeName,
		NoManage:     noManage,
		PVCData:      base64.StdEncoding.EncodeToString(pvcDataJSON),
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	// Send the request to Trident
	url := BaseURL() + "/volume/import"

	response, responseBody, err := api.InvokeRESTAPI("POST", url, requestBytes, Debug)
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("could not import volume: %v", GetErrorFromHTTPResponse(response, responseBody))
	}

	var importVolumeResponse rest.ImportVolumeResponse
	err = json.Unmarshal(responseBody, &importVolumeResponse)
	if err != nil {
		return err
	}
	if importVolumeResponse.Volume == nil {
		return fmt.Errorf("could not import volume: no volume returned")
	}
	volume := *importVolumeResponse.Volume

	volumes := make([]storage.VolumeExternal, 0, 10)
	volumes = append(volumes, volume)
	WriteVolumes(volumes)

	return nil
}
