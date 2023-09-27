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
