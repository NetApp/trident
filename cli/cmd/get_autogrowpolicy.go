// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

func init() {
	getCmd.AddCommand(getAutogrowPolicyCmd)
}

var getAutogrowPolicyCmd = &cobra.Command{
	Use:     "autogrowpolicy [<name>...]",
	Short:   "Get one or more autogrow policies from Trident",
	Aliases: []string{"agp", "autogrowpolicies"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"get", "autogrowpolicy"}
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		} else {
			return autogrowPolicyList(args)
		}
	},
}

func autogrowPolicyList(autogrowPolicyNames []string) error {
	var err error

	// If no policies were specified, we'll get all of them
	getAll := false
	if len(autogrowPolicyNames) == 0 {
		getAll = true
		autogrowPolicyNames, err = GetAutogrowPolicies()
		if err != nil {
			return err
		}
	}

	autogrowPolicies := make([]storage.AutogrowPolicyExternal, 0, config.DefaultAutogrowPoliciesForCLI)

	// Get the actual policy objects
	for _, autogrowPolicyName := range autogrowPolicyNames {
		autogrowPolicy, err := GetAutogrowPolicy(autogrowPolicyName)
		if err != nil {
			if getAll && errors.IsAutogrowPolicyNotFoundError(err) {
				continue
			}
			return err
		}
		autogrowPolicies = append(autogrowPolicies, autogrowPolicy)
	}

	WriteAutogrowPolicies(autogrowPolicies)

	return nil
}

func GetAutogrowPolicies() ([]string, error) {
	url := BaseURL() + "/autogrowpolicy"

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get Autogrow policies: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	var listPoliciesResponse rest.ListAutogrowPoliciesResponse
	err = json.Unmarshal(responseBody, &listPoliciesResponse)
	if err != nil {
		return nil, err
	}

	return listPoliciesResponse.AutogrowPolicies, nil
}

func GetAutogrowPolicy(autogrowPolicyName string) (storage.AutogrowPolicyExternal, error) {
	url := BaseURL() + "/autogrowpolicy/" + autogrowPolicyName

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return storage.AutogrowPolicyExternal{}, err
	} else if response.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("could not get autogrow policy %s: %v", autogrowPolicyName,
			GetErrorFromHTTPResponse(response, responseBody))
		switch response.StatusCode {
		case http.StatusNotFound:
			return storage.AutogrowPolicyExternal{}, errors.AutogrowPolicyNotFoundError(errorMessage)
		default:
			return storage.AutogrowPolicyExternal{}, errors.New(errorMessage)
		}
	}

	var getPolicyResponse rest.GetAutogrowPolicyResponse
	err = json.Unmarshal(responseBody, &getPolicyResponse)
	if err != nil {
		return storage.AutogrowPolicyExternal{}, err
	}

	return getPolicyResponse.AutogrowPolicy, nil
}

func WriteAutogrowPolicies(autogrowPolicies []storage.AutogrowPolicyExternal) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(api.MultipleAutogrowPolicyResponse{Items: autogrowPolicies})
	case FormatYAML:
		WriteYAML(api.MultipleAutogrowPolicyResponse{Items: autogrowPolicies})
	case FormatName:
		writeAutogrowPolicyNames(autogrowPolicies)
	default:
		writeAutogrowPolicyTable(autogrowPolicies)
	}
}

func writeAutogrowPolicyTable(autogrowPolicies []storage.AutogrowPolicyExternal) {
	// Sort autogrowPolicies alphabetically by name
	sort.Slice(autogrowPolicies, func(i, j int) bool {
		return autogrowPolicies[i].Name < autogrowPolicies[j].Name
	})

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Used Threshold", "Growth Amount", "Max Size", "Volumes"})

	for _, agp := range autogrowPolicies {
		maxSize := agp.MaxSize
		if maxSize == "" {
			maxSize = "N/A"
		}

		growthAmount := agp.GrowthAmount
		if growthAmount == "" {
			growthAmount = config.DefaultAGPGrowthAmount // Default value
		}

		table.Append([]string{
			agp.Name,
			agp.UsedThreshold,
			growthAmount,
			maxSize,
			strconv.Itoa(agp.VolumeCount),
		})
	}

	table.Render()
}

func writeAutogrowPolicyNames(autogrowPolicies []storage.AutogrowPolicyExternal) {
	// Sort autogrowPolicies alphabetically by name
	sort.Slice(autogrowPolicies, func(i, j int) bool {
		return autogrowPolicies[i].Name < autogrowPolicies[j].Name
	})

	for _, agp := range autogrowPolicies {
		fmt.Println(agp.Name)
	}
}
