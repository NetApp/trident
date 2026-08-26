// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"time"

	db "github.com/netapp/trident/core/concurrent_cache"
	. "github.com/netapp/trident/logging"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage"
)

const (
	txnMonitorPeriod = 60 * time.Minute
	txnMonitorMaxAge = 24 * time.Hour
)

// StartTransactionMonitor starts the thread that reaps abandoned long-running transactions.
func (o *TridentOrchestrator) StartTransactionMonitor(
	ctx context.Context, txnPeriod, txnMaxAge time.Duration,
) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	o.txnMonitorTicker = time.NewTicker(txnPeriod)
	o.txnMonitorChannel = make(chan struct{})
	Logc(ctx).Debug("Transaction monitor started.")

	// Perform the check once and run it in a goroutine for every tick
	o.checkLongRunningTransactions(ctx, txnMaxAge)
	go func() {
		for {
			select {
			case tick := <-o.txnMonitorTicker.C:
				Logc(ctx).WithField("tick", tick).Debug("Transaction monitor running.")
				o.checkLongRunningTransactions(ctx, txnMaxAge)
			case <-o.txnMonitorChannel:
				Logc(ctx).Debugf("Transaction monitor stopped.")
				return
			}
		}
	}()
}

// StopTransactionMonitor stops the thread that reaps abandoned long-running transactions.
func (o *TridentOrchestrator) StopTransactionMonitor() {
	if o.txnMonitorTicker != nil {
		o.txnMonitorTicker.Stop()
	}
	if o.txnMonitorChannel != nil && !o.txnMonitorStopped {
		close(o.txnMonitorChannel)
		o.txnMonitorStopped = true
	}
	Logc(context.Background()).Debug("Transaction monitor stopped.")
}

// checkLongRunningTransactions is called periodically by the transaction monitor to
// see if any long-running transactions exist that have expired and must be reaped.
func (o *TridentOrchestrator) checkLongRunningTransactions(ctx context.Context, txnMaxAge time.Duration) {
	if o.bootstrapError != nil {
		Logc(ctx).WithField("error", o.bootstrapError).Errorf("Transaction monitor blocked by bootstrap error.")
		return
	}

	txns, err := o.storeClient.GetVolumeTransactions(ctx)
	if err != nil {
		if !persistentstore.MatchKeyNotFoundErr(err) {
			Logc(ctx).WithField("error", err).Errorf("Could not read transactions.")
		}
		return
	}
	Log().Debugf("Transaction monitor found %d long-running transaction(s).", len(txns))

	for _, txn := range txns {
		if !isResumableVolumeCreatingTransaction(txn) {
			continue
		}
		expirationTime := txn.VolumeCreatingConfig.StartTime.Add(txnMaxAge)
		Logc(ctx).WithFields(LogFields{
			"started": txn.VolumeCreatingConfig.StartTime,
			"expires": expirationTime,
			"op":      txn.Op,
			"name":    txn.Name(),
		}).Debug("Transaction monitor checking transaction.")
		if expirationTime.Before(time.Now()) {
			o.reapLongRunningTransaction(ctx, txn)
		}
	}
}

// reapLongRunningTransaction cleans up any transactions that have expired so that any
// storage resources associated with them are not orphaned indefinitely.
func (o *TridentOrchestrator) reapLongRunningTransaction(ctx context.Context, txn *storage.VolumeTransaction) {
	if txn == nil || txn.VolumeCreatingConfig == nil {
		return
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	currentTxn, err := o.GetVolumeTransaction(ctx, txn)
	if err != nil {
		Logc(ctx).WithError(err).Error("Could not re-read expired transaction.")
		return
	}
	if !isResumableVolumeCreatingTransaction(currentTxn) ||
		!currentTxn.VolumeCreatingConfig.StartTime.Equal(txn.VolumeCreatingConfig.StartTime) {
		return
	}

	volumeConfig := &currentTxn.VolumeCreatingConfig.VolumeConfig
	backendUUID := currentTxn.VolumeCreatingConfig.BackendUUID

	Logc(ctx).WithFields(LogFields{
		"op":   currentTxn.Op,
		"name": currentTxn.Name(),
	}).Debug("Transaction monitor reaping transaction.")

	deleteTxn := false
	switch {
	case o.volumes[volumeConfig.Name] != nil:
		Logc(ctx).WithField("volume", volumeConfig.Name).
			Warning("Volume for expired transaction is known to Trident and will not be reaped.")
		deleteTxn = true
	default:
		backend, found := o.backends[backendUUID]
		if !found {
			Logc(ctx).WithFields(LogFields{
				"backendUUID": backendUUID,
				"volume":      volumeConfig.Name,
			}).Error("Backend for expired transaction not found. Volume may have to be removed manually.")
		} else if removeErr := backend.RemoveVolume(ctx, volumeConfig); removeErr != nil {
			Logc(ctx).WithFields(LogFields{
				"backendUUID": backendUUID,
				"volume":      volumeConfig.Name,
				"error":       removeErr,
			}).Error("Volume for expired transaction not deleted. Volume may have to be removed manually.")
		} else {
			deleteTxn = true
		}
	}

	if !deleteTxn {
		return
	}

	if err = o.DeleteVolumeTransaction(ctx, currentTxn); err != nil {
		Logc(ctx).WithFields(LogFields{
			"op":   currentTxn.Op,
			"name": currentTxn.Name(),
		}).Error("Could not delete expired transaction. Transaction record may have to be removed manually.")
	}
}

// StartTransactionMonitor starts the concurrent core thread that reaps abandoned long-running transactions.
func (o *ConcurrentTridentOrchestrator) StartTransactionMonitor(
	ctx context.Context, txnPeriod, txnMaxAge time.Duration,
) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	o.mtx.Lock()
	o.txnMonitorTicker = time.NewTicker(txnPeriod)
	o.txnMonitorChannel = make(chan struct{})
	o.txnMonitorStopped = false
	ticker := o.txnMonitorTicker
	stopChannel := o.txnMonitorChannel
	o.mtx.Unlock()

	Logc(ctx).Debug("Transaction monitor started.")
	o.checkLongRunningTransactions(ctx, txnMaxAge)
	go func() {
		for {
			select {
			case tick := <-ticker.C:
				Logc(ctx).WithField("tick", tick).Debug("Transaction monitor running.")
				o.checkLongRunningTransactions(ctx, txnMaxAge)
			case <-stopChannel:
				Logc(ctx).Debug("Transaction monitor stopped.")
				return
			}
		}
	}()
}

