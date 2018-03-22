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
	LogLimitBytes         = 10485760 // 10 MiB
	tridentLogTrident     = "trident"
	tridentLogEtcd        = "etcd"
	archiveFilenameFormat = "support-2006-01-02T15-04-05-MST.zip"
)

var (
	Log     string
	archive bool
)

func init() {
	RootCmd.AddCommand(logsCmd)
	logsCmd.Flags().StringVarP(&Log, "log", "l", "auto", "Trident log to display. One of trident|etcd|auto|all")
	logsCmd.Flags().BoolVarP(&archive, "archive", "a", false, "Create a support archive with all logs unless otherwise specified.")
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

	// In archive mode, "auto" means to attempt to get all logs.
	if Log == "auto" {
		Log = "all"
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
	filename = time.Now().Format(archiveFilenameFormat)
	zipFile, err := os.Create(filename)
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
		fmt.Printf("Wrote %s log to %s archive file.\n", log, filename)
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

	switch OperatingMode {

	case ModeTunnel:
		switch Log {
		case "trident", "auto":
			err = getTridentLogs(tridentLogTrident, logMap)
		case "etcd":
			err = getTridentLogs(tridentLogEtcd, logMap)
		case "all":
			getTridentLogs(tridentLogTrident, logMap)
			getTridentLogs(tridentLogEtcd, logMap)
		}

	case ModeDirect:
		err = errors.New("'tridentctl logs' only supports Trident running in a Kubernetes pod")
	}

	return err
}

func checkValidLog() error {
	switch Log {
	case "trident", "etcd", "auto", "all":
		return nil
	default:
		return fmt.Errorf("%s is not a valid Trident log", Log)
	}
}

func getTridentLogs(log string, logMap map[string][]byte) error {

	var container string

	switch log {
	case "trident":
		container = config.ContainerTrident
	case "etcd":
		container = config.ContainerEtcd
	default:
		return fmt.Errorf("%s is not a valid Trident log", log)
	}

	// Build command to get K8S logs
	limit := fmt.Sprintf("--limit-bytes=%d", LogLimitBytes)
	logsCommand := []string{"logs", TridentPodName, "-n", TridentPodNamespace, "-c", container, limit}

	if Debug {
		fmt.Printf("Invoking command: %s %v\n", KubernetesCLI, strings.Join(logsCommand, " "))
	}

	// Get logs
	logBytes, err := exec.Command(KubernetesCLI, logsCommand...).CombinedOutput()
	if err != nil {
		logMap["error"] = appendError(logMap["error"], logBytes)
	} else {
		logMap[log] = logBytes
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
