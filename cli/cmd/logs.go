// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/netapp/trident/config"
	"github.com/spf13/cobra"
)

const (
	LogLimitBytes = 10485760 // 10 MiB

	logNameTrident         = "trident"
	logNameTridentPrevious = "trident-previous"
	logNameEtcd            = "etcd"
	logNameEtcdPrevious    = "etcd-previous"

	logTypeAuto    = "auto"
	logTypeTrident = "trident"
	logTypeEtcd    = "etcd"
	logTypeAll     = "all"

	archiveFilenameFormat = "support-2006-01-02T15-04-05-MST.zip"
)

var (
	logType  string
	archive  bool
	previous bool
)

func init() {
	RootCmd.AddCommand(logsCmd)
	logsCmd.Flags().StringVarP(&logType, "log", "l", logTypeAuto, "Trident log to display. One of trident|etcd|auto|all")
	logsCmd.Flags().BoolVarP(&archive, "archive", "a", false, "Create a support archive with all logs unless otherwise specified.")
	logsCmd.Flags().BoolVarP(&previous, "previous", "p", false, "Get the logs for the previous container instance if it exists.")
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print the logs from Trident",
	Long:  "Print the logs from the Trident storage orchestrator for Kubernetes",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		err := discoverOperatingMode(cmd)
		return err
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

func archiveLogs() error {

	// In archive mode, "auto" means to attempt to get all logs (current & previous).
	if logType == logTypeAuto {
		logType = logTypeAll
		previous = true
	}

	logMap := make(map[string][]byte)
	getLogs(logMap)

	// If there aren't any logs, bail out.
	anyLogs := false
	for log := range logMap {
		if log != "error" {
			anyLogs = true
			break
		}
	}
	if !anyLogs {
		return errors.New("no Trident-related logs found")
	}

	// Create archive file.
	zipFileName := time.Now().Format(archiveFilenameFormat)
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Write to the archive file.
	for log, logBytes := range logMap {
		entry, err := zipWriter.Create(log)
		if err != nil {
			return err
		}
		_, err = entry.Write(logBytes)
		if err != nil {
			return err
		}
		fmt.Printf("Wrote %s log to %s archive file.\n", log, zipFileName)
	}

	return nil
}

func consoleLogs() error {

	logMap := make(map[string][]byte)
	err := getLogs(logMap)

	SetExitCodeFromError(err)
	if err != nil {
		// Preserve anything written to stdout/stderr
		logMessage := strings.TrimSuffix(strings.TrimSpace(string(logMap["error"])), ".")
		if len(logMessage) > 0 {
			errMessage := strings.TrimSuffix(strings.TrimSpace(err.Error()), ".")
			return fmt.Errorf("%s. %s", errMessage, logMessage)
		} else {
			return err
		}
	}

	// Print to the console
	anyLogs := false
	for log, logBytes := range logMap {
		if log != "error" {
			fmt.Printf("%s log:\n", log)
			fmt.Printf("%s\n", string(logBytes))
			anyLogs = true
		}
	}
	if !anyLogs {
		return errors.New("no Trident-related logs found")
	}

	return nil
}

func getLogs(logMap map[string][]byte) error {

	var err error

	if OperatingMode != ModeTunnel {
		return errors.New("'tridentctl logs' only supports Trident running in a Kubernetes pod")
	}

	switch logType {
	case logTypeTrident, logTypeAuto:
		err = getTridentLogs(logNameTrident, logMap)
	case logTypeEtcd:
		err = getTridentLogs(logNameEtcd, logMap)
	case logTypeAll:
		getTridentLogs(logNameTrident, logMap)
		getTridentLogs(logNameEtcd, logMap)
	}

	if previous {
		switch logType {
		case logTypeTrident, logTypeAuto:
			getTridentLogs(logNameTridentPrevious, logMap)
		case logTypeEtcd:
			getTridentLogs(logNameEtcdPrevious, logMap)
		case logTypeAll:
			getTridentLogs(logNameTridentPrevious, logMap)
			getTridentLogs(logNameEtcdPrevious, logMap)
		}
	}

	return err
}

func checkValidLog() error {
	switch logType {
	case logTypeTrident, logTypeEtcd, logTypeAuto, logTypeAll:
		return nil
	default:
		return fmt.Errorf("%s is not a valid Trident log", logType)
	}
}

func getTridentLogs(logName string, logMap map[string][]byte) error {

	var container string
	var prev bool

	switch logName {
	case logNameTrident:
		container, prev = config.ContainerTrident, false
	case logNameTridentPrevious:
		container, prev = config.ContainerTrident, true
	case logNameEtcd:
		container, prev = config.ContainerEtcd, false
	case logNameEtcdPrevious:
		container, prev = config.ContainerEtcd, true
	default:
		return fmt.Errorf("%s is not a valid Trident log", logName)
	}

	// Build command to get K8S logs
	limitArg := fmt.Sprintf("--limit-bytes=%d", LogLimitBytes)
	prevArg := fmt.Sprintf("--previous=%v", prev)
	logsCommand := []string{"logs", TridentPodName, "-n", TridentPodNamespace, "-c", container, limitArg, prevArg}

	if Debug {
		fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
	}

	// Get logs
	logBytes, err := exec.Command(KubernetesCLI, logsCommand...).CombinedOutput()
	if err != nil {
		logMap["error"] = appendError(logMap["error"], logBytes)
	} else {
		logMap[logName] = logBytes
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
