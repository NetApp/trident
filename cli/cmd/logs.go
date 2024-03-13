// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"archive/zip"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/utils/errors"
)

const (
	logNameTrident                 = "trident-controller"
	logNameTridentPrevious         = "trident-controller-previous"
	logNameNode                    = "trident-node"
	logNameNodePrevious            = "trident-node-previous"
	logNameTridentOperator         = "trident-operator"
	logNameTridentOperatorPrevious = "trident-operator-previous"
	logNameTridentACP              = "trident-acp"
	logNameTridentACPPrevious      = "trident-acp-previous"

	logTypeAuto            = "auto"
	logTypeTrident         = "trident"
	logTypeTridentACP      = "acp"
	logTypeTridentOperator = "trident-operator"
	logTypeAll             = "all"

	archiveFilenameFormat = "support-2006-01-02T15-04-05-MST.zip"

	logNameTridentBackendConfig         = "tridentBackendConfig"
	logNameTridentActionMirrorUpdate    = "tridentActionMirrorUpdate"
	logNameTridentActionSnapshotRestore = "tridentActionSnapshotRestore"
	logNameTridentBackend               = "tridentBackend"
	logNameTridentMirrorRelationship    = "tridentMirrorRelationship"
	logNameTridentNode                  = "tridentNode"
	logNameTridentSnapshotInfo          = "tridentSnapshotInfo"
	logNameTridentSnapshot              = "tridentSnapshot"
	logNameTridentStorageClass          = "tridentStorageClass"
	logNameTridentTransaction           = "tridentTransaction"
	logNameTridentVersion               = "tridentVersion"
	logNameTridentVolumePublication     = "tridentVolumePublication"
	logNameTridentVolumeReference       = "tridentVolumeReference"
	logNameTridentVolume                = "tridentVolume"

	logNamePersistentVolume      = "persistentVolume"
	logNamePersistentVolumeClaim = "persistentVolumeClaim"
	logNameVolumeSnapshotClass   = "volumeSnapshotClass"
	logNameVolumeSnapshotContent = "volumeSnapshotContent"
	logNameVolumeSnapshot        = "volumeSnapshot"
)

var (
	logType     string
	archive     bool
	previous    bool
	node        string
	sidecars    bool
	zipFileName string
	zipWriter   *zip.Writer
	logErrors   []byte

	tridentOperatorPodName      string
	tridentOperatorPodNamespace string
)

func init() {
	RootCmd.AddCommand(logsCmd)
	logsCmd.Flags().StringVarP(&logType, "log", "l", logTypeAuto,
		"Trident log to display. One of trident|acp|auto|trident-operator|all")
	logsCmd.Flags().BoolVarP(&archive, "archive", "a", false,
		"Create a support archive with all logs unless otherwise specified.")
	logsCmd.Flags().BoolVarP(&previous, "previous", "p", false,
		"Get the logs for the previous container instance if it exists.")
	logsCmd.Flags().StringVar(&node, "node", "", "The kubernetes node name to gather node pod logs from.")
	logsCmd.Flags().BoolVar(&sidecars, "sidecars", false, "Get the logs for the sidecar containers as well.")
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print the logs from Trident",
	Long:  "Print the logs from the Trident storage orchestrator for Kubernetes",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		tridentOperatorPodName = ""
		tridentOperatorPodNamespace = ""
		var operatorErr error

		initCmdLogging()
		err := discoverOperatingMode(cmd)

		// Initialize the clients
		if err := initClients(); err != nil {
			return err
		}

		switch logType {
		case logTypeAll:
			// We know the operating mode, just get Operator details but don't throw error as it may not exist
			if err == nil {
				tridentOperatorPodName, tridentOperatorPodNamespace, operatorErr = getTridentOperatorPod(TridentOperatorLabel)
			}
			return err
		case logTypeTridentOperator:
			if err != nil {
				// Need to discover Operating mode first
				operatorErr := discoverJustOperatingMode(cmd)
				if operatorErr != nil {
					return fmt.Errorf("unable to discover operating mode for the operator; err: %s", operatorErr)
				}
			}

			// Now, we know the operating mode, just get Operator details but need to throw error as it may not exist
			if tridentOperatorPodName, tridentOperatorPodNamespace,
				operatorErr = getTridentOperatorPod(TridentOperatorLabel); operatorErr != nil {
				return operatorErr
			}
			return nil
		default:
			return err
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := checkValidLog()
		if err != nil {
			return err
		}

		if archive {
			return archiveLogs()
		} else {
			return consoleLogs()
		}
	},
}

