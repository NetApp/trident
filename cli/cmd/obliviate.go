// Copyright 2020 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/logging"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
)

var forceObliviate bool

const (
	forceConfirmation   = "yesireallymeanit"
	crdConfirmation     = "Are you sure you want to wipe out all of Trident's custom resources and CRDs???"
	storageConfirmation = "Are you sure you want to wipe out all of Trident's storage resources???"
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
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	logLevel := "info"
	if silent {
		logLevel = "fatal"
	}
	err := logging.InitLogLevel(Debug, logLevel)
	if err != nil {
		log.WithField("error", err).Fatal("Failed to initialize logging.")
	}

	log.WithField("logLevel", log.GetLevel().String()).Debug("Initialized logging.")
}

func initClients() error {

	// Detect whether we are running inside a pod
	if namespaceBytes, err := ioutil.ReadFile(config.TridentNamespaceFile); err == nil {

		if !forceObliviate {
			return errors.New("obliviation canceled")
		}

		// The namespace file exists, so we're in a pod.  Create an API-based client.
		kubeConfig, err := rest.InClusterConfig()
		if err != nil {
			return err
		}
		resetNamespace = string(namespaceBytes)

		log.WithField("namespace", resetNamespace).Debug("Running in a pod, creating API-based clients.")

		if kubeClient, err = k8sclient.NewKubeClient(kubeConfig, resetNamespace, k8sTimeout); err != nil {
			return err
		}

		if crdClientset, err = crdclient.NewForConfig(kubeConfig); err != nil {
			return err
		}

	} else {

		if !forceObliviate {
			if forceObliviate, err = getUserConfirmation(crdConfirmation, obliviateCmd); err != nil {
				return err
			} else if !forceObliviate {
				return errors.New("obliviation canceled")
			}
		}

		// The namespace file didn't exist, so assume we're outside a pod.  Create a CLI-based client.
		log.WithField("kubeConfigPath", configPath).Debug("Running outside a pod, creating CLI-based client.")

		if kubeClient, err = k8sclient.NewKubectlClient("", k8sTimeout); err != nil {
			return err
		}
		resetNamespace = TridentPodNamespace
		if resetNamespace == "" {
			resetNamespace = kubeClient.Namespace()
		}

		if restConfig, err := clientcmd.BuildConfigFromFlags("", configPath); err != nil {
			return err
		} else if crdClientset, err = crdclient.NewForConfig(restConfig); err != nil {
			return err
		}
	}

	return nil
}
