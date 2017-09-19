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
	LOG_LIMIT_BYTES         = 10485760 // 10 MiB
	tridentLauncherPodName  = "trident-launcher"
	tridentEphemeralPodName = "trident-ephemeral"
	tridentLogLauncher      = "launcher"
	tridentLogEphemeral     = "ephemeral"
	tridentLogTrident       = "trident"
	tridentLogEtcd          = "etcd"
	archiveFilenameFormat   = "support-2006-01-02T15-04-05-MST.zip"
)

var (
	log     string
	archive bool
)

func init() {
	RootCmd.AddCommand(logsCmd)
	logsCmd.Flags().StringVarP(&log, "log", "l", "auto", "Trident log to display. One of trident|etcd|launcher|ephemeral|auto|all")
	logsCmd.Flags().BoolVarP(&archive, "archive", "a", false, "Create a support archive with all logs unless otherwise specified.")
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print the logs from Trident",
	Long:  "Print the logs from the Trident storage orchestrator for Kubernetes",
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
	if log == "auto" {
		log = "all"
	}

	logMap := make(map[string][]byte)
	getLogs(logMap)

	// If there aren't any logs, bail out.
	anyLogs := false
	for log, _ := range logMap {
		if log != "error" {
			anyLogs = true
			break
		}
	}
	if !anyLogs {
		return errors.New("no Trident-related logs found.")
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
			return fmt.Errorf("%s. %s.", errMessage, logMessage)
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
		return errors.New("no Trident-related logs found.")
	}

	return nil
}

func getLogs(logMap map[string][]byte) error {

	var err error = nil

	switch OperatingMode {

	case MODE_LOGS:
		switch log {
		case "trident", "etcd":
			return fmt.Errorf("could not find a Trident pod in the %s namespace.", TridentPodNamespace)
		case "ephemeral":
			err = getPodLogs(tridentLogEphemeral, logMap)
		case "launcher":
			err = getPodLogs(tridentLogLauncher, logMap)
		case "auto":
			err = getPodLogs(tridentLogEphemeral, logMap)
			if err != nil {
				err = getPodLogs(tridentLogLauncher, logMap)
			}
		case "all":
			getPodLogs(tridentLogEphemeral, logMap)
			getPodLogs(tridentLogLauncher, logMap)
		}

	case MODE_TUNNEL:
		switch log {
		case "ephemeral":
			err = getPodLogs(tridentLogEphemeral, logMap)
		case "launcher":
			err = getPodLogs(tridentLogLauncher, logMap)
		case "trident", "auto":
			err = getTridentLogs(tridentLogTrident, logMap)
		case "etcd":
			err = getTridentLogs(tridentLogEtcd, logMap)
		case "all":
			getPodLogs(tridentLogEphemeral, logMap)
			getPodLogs(tridentLogLauncher, logMap)
			getTridentLogs(tridentLogTrident, logMap)
			getTridentLogs(tridentLogEtcd, logMap)
		}

	case MODE_DIRECT:
		err = errors.New("'tridentctl logs' only supports Trident running in a Kubernetes pod.")
	}

	return err
}

func checkValidLog() error {
	switch log {
	case "trident", "etcd", "launcher", "ephemeral", "auto", "all":
		return nil
	default:
		return fmt.Errorf("%s is not a valid Trident log", log)
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

	// Build command for 'kubectl logs'
	limit := fmt.Sprintf("--limit-bytes=%d", LOG_LIMIT_BYTES)
	logsCommand := []string{"logs", TridentPodName, "-n", TridentPodNamespace, "-c", container, limit}

	if Debug {
		fmt.Printf("Invoking command: kubectl %v\n", strings.Join(logsCommand, " "))
	}

	// Get logs
	logBytes, err := exec.Command("kubectl", logsCommand...).CombinedOutput()
	if err != nil {
		logMap["error"] = appendError(logMap["error"], logBytes)
	} else {
		logMap[log] = logBytes
	}
	return err
}

func getPodLogs(log string, logMap map[string][]byte) error {

	var pod string

	switch log {
	case "launcher":
		pod = tridentLauncherPodName
	case "ephemeral":
		pod = tridentEphemeralPodName
	default:
		return fmt.Errorf("%s is not a valid Trident log", log)
	}

	// Build command for 'kubectl logs'
	limit := fmt.Sprintf("--limit-bytes=%d", LOG_LIMIT_BYTES)
	logsCommand := []string{"logs", pod, "-n", TridentPodNamespace, limit}

	if Debug {
		fmt.Printf("Invoking command: kubectl %v\n", strings.Join(logsCommand, " "))
	}

	// Get logs
	logBytes, err := exec.Command("kubectl", logsCommand...).CombinedOutput()
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