func writeLogs(logName string, logEntry []byte) error {
	if archive {
		entry, err := zipWriter.Create(logName)
		if err != nil {
			return err
		}
		_, err = entry.Write(logEntry)
		if err != nil {
			return err
		}
		fmt.Printf("Wrote %s log to %s archive file.\n", logName, zipFileName)
	} else {
		fmt.Printf("%s log:\n", logName)
		fmt.Printf("%s\n", string(logEntry))
	}
	return nil
}

func archiveLogs() error {
	// In archive mode, "auto" means to attempt to get all logs (current & previous).
	if logType == logTypeAuto {
		logType = logTypeAll
		previous = true
		sidecars = true
	}

	// Create archive file.
	zipFileName = time.Now().Format(archiveFilenameFormat)
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter = zip.NewWriter(zipFile)
	defer zipWriter.Close()

	if err := getLogs(); err != nil {
		fmt.Fprintf(os.Stderr, "Errors collected during log aggregation. Please check %s for more information.\n",
			zipFileName)
	}

	if len(logErrors) > 0 {
		entry, err := zipWriter.Create("errors")
		if err != nil {
			return err
		}
		_, err = entry.Write(logErrors)
		if err != nil {
			return err
		}
		fmt.Printf("Wrote %s log to %s archive file.\n", "errors", zipFileName)
	}

	return nil
}

func consoleLogs() error {
	err := getLogs()

	SetExitCodeFromError(err)
	if err != nil {
		// Preserve anything written to stdout/stderr
		logMessage := strings.TrimSuffix(strings.TrimSpace(string(logErrors)), ".")
		if len(logMessage) > 0 {
			errMessage := strings.TrimSuffix(strings.TrimSpace(err.Error()), ".")
			return fmt.Errorf("%s. %s", errMessage, logMessage)
		} else {
			return err
		}
	}
	return nil
}

func getLogs() error {
	var err error

	if OperatingMode != ModeTunnel {
		return errors.New("'tridentctl logs' only supports Trident running in a Kubernetes pod")
	}

	switch logType {
	case logTypeTrident, logTypeAuto:
		if node == "" {
			err = getTridentLogs(logNameTrident)
		} else {
			err = getNodeLogs(logNameNode, node)
		}
	case logTypeTridentACP:
		if err = getTridentLogs(logNameTridentACP); err != nil {
			logErrors = appendErrorf(logErrors, "error retrieving ACP logs from running container: %s", err)
		}
	case logTypeAll:
		if err := getTridentLogs(logNameTrident); err != nil {
			logErrors = appendErrorf(logErrors, "error retrieving trident logs from running container: %s", err)
		}
		if node == "" {
			err = getAllNodeLogs(logNameNode)
		} else {
			err = getNodeLogs(logNameNode, node)
		}

		// getAllTridentResources fetches all Trident CRs
		getAllTridentResources()

		// getOtherResources fetches PV,PVC,VSClass,VSC,VS
		getOtherResources()

	case logTypeTridentOperator:
		err = getTridentOperatorLogs(logNameTridentOperator)
	}
	if err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving auxiliary logs: %s", err)
	}
	err = nil
	if previous {
		switch logType {
		case logTypeTrident, logTypeAuto:
			if node == "" {
				err = getTridentLogs(logNameTridentPrevious)
			} else {
				err = getNodeLogs(logNameNodePrevious, node)
			}
		case logTypeTridentACP:
			if err = getTridentLogs(logNameTridentACPPrevious); err != nil {
				logErrors = appendErrorf(logErrors, "error retrieving ACP logs from previously running container: %s",
					err)
			}
		case logTypeAll:
			err = getTridentLogs(logNameTridentPrevious)
			if err != nil {
				logErrors = appendErrorf(logErrors,
					"error retrieving trident logs from previously running container: %s", err)
			}
			if node == "" {
				err = getAllNodeLogs(logNameNodePrevious)
			} else {
				err = getNodeLogs(logNameNodePrevious, node)
			}
		case logTypeTridentOperator:
			err = getTridentOperatorLogs(logNameTridentOperatorPrevious)
		}
	}
	if err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving previous trident container logs: %s", err)
	}
	return nil
}