// StopTransactionMonitor stops the concurrent core transaction monitor.
func (o *ConcurrentTridentOrchestrator) StopTransactionMonitor() {
	o.mtx.Lock()
	defer o.mtx.Unlock()

	if o.txnMonitorTicker != nil {
		o.txnMonitorTicker.Stop()
	}
	if o.txnMonitorChannel != nil && !o.txnMonitorStopped {
		close(o.txnMonitorChannel)
		o.txnMonitorStopped = true
	}
	Logc(context.Background()).Debug("Transaction monitor stopped.")
}

func (o *ConcurrentTridentOrchestrator) checkLongRunningTransactions(
	ctx context.Context, txnMaxAge time.Duration,
) {
	if o.bootstrapError != nil {
		Logc(ctx).WithField("error", o.bootstrapError).Error("Transaction monitor blocked by bootstrap error.")
		return
	}

	txns, err := o.storeClient.GetVolumeTransactions(ctx)
	if err != nil {
		if !persistentstore.MatchKeyNotFoundErr(err) {
			Logc(ctx).WithField("error", err).Error("Could not read transactions.")
		}
		return
	}
	Logc(ctx).Debugf("Transaction monitor found %d long-running transaction(s).", len(txns))

	for _, txn := range txns {
		if !isResumableVolumeCreatingTransaction(txn) {
			continue
		}
		expirationTime := txn.VolumeCreatingConfig.StartTime.Add(txnMaxAge)
		Logc(ctx).WithFields(LogFields{
			"started": txn.VolumeCreatingConfig.StartTime,
			"expires": expirationTime,
			"op":      txn.Op,
			"name":    txn.Name(),
		}).Debug("Transaction monitor checking transaction.")
		if expirationTime.Before(time.Now()) {
			o.reapLongRunningTransaction(ctx, txn)
		}
	}
}

func (o *ConcurrentTridentOrchestrator) reapLongRunningTransaction(
	ctx context.Context, txn *storage.VolumeTransaction,
) {
	if txn == nil || txn.VolumeCreatingConfig == nil {
		return
	}

	o.txnMutex.Lock(txn.Name())
	defer o.txnMutex.Unlock(txn.Name())

	currentTxn, err := o.GetVolumeTransaction(ctx, txn)
	if err != nil {
		Logc(ctx).WithError(err).Error("Could not re-read expired transaction.")
		return
	}
	if !isResumableVolumeCreatingTransaction(currentTxn) ||
		!currentTxn.VolumeCreatingConfig.StartTime.Equal(txn.VolumeCreatingConfig.StartTime) {
		return
	}

	volumeConfig := &currentTxn.VolumeCreatingConfig.VolumeConfig
	backendUUID := currentTxn.VolumeCreatingConfig.BackendUUID
	results, unlocker, err := db.Lock(
		ctx,
		db.Query(db.ReadVolume(volumeConfig.Name)),
		db.Query(db.ReadBackend(backendUUID)),
	)
	if err != nil {
		Logc(ctx).WithError(err).Error("Could not lock resources for expired transaction.")
		return
	}
	defer unlocker()

	Logc(ctx).WithFields(LogFields{
		"op":   currentTxn.Op,
		"name": currentTxn.Name(),
	}).Debug("Transaction monitor reaping transaction.")

	volume := results[0].Volume.Read
	backend := results[1].Backend.Read
	deleteTxn := false
	switch {
	case volume != nil:
		Logc(ctx).WithField("volume", volumeConfig.Name).
			Warning("Volume for expired transaction is known to Trident and will not be reaped.")
		deleteTxn = true
	case backend == nil:
		Logc(ctx).WithFields(LogFields{
			"backendUUID": backendUUID,
			"volume":      volumeConfig.Name,
		}).Error("Backend for expired transaction not found. Volume may have to be removed manually.")
	default:
		if removeErr := backend.RemoveVolume(ctx, volumeConfig); removeErr != nil {
			Logc(ctx).WithFields(LogFields{
				"backendUUID": backendUUID,
				"volume":      volumeConfig.Name,
				"error":       removeErr,
			}).Error("Volume for expired transaction not deleted. Volume may have to be removed manually.")
		} else {
			deleteTxn = true
		}
	}

	if !deleteTxn {
		return
	}

	if err = o.DeleteVolumeTransaction(ctx, currentTxn); err != nil {
		Logc(ctx).WithFields(LogFields{
			"op":   currentTxn.Op,
			"name": currentTxn.Name(),
		}).Error("Could not delete expired transaction. Transaction record may have to be removed manually.")
	}
}
