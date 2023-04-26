// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	operatorconfig "github.com/netapp/trident/operator/config"
	versionutils "github.com/netapp/trident/utils/version"
)

var (
	K8sVersion          string
	versionNotSupported = "The provided Kubernetes version is not supported: "
	versionNotSemantic  = "The provided Kubernetes version was not given in semantic versioning: "
)

type ImageSet struct {
	Images     []string `json:"images"`
	K8sVersion string   `json:"k8sVersion"`
}

type ImageList struct {
	ImageSets []ImageSet `json:"imageSets"`
}

func init() {
	getImageCmd.Flags().StringVarP(&K8sVersion, "k8s-version", "v", "all",
		"Semantic version of Kubernetes cluster")
	RootCmd.AddCommand(getImageCmd)
}

var getImageCmd = &cobra.Command{
	Use:     "images",
	Short:   "Print a table of the container images Trident needs",
	Aliases: []string{},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"images"}
			TunnelCommand(append(command, args...))
			return nil
		} else {

			if err := InitLogLevel("warn"); err != nil {
				return err
			}

			// Create the Kubernetes client
			var err error
			if client, err = initClient(); err != nil {
				Log().Fatalf("could not initialize Kubernetes client; %v", err)
			}
			return printImages()
		}
	},
}

func printImages() error {
	err := listImages()
	if err != nil {
		if err.Error() == versionNotSupported {
			Log().Fatal(fmt.Sprintln(versionNotSupported, K8sVersion))
		} else {
			Log().Fatal(fmt.Sprintln(versionNotSemantic, K8sVersion))
		}
	}

	return nil
}

func listImages() error {
	var err error
	var semVersion *versionutils.Version
	k8sVersions := make([]string, 0)
	imageMap := make(map[string][]string)
	var installYaml string
	var yamlErr error

	if K8sVersion == "all" {
		minMinorVersion := versionutils.MustParseMajorMinorVersion(tridentconfig.KubernetesVersionMin).MinorVersion()
		maxMinorVersion := versionutils.MustParseMajorMinorVersion(tridentconfig.KubernetesVersionMax).MinorVersion()
		for i := minMinorVersion; i <= maxMinorVersion; i++ {
			semVersion := fmt.Sprintf("v1.%d.0", i)
			k8sVersions = append(k8sVersions, semVersion)
			installYaml, yamlErr = getInstallYaml(versionutils.MustParseSemantic(semVersion))
			if yamlErr != nil {
				return yamlErr
			}
			imageMap[semVersion] = getImageNames(installYaml)
		}
	} else {
		semVersion, err = versionutils.ParseSemantic(K8sVersion)
		if err != nil {
			return err
		}
		k8sVersions = append(k8sVersions, K8sVersion)
		installYaml, yamlErr = getInstallYaml(semVersion)
		if yamlErr != nil {
			return yamlErr
		}
		imageMap[K8sVersion] = getImageNames(installYaml)
	}
	writeImages(k8sVersions, imageMap)
	return nil
}

func getInstallYaml(semVersion *versionutils.Version) (string, error) {
	minorVersion := semVersion.ToMajorMinorVersion().MinorVersion()
	isSupportedVersion := minorVersion <= versionutils.MustParseMajorMinorVersion(tridentconfig.KubernetesVersionMax).MinorVersion() &&
		minorVersion >= versionutils.MustParseMajorMinorVersion(tridentconfig.KubernetesVersionMin).MinorVersion()

	if !isSupportedVersion {
		return "", errors.New(versionNotSupported)
	}

	var snapshotCRDVersion string
	if client != nil {
		snapshotCRDVersion = client.GetSnapshotterCRDVersion()
	}

	deploymentArgs := &k8sclient.DeploymentYAMLArguments{
		DeploymentName:          getDeploymentName(true),
		TridentImage:            tridentconfig.BuildImage,
		AutosupportImage:        tridentconfig.DefaultAutosupportImage,
		AutosupportProxy:        "",
		AutosupportCustomURL:    "",
		AutosupportSerialNumber: "",
		AutosupportHostname:     "",
		ImageRegistry:           "",
		LogFormat:               "",
		LogLevel:                "info",
		SnapshotCRDVersion:      snapshotCRDVersion,
		ImagePullSecrets:        []string{},
		Labels:                  map[string]string{},
		ControllingCRDetails:    map[string]string{},
		UseIPv6:                 false,
		SilenceAutosupport:      true,
		Version:                 semVersion,
		TopologyEnabled:         false,
		HTTPRequestTimeout:      tridentconfig.HTTPTimeoutString,
		ServiceAccountName:      getControllerRBACResourceName(true),
	}
	// Get Deployment and Daemonset YAML and collect the names of the container images Trident needs to run.
	yaml := k8sclient.GetCSIDeploymentYAML(deploymentArgs)
	daemonSetArgs := &k8sclient.DaemonsetYAMLArguments{
		DaemonsetName:        "",
		TridentImage:         "",
		ImageRegistry:        "",
		KubeletDir:           "",
		LogFormat:            "",
		LogLevel:             "info",
		ProbePort:            "",
		ImagePullSecrets:     []string{},
		Labels:               map[string]string{},
		ControllingCRDetails: map[string]string{},
		Version:              semVersion,
		HTTPRequestTimeout:   tridentconfig.HTTPTimeoutString,
		ServiceAccountName:   getNodeRBACResourceName(false),
	}
	// trident image here is an empty string because we are already going to get it from the deployment yaml
	yaml += k8sclient.GetCSIDaemonSetYAMLLinux(daemonSetArgs)

	return yaml, nil
}

func getImageNames(yaml string) []string {
	var images []string

	lines := strings.Split(yaml, "\n")
	// don't get images that are empty strings
	imageRegex := regexp.MustCompile(`\s*image:\s*(.*)`)
	for i := 0; i < len(lines); i++ {
		imageLine := lines[i]
		image := strings.TrimSpace(imageRegex.ReplaceAllString(imageLine, "$1"))
		if matches := imageRegex.MatchString(imageLine); matches {
			// strip out "    image:" prefix
			if image != "" {
				images = append(images, image)
			}
		}
	}

	// Add operator image
	switch OutputFormat {
	case FormatJSON, FormatYAML:
		images = append(images, operatorconfig.BuildImage)
	default:
		images = append(images, operatorconfig.BuildImage+" (optional)")
	}

	return images
}

func writeImages(k8sVersions []string, imageMap map[string][]string) {
	var imageSets []ImageSet
	for _, k8sVersion := range k8sVersions {
		var images []string
		for _, image := range imageMap[k8sVersion] {
			images = append(images, image)
		}
		imageSets = append(imageSets, ImageSet{Images: images, K8sVersion: k8sVersion})
	}
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(ImageList{ImageSets: imageSets})
	case FormatYAML:
		WriteYAML(ImageList{ImageSets: imageSets})
	default:
		writeImageTable(k8sVersions, imageMap)
	}
}

func writeImageTable(k8sVersions []string, imageMap map[string][]string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetAutoWrapText(false)
	table.SetHeader([]string{"Kubernetes Version", "Container Image"})
	if OutputFormat == FormatMarkdown {
		// print in markdown table format
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetCenterSeparator("|")
	}
	table.SetRowLine(true)
	for _, k8sVersion := range k8sVersions {
		table.Append([]string{k8sVersion, strings.Join(imageMap[k8sVersion], "\n")})
	}
	table.Render()
}