func checkValidLog() error {
	switch logType {
	case logTypeTrident, logTypeTridentACP, logTypeAuto, logTypeAll, logTypeTridentOperator:
		return nil
	default:
		return fmt.Errorf("%s is not a valid Trident log", logType)
	}
}

func getTridentLogs(logName string) error {
	var container string
	var prev bool

	switch logName {
	case logNameTrident:
		container, prev = config.ContainerTrident, false
	case logNameTridentPrevious:
		container, prev = config.ContainerTrident, true
	case logNameTridentACP:
		container, prev = config.ContainerACP, false
		sidecars = false
	case logNameTridentACPPrevious:
		container, prev = config.ContainerACP, true
		sidecars = false
	default:
		return fmt.Errorf("%s is not a valid Trident log", logName)
	}

	// Build command to get K8S logs
	prevArg := fmt.Sprintf("--previous=%v", prev)
	logsCommand := []string{"logs", TridentPodName, "-n", TridentPodNamespace, "-c", container, prevArg}

	if Debug {
		fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
	}

	// Get logs
	logBytes, err := execKubernetesCLI(logsCommand...)
	if err != nil {
		logErrors = appendError(logErrors, logBytes)
	} else {
		if err = writeLogs(logName, logBytes); err != nil {
			logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
		}
	}

	if sidecars {
		var tridentSidecars []string
		tridentSidecars, err = listTridentSidecars(TridentPodName, TridentPodNamespace)
		if err != nil {
			return fmt.Errorf("error listing trident sidecar containers; %v", err)
		}
		for _, sidecar := range tridentSidecars {
			logsCommand = []string{"logs", TridentPodName, "-n", TridentPodNamespace, "-c", sidecar, prevArg}

			if Debug {
				fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
			}

			// Get logs
			logBytes, err = execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(logName+"-sidecar-"+sidecar, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName+"-sidecar-"+sidecar, err)
				}
			}
		}
	}

	return err
}

func getNodeLogs(logName, nodeName string) error {
	var container string
	var prev bool

	switch logName {
	case logNameNode:
		container, prev = config.ContainerTrident, false
	case logNameNodePrevious:
		container, prev = config.ContainerTrident, true
	default:
		return fmt.Errorf("%s is not a valid Trident node log", logName)
	}

	pod, err := getTridentNode(nodeName, TridentPodNamespace)
	if err != nil {
		return fmt.Errorf("error listing trident node pods; %v", err)
	}

	nodeLogName := "trident-node-" + nodeName
	if prev {
		nodeLogName = nodeLogName + "-previous"
	}
	// Build command to get K8S logs
	prevArg := fmt.Sprintf("--previous=%v", prev)
	logsCommand := []string{"logs", pod, "-n", TridentPodNamespace, "-c", container, prevArg}

	if Debug {
		fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
	}

	// Get logs
	logBytes, err := execKubernetesCLI(logsCommand...)
	if err != nil {
		logErrors = appendError(logErrors, logBytes)
	} else {
		if err = writeLogs(nodeLogName, logBytes); err != nil {
			logErrors = appendErrorf(logErrors, "could not write log %s; %v", nodeLogName, err)
		}
	}

	if sidecars {
		var tridentSidecars []string
		tridentSidecars, err = listTridentSidecars(pod, TridentPodNamespace)
		if err != nil {
			return fmt.Errorf("error listing trident sidecar containers; %v", err)
		}
		for _, sidecar := range tridentSidecars {
			logsCommand = []string{"logs", pod, "-n", TridentPodNamespace, "-c", sidecar, prevArg}

			if Debug {
				fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
			}

			// Get logs
			logBytes, err = execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(nodeLogName+"-sidecar-"+sidecar, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", nodeLogName+"-sidecar-"+sidecar,
						err)
				}
			}
		}

	}
	return nil
}

