// Copyright 2020 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

var forceObliviate bool

const (
	forceConfirmation   = "yesireallymeanit"
	crdConfirmation     = "Are you sure you want to wipe out all of Trident's custom resources and CRDs???"
	storageConfirmation = "Are you sure you want to wipe out all of Trident's storage resources???"
	secretConfirmation  = "Are you sure you want to wipe out all of Trident's secrets???"
)

func init() {
	RootCmd.AddCommand(obliviateCmd)
	obliviateCmd.PersistentFlags().BoolVar(&forceObliviate, forceConfirmation, false, "Obliviate without confirmation.")
}

var obliviateCmd = &cobra.Command{
	Use:    "obliviate",
	Short:  "Reset Trident state",
	Hidden: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		err := discoverOperatingMode(cmd)
		return err
	},
}

// initLogging configures logging. Logs are written to stdout.
func initLogging() {
	// Log to stdout only
	InitLogOutput(os.Stdout)
	InitLogFormatter(&log.TextFormatter{DisableTimestamp: true})

	initCmdLogging()
	logLevel := GetDefaultLogLevel()
	if silent {
		logLevel = "fatal"
		err := InitLogLevel(logLevel)
		if err != nil {
			Log().WithField("error", err).Fatal("Failed to initialize logging.")
		}
	}

	Log().WithField("logLevel", GetLogLevel()).Debug("Initialized logging.")
}

func initClients() error {
	clients, err := k8sclient.CreateK8SClients("", configPath, TridentPodNamespace)
	if err != nil {
		return err
	}
	clients.K8SClient.SetTimeout(k8sTimeout)
	k8sClient = clients.K8SClient
	crdClientset = clients.TridentClient

	// Detect whether we are running inside a pod
	if clients.InK8SPod {

		if !forceObliviate {
			return errors.New("obliviation canceled")
		}

		Log().Debug("Running in a pod.")

	} else {
		Log().Debug("Running outside a pod.")
	}

	return nil
}

func confirmObliviate(confirmation string) error {
	if !forceObliviate {
		if forceObliviate, err := getUserConfirmation(confirmation, obliviateCmd); err != nil {
			return err
		} else if !forceObliviate {
			return errors.New("obliviation canceled")
		}
	}

	return nil
}
