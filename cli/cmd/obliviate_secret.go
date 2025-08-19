// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func init() {
	obliviateCmd.AddCommand(obliviateSecretCmd)
}

var obliviateSecretCmd = &cobra.Command{
	Use:              "secret",
	Short:            "Reset Trident's Secret state (deletes all Trident Secrets present in a cluster)",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		initLogging()

		if OperatingMode == ModeTunnel {
			if !forceObliviate {
				if forceObliviate, err = getUserConfirmation(secretConfirmation, cmd); err != nil {
					return err
				} else if !forceObliviate {
					return errors.New("obliviation canceled")
				}
			}
			command := []string{"obliviate", "secret", fmt.Sprintf("--%s", forceConfirmation)}
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		} else {
			if err := initClients(); err != nil {
				return err
			}

			if err := confirmObliviate(secretConfirmation); err != nil {
				return err
			}

			// mischief managed!
			return obliviateSecrets()
		}
	},
}

func obliviateSecrets() error {
	// Relying on the trident-csi label is safe because both ephemeral and persistent Secrets will have it.
	secrets, err := k8sClient.GetSecretsByLabel(TridentCSILabel, true)
	if err != nil {
		return err
	} else if len(secrets) == 0 {
		Log().Debug("No Trident secrets were found.")
		return nil
	}

	// Delete all secrets.
	if err := deleteSecrets(secrets); err != nil {
		return err
	}

	Log().Infof("Reset Trident's secret state.")

	return nil
}

func deleteSecrets(secrets []v1.Secret) error {
	for _, secret := range secrets {

		logFields := LogFields{
			"secret":    secret.Name,
			"namespace": secret.Namespace,
		}

		Log().WithFields(logFields).Debug("Deleting Trident secret.")

		// Try deleting the Secret. We don't add finalizers to the secret, so we shouldn't need to check for them.
		if secret.DeletionTimestamp.IsZero() {
			Log().WithFields(logFields).Debug("Deleting Secret.")

			err := k8sClient.DeleteSecret(secret.Name, secret.Namespace)
			if isNotFoundError(err) {
				Log().WithFields(logFields).Info("Secret not found during deletion.")
				continue
			} else if err != nil {
				Log().WithFields(logFields).Errorf("Could not delete Secret; %v", err)
				return err
			}
		} else {
			Log().WithFields(logFields).Debug("Secret already has deletion timestamp.")
		}

		// Wait for the Secret to delete.
		if err := waitForSecretDeletion(secret.Name, k8sTimeout); err != nil {
			Log().WithFields(logFields).Error(err)
			return err
		}

		Log().WithFields(logFields).Info("Secret deleted.")
	}

	return nil
}

func waitForSecretDeletion(name string, timeout time.Duration) error {
	retries := 0

	checkDeleted := func() error {
		exists, err := k8sClient.CheckSecretExists(name)
		if !exists || isNotFoundError(err) {
			return nil
		}

		return fmt.Errorf("secret %s not yet deleted", name)
	}

	checkDeletedNotify := func(err error, duration time.Duration) {
		Log().WithFields(LogFields{
			"secret": name,
			"err":    err,
		}).Debug("Secret not yet deleted, waiting.")

		retries++
	}

	deleteBackoff := backoff.NewExponentialBackOff()
	deleteBackoff.InitialInterval = 1 * time.Second
	deleteBackoff.RandomizationFactor = 0.1
	deleteBackoff.Multiplier = 1.414
	deleteBackoff.MaxInterval = 5 * time.Second
	deleteBackoff.MaxElapsedTime = timeout

	Log().WithField("secret", name).Trace("Waiting for Secret to be deleted.")

	if err := backoff.RetryNotify(checkDeleted, deleteBackoff, checkDeletedNotify); err != nil {
		return fmt.Errorf("secret %s was not deleted after %3.2f seconds", name, timeout.Seconds())
	}

	Log().WithFields(LogFields{
		"secret":      name,
		"retries":     retries,
		"waitSeconds": fmt.Sprintf("%3.2f", deleteBackoff.GetElapsedTime().Seconds()),
	}).Debugf("Secret deleted.")

	return nil
}