func getAllNodeLogs(logName string) error {
	var container string
	var prev bool

	switch logName {
	case logNameNode:
		container, prev = config.ContainerTrident, false
	case logNameNodePrevious:
		container, prev = config.ContainerTrident, true
	default:
		return fmt.Errorf("%s is not a valid Trident node log", logName)
	}

	tridentNodeNames, err := listTridentNodes(TridentPodNamespace)
	if err != nil {
		return fmt.Errorf("error listing trident node pods; %v", err)
	}

	for node, pod := range tridentNodeNames {
		nodeLogName := "trident-node-" + node
		if prev {
			nodeLogName = nodeLogName + "-previous"
		}
		// Build command to get K8S logs
		prevArg := fmt.Sprintf("--previous=%v", prev)
		logsCommand := []string{"logs", pod, "-n", TridentPodNamespace, "-c", container, prevArg}

		if Debug {
			fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
		}

		// Get logs
		logBytes, err := execKubernetesCLI(logsCommand...)
		if err != nil {
			logErrors = appendError(logErrors, logBytes)
		} else {
			if err = writeLogs(nodeLogName, logBytes); err != nil {
				logErrors = appendErrorf(logErrors, "could not write log %s; %v", nodeLogName, err)
			}
		}

		if sidecars {
			var tridentSidecars []string
			tridentSidecars, err = listTridentSidecars(pod, TridentPodNamespace)
			if err != nil {
				return fmt.Errorf("error listing trident sidecar containers; %v", err)
			}
			for _, sidecar := range tridentSidecars {
				logsCommand = []string{"logs", pod, "-n", TridentPodNamespace, "-c", sidecar, prevArg}

				if Debug {
					fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
				}

				// Get logs
				logBytes, err = execKubernetesCLI(logsCommand...)
				if err != nil {
					logErrors = appendError(logErrors, logBytes)
				} else {
					if err = writeLogs(nodeLogName+"-sidecar-"+sidecar, logBytes); err != nil {
						logErrors = appendErrorf(logErrors, "could not write log %s; %v",
							nodeLogName+"-sidecar-"+sidecar, err)
					}
				}
			}
		}
	}
	return nil
}

func getTridentOperatorLogs(logName string) error {
	var container string
	var prev bool

	switch logName {
	case logNameTridentOperator:
		container, prev = config.OperatorContainerName, false
	case logNameTridentOperatorPrevious:
		container, prev = config.OperatorContainerName, true
	default:
		return fmt.Errorf("%s is not a valid Trident Operator log", logName)
	}

	// Build command to get K8S logs
	prevArg := fmt.Sprintf("--previous=%v", prev)
	logsCommand := []string{"logs", tridentOperatorPodName, "-n", tridentOperatorPodNamespace, "-c", container, prevArg}

	if Debug {
		fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
	}

	// Get logs
	logBytes, err := execKubernetesCLI(logsCommand...)
	if err != nil {
		logErrors = appendError(logErrors, logBytes)
	} else {
		if err = writeLogs(logName, logBytes); err != nil {
			logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
		}
	}

	return err
}

func getAllTridentResources() {
	if err := getAllTridentBackendConfigs(logNameTridentBackendConfig); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentBackendConfig logs : %v", err)
	}

	if err := getAllTridentActionMirrorUpdates(logNameTridentActionMirrorUpdate); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentActionMirrorUpdate logs : %v", err)
	}

	if err := getAllTridentActionSnapshotRestores(logNameTridentActionSnapshotRestore); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentActionSnapshotRestore logs : %v", err)
	}

	if err := getAllTridentBackends(logNameTridentBackend); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentBackend logs : %v", err)
	}

	if err := getAllTridentMirrorRelationships(logNameTridentMirrorRelationship); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentMirrorRelationship logs : %v", err)
	}

	if err := getAllTridentNodes(logNameTridentNode); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentNode logs : %v", err)
	}

	if err := getAllTridentSnapshotInfos(logNameTridentSnapshotInfo); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentSnapshotInfo logs : %v", err)
	}

	if err := getAllTridentSnapshots(logNameTridentSnapshot); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentSnapshot logs : %v", err)
	}

	if err := getAllTridentStorageClasses(logNameTridentStorageClass); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentStorageClass logs : %v", err)
	}

	if err := getAllTridentTransactions(logNameTridentTransaction); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentTransaction logs : %v", err)
	}

	if err := getAllTridentVersions(logNameTridentVersion); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentVersion logs : %v", err)
	}

	if err := getAllTridentVolumePublications(logNameTridentVolumePublication); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentVolumePublication logs : %v", err)
	}

	if err := getAllTridentVolumeReferences(logNameTridentVolumeReference); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentVolumeReference logs : %v", err)
	}

	if err := getAllTridentVolumes(logNameTridentVolume); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving TridentVolume logs : %v", err)
	}
}

func getOtherResources() {
	if err := getAllPersistentVolumes(logNamePersistentVolume); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving PersistentVolume logs : %v", err)
	}

	if err := getAllPersistentVolumeClaims(logNamePersistentVolumeClaim); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving PersistentVolumeClaim logs : %v", err)
	}

	if err := getAllVolumeSnapshotClasses(logNameVolumeSnapshotClass); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving VolumeSnapshotClass logs : %v", err)
	}

	if err := getAllVolumeSnapshotContents(logNameVolumeSnapshotContent); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving VolumeSnapshotContent logs : %v", err)
	}

	if err := getAllVolumeSnapshots(logNameVolumeSnapshot); err != nil {
		logErrors = appendErrorf(logErrors, "error retrieving VolumeSnapshot logs : %v", err)
	}
}

func getAllTridentBackendConfigs(logName string) error {
	tbcs, err := crdClientset.TridentV1().TridentBackendConfigs(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentBackendConfig; %v", err)
	}

	if len(tbcs.Items) == 0 {
		fmt.Println("resources of type 'TridentBackendConfig' not present")
		return nil
	}

	for _, tbc := range tbcs.Items {
		getCommand := []string{"get", "tridentbackendconfig", tbc.Name, "-o", "yaml", "-n", tbc.Namespace}
		getCommandFileName := logName + "/" + "get-" + tbc.Name

		describeCommand := []string{"describe", "tridentbackendconfig", tbc.Name, "-n", tbc.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tbc.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentActionMirrorUpdates(logName string) error {
	tamus, err := crdClientset.TridentV1().TridentActionMirrorUpdates(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentActionMirrorUpdate; %v", err)
	}

	if len(tamus.Items) == 0 {
		fmt.Println("resources of type 'TridentActionMirrorUpdate' not present")
		return nil
	}

	for _, tamu := range tamus.Items {
		getCommand := []string{"get", "tridentactionmirrorupdate", tamu.Name, "-o", "yaml", "-n", tamu.Namespace}
		getCommandFileName := logName + "/" + "get-" + tamu.Name

		describeCommand := []string{"describe", "tridentactionmirrorupdate", tamu.Name, "-n", tamu.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tamu.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentActionSnapshotRestores(logName string) error {
	tasrs, err := crdClientset.TridentV1().TridentActionSnapshotRestores(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentActionSnapshotRestore; %v", err)
	}

	if len(tasrs.Items) == 0 {
		fmt.Println("resources of type 'TridentActionSnapshotRestore' not present")
		return nil
	}

	for _, tasr := range tasrs.Items {
		getCommand := []string{"get", "tridentactionsnapshotrestore", tasr.Name, "-o", "yaml", "-n", tasr.Namespace}
		getCommandFileName := logName + "/" + "get-" + tasr.Name

		describeCommand := []string{"describe", "tridentactionsnapshotrestore", tasr.Name, "-n", tasr.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tasr.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentBackends(logName string) error {
	tbes, err := crdClientset.TridentV1().TridentBackends(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentBackend; %v", err)
	}

	if len(tbes.Items) == 0 {
		fmt.Println("resources of type 'TridentBackend' not present")
		return nil
	}

	for _, tbe := range tbes.Items {
		getCommand := []string{"get", "tridentbackend", tbe.Name, "-o", "yaml", "-n", tbe.Namespace}
		getCommandFileName := logName + "/" + "get-" + tbe.Name

		describeCommand := []string{"describe", "tridentbackend", tbe.Name, "-n", tbe.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tbe.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentMirrorRelationships(logName string) error {
	tmrs, err := crdClientset.TridentV1().TridentMirrorRelationships(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentMirrorRelationship; %v", err)
	}

	if len(tmrs.Items) == 0 {
		fmt.Println("resources of type 'TridentMirrorRelationship' not present")
		return nil
	}

	for _, tmr := range tmrs.Items {
		getCommand := []string{"get", "tridentmirrorrelationship", tmr.Name, "-o", "yaml", "-n", tmr.Namespace}
		getCommandFileName := logName + "/" + "get-" + tmr.Name

		describeCommand := []string{"describe", "tridentmirrorrelationship", tmr.Name, "-n", tmr.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tmr.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentNodes(logName string) error {
	tnodes, err := crdClientset.TridentV1().TridentNodes(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentNode; %v", err)
	}

	if len(tnodes.Items) == 0 {
		fmt.Println("resources of type 'TridentNode' not present")
		return nil
	}

	for _, tnode := range tnodes.Items {
		getCommand := []string{"get", "tridentnode", tnode.Name, "-o", "yaml", "-n", tnode.Namespace}
		getCommandFileName := logName + "/" + "get-" + tnode.Name

		describeCommand := []string{"describe", "tridentnode", tnode.Name, "-n", tnode.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tnode.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentSnapshotInfos(logName string) error {
	tsis, err := crdClientset.TridentV1().TridentSnapshotInfos(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentSnapshotInfo; %v", err)
	}

	if len(tsis.Items) == 0 {
		fmt.Println("resources of type 'TridentSnapshotInfo' not present")
		return nil
	}

	for _, tsi := range tsis.Items {
		getCommand := []string{"get", "tridentsnapshotinfo", tsi.Name, "-o", "yaml", "-n", tsi.Namespace}
		getCommandFileName := logName + "/" + "get-" + tsi.Name

		describeCommand := []string{"describe", "tridentsnapshotinfo", tsi.Name, "-n", tsi.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tsi.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentSnapshots(logName string) error {
	tsnaps, err := crdClientset.TridentV1().TridentSnapshots(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentSnapshot; %v", err)
	}

	if len(tsnaps.Items) == 0 {
		fmt.Println("resources of type 'TridentSnapshot' not present")
		return nil
	}

	for _, tsnap := range tsnaps.Items {
		getCommand := []string{"get", "tridentsnapshot", tsnap.Name, "-o", "yaml", "-n", tsnap.Namespace}
		getCommandFileName := logName + "/" + "get-" + tsnap.Name

		describeCommand := []string{"describe", "tridentsnapshot", tsnap.Name, "-n", tsnap.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tsnap.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentStorageClasses(logName string) error {
	tscs, err := crdClientset.TridentV1().TridentStorageClasses(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentStorageClass; %v", err)
	}

	if len(tscs.Items) == 0 {
		fmt.Println("resources of type 'TridentStorageClass' not present")
		return nil
	}

	for _, tsc := range tscs.Items {
		getCommand := []string{"get", "tridentstorageclass", tsc.Name, "-o", "yaml", "-n", tsc.Namespace}
		getCommandFileName := logName + "/" + "get-" + tsc.Name

		describeCommand := []string{"describe", "tridentstorageclass", tsc.Name, "-n", tsc.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tsc.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentTransactions(logName string) error {
	ttxs, err := crdClientset.TridentV1().TridentTransactions(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentTransaction; %v", err)
	}

	if len(ttxs.Items) == 0 {
		fmt.Println("resources of type 'TridentTransaction' not present")
		return nil
	}

	for _, ttx := range ttxs.Items {
		getCommand := []string{"get", "tridenttransaction", ttx.Name, "-o", "yaml", "-n", ttx.Namespace}
		getCommandFileName := logName + "/" + "get-" + ttx.Name

		describeCommand := []string{"describe", "tridenttransaction", ttx.Name, "-n", ttx.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + ttx.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentVersions(logName string) error {
	tvers, err := crdClientset.TridentV1().TridentVersions(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentVersion; %v", err)
	}

	if len(tvers.Items) == 0 {
		fmt.Println("resources of type 'TridentVersion' not present")
		return nil
	}

	for _, tver := range tvers.Items {
		getCommand := []string{"get", "tridentversion", tver.Name, "-o", "yaml", "-n", tver.Namespace}
		getCommandFileName := logName + "/" + "get-" + tver.Name

		describeCommand := []string{"describe", "tridentversion", tver.Name, "-n", tver.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tver.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentVolumePublications(logName string) error {
	tvps, err := crdClientset.TridentV1().TridentVolumePublications(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentVolumePublication; %v", err)
	}

	if len(tvps.Items) == 0 {
		fmt.Println("resources of type 'TridentVolumePublication' not present")
		return nil
	}

	for _, tvp := range tvps.Items {
		getCommand := []string{"get", "tridentvolumepublication", tvp.Name, "-o", "yaml", "-n", tvp.Namespace}
		getCommandFileName := logName + "/" + "get-" + tvp.Name

		describeCommand := []string{"describe", "tridentvolumepublication", tvp.Name, "-n", tvp.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tvp.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentVolumeReferences(logName string) error {
	tvrefs, err := crdClientset.TridentV1().TridentVolumeReferences(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentVolumeReference; %v", err)
	}

	if len(tvrefs.Items) == 0 {
		fmt.Println("resources of type 'TridentVolumeReference' not present")
		return nil
	}

	for _, tvref := range tvrefs.Items {
		getCommand := []string{"get", "tridentvolumereference", tvref.Name, "-o", "yaml", "-n", tvref.Namespace}
		getCommandFileName := logName + "/" + "get-" + tvref.Name

		describeCommand := []string{"describe", "tridentvolumereference", tvref.Name, "-n", tvref.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tvref.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllTridentVolumes(logName string) error {
	tvols, err := crdClientset.TridentV1().TridentVolumes(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return fmt.Errorf("error listing TridentVolume; %v", err)
	}

	if len(tvols.Items) == 0 {
		fmt.Println("resources of type 'TridentVolume' not present")
		return nil
	}

	for _, tvol := range tvols.Items {
		getCommand := []string{"get", "tridentvolume", tvol.Name, "-o", "yaml", "-n", tvol.Namespace}
		getCommandFileName := logName + "/" + "get-" + tvol.Name

		describeCommand := []string{"describe", "tridentvolume", tvol.Name, "-n", tvol.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + tvol.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllPersistentVolumes(logName string) error {
	pvs, err := k8sClient.GetPersistentVolumes()
	if err != nil {
		return fmt.Errorf("error listing PersistentVolume; %v", err)
	}

	if len(pvs) == 0 {
		fmt.Println("resources of type 'PersistentVolume' not present")
		return nil
	}

	for _, pv := range pvs {
		getCommand := []string{"get", "pv", pv.Name, "-o", "yaml"}
		getCommandFileName := logName + "/" + "get-" + pv.Name

		describeCommand := []string{"describe", "pv", pv.Name}
		describeCommandFileName := logName + "/" + "describe-" + pv.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllPersistentVolumeClaims(logName string) error {
	pvcs, err := k8sClient.GetPersistentVolumeClaims(true)
	if err != nil {
		return fmt.Errorf("error listing PersistentVolumeClaim; %v", err)
	}

	if len(pvcs) == 0 {
		fmt.Println("resources of type 'PersistentVolumeClaim' not present")
		return nil
	}

	for _, pvc := range pvcs {
		getCommand := []string{"get", "pvc", pvc.Name, "-o", "yaml", "-n", pvc.Namespace}
		getCommandFileName := logName + "/" + "get-" + pvc.Name

		describeCommand := []string{"describe", "pvc", pvc.Name, "-n", pvc.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + pvc.Name

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllVolumeSnapshotClasses(logName string) error {
	vsclasses, err := k8sClient.GetVolumeSnapshotClasses()
	if err != nil {
		return fmt.Errorf("error listing VolumeSnapshotClass; %v", err)
	}

	if len(vsclasses) == 0 {
		fmt.Println("resources of type 'VolumeSnapshotClass' not present")
		return nil
	}

	for _, vsclass := range vsclasses {
		getCommand := []string{"get", "volumesnapshotclass", vsclass.GetName(), "-o", "yaml"}
		getCommandFileName := logName + "/" + "get-" + vsclass.GetName()

		describeCommand := []string{"describe", "volumesnapshotclass", vsclass.GetName()}
		describeCommandFileName := logName + "/" + "describe-" + vsclass.GetName()

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllVolumeSnapshotContents(logName string) error {
	vscs, err := k8sClient.GetVolumeSnapshotContents()
	if err != nil {
		return fmt.Errorf("error listing VolumeSnapshotContent; %v", err)
	}

	if len(vscs) == 0 {
		fmt.Println("resources of type 'VolumeSnapshotContent' not present")
		return nil
	}

	for _, vsc := range vscs {
		getCommand := []string{"get", "volumesnapshotcontent", vsc.GetName(), "-o", "yaml"}
		getCommandFileName := logName + "/" + "get-" + vsc.GetName()

		describeCommand := []string{"describe", "volumesnapshotcontent", vsc.GetName()}
		describeCommandFileName := logName + "/" + "describe-" + vsc.GetName()

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func getAllVolumeSnapshots(logName string) error {
	vss, err := k8sClient.GetVolumeSnapshots(true)
	if err != nil {
		return fmt.Errorf("error listing VolumeSnapshotContent; %v", err)
	}

	if len(vss) == 0 {
		fmt.Println("resources of type 'VolumeSnapshotContent' not present")
		return nil
	}

	for _, vs := range vss {
		getCommand := []string{"get", "volumesnapshot", vs.GetName(), "-o", "yaml", "-n", vs.Namespace}
		getCommandFileName := logName + "/" + "get-" + vs.GetName()

		describeCommand := []string{"describe", "volumesnapshot", vs.GetName(), "-n", vs.Namespace}
		describeCommandFileName := logName + "/" + "describe-" + vs.GetName()

		commands := map[string][]string{getCommandFileName: getCommand, describeCommandFileName: describeCommand}
		for fileName, logsCommand := range commands {
			logBytes, err := execKubernetesCLI(logsCommand...)
			if err != nil {
				logErrors = appendError(logErrors, logBytes)
			} else {
				if err = writeLogs(fileName, logBytes); err != nil {
					logErrors = appendErrorf(logErrors, "could not write log %s; %v", logName, err)
				}
			}
		}
	}

	return nil
}

func appendError(oldErrors, newError []byte) []byte {
	if len(oldErrors) == 0 {
		return newError
	} else {
		oldErrorsStr := string(oldErrors)
		oldErrorsStr = strings.TrimSpace(oldErrorsStr)
		oldErrorsStr = strings.TrimSuffix(oldErrorsStr, ".")
		oldErrorsStr += ". "
		oldErrors = append([]byte(oldErrorsStr), newError...)
		return oldErrors
	}
}

// appendErrorf is a printf-like function to append a string-format error to a byte slice of related errors
// eg newErrors = appendErrorf(oldErrors, "this is my new %s", "error")
func appendErrorf(oldErrors []byte, formatString string, a ...interface{}) []byte {
	formattedString := fmt.Sprintf(formatString, a...)
	return appendError(oldErrors, []byte(formattedString))
}
