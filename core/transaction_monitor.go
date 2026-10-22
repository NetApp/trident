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
	for _, txn := range expiredCreatingTransactions(ctx, o.storeClient, txnMaxAge) {
		o.reapLongRunningTransaction(ctx, txn)
	}
}

// reapLongRunningTransaction cleans up any transactions that have expired so that any
// storage resources associated with them are not orphaned indefinitely.
func (o *TridentOrchestrator) reapLongRunningTransaction(ctx context.Context, txn *storage.VolumeTransaction) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	currentTxn, err := o.GetVolumeTransaction(ctx, txn)
	if err != nil {
		Logc(ctx).WithError(err).Error("Could not re-read expired transaction.")
		return
	}
	if !stillExpired(currentTxn, txn) {
		return
	}

	volumeConfig := &currentTxn.VolumeCreatingConfig.VolumeConfig
	backendUUID := currentTxn.VolumeCreatingConfig.BackendUUID
	if reapExpiredCreate(ctx, currentTxn, o.volumes[volumeConfig.Name] != nil, o.backends[backendUUID]) {
		deleteReapedTransaction(ctx, currentTxn, o.DeleteVolumeTransaction)
	}
}

// StartTransactionMonitor starts the thread that reaps abandoned VolumeCreating transactions.
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

// StopTransactionMonitor stops the thread that reaps abandoned VolumeCreating transactions.
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
	for _, txn := range expiredCreatingTransactions(ctx, o.storeClient, txnMaxAge) {
		o.reapLongRunningTransaction(ctx, txn)
	}
}

func (o *ConcurrentTridentOrchestrator) reapLongRunningTransaction(
	ctx context.Context, txn *storage.VolumeTransaction,
) {
	o.txnMutex.Lock(txn.Name())
	defer o.txnMutex.Unlock(txn.Name())

	currentTxn, err := o.GetVolumeTransaction(ctx, txn)
	if err != nil {
		Logc(ctx).WithError(err).Error("Could not re-read expired transaction.")
		return
	}
	if !stillExpired(currentTxn, txn) {
		return
	}

	volumeName := currentTxn.VolumeCreatingConfig.Name
	backendUUID := currentTxn.VolumeCreatingConfig.BackendUUID
	_, results, unlocker, err := db.Lock(ctx, db.Query(db.ReadVolume(volumeName), db.ReadBackend(backendUUID)))
	if err != nil {
		Logc(ctx).WithError(err).Error("Could not lock resources for expired transaction.")
		return
	}
	defer unlocker()

	if reapExpiredCreate(ctx, currentTxn, results[0].Volume.Read != nil, results[0].Backend.Read) {
		deleteReapedTransaction(ctx, currentTxn, o.DeleteVolumeTransaction)
	}
}

// expiredCreatingTransactions returns the resumable creating transactions that started more than txnMaxAge ago.
func expiredCreatingTransactions(
	ctx context.Context, store persistentstore.Client, txnMaxAge time.Duration,
) []*storage.VolumeTransaction {
	txns, err := store.GetVolumeTransactions(ctx)
	if err != nil {
		if !persistentstore.MatchKeyNotFoundErr(err) {
			Logc(ctx).WithField("error", err).Error("Could not read transactions.")
		}
		return nil
	}
	Logc(ctx).Debugf("Transaction monitor found %d long-running transaction(s).", len(txns))

	var expired []*storage.VolumeTransaction
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
			expired = append(expired, txn)
		}
	}
	return expired
}

// stillExpired reports whether current, re-read under the caller's lock, is still the expired transaction the
// monitor found, so a create that resumed or finished in the meantime is left alone.
func stillExpired(current, expired *storage.VolumeTransaction) bool {
	return isResumableVolumeCreatingTransaction(current) &&
		current.VolumeCreatingConfig.StartTime.Equal(expired.VolumeCreatingConfig.StartTime)
}

// reapExpiredCreate removes the backend volume an abandoned create left behind and reports whether its
// transaction can now be deleted. A volume Trident already knows is never removed; its transaction is simply
// stale. Without the backend, or if the removal fails, the transaction is kept for a later attempt.
func reapExpiredCreate(
	ctx context.Context, txn *storage.VolumeTransaction, volumeKnown bool, backend storage.Backend,
) bool {
	volumeConfig := &txn.VolumeCreatingConfig.VolumeConfig
	backendUUID := txn.VolumeCreatingConfig.BackendUUID

	Logc(ctx).WithFields(LogFields{"op": txn.Op, "name": txn.Name()}).Debug("Transaction monitor reaping transaction.")

	switch {
	case volumeKnown:
		Logc(ctx).WithField("volume", volumeConfig.Name).
			Warning("Volume for expired transaction is known to Trident and will not be reaped.")
		return true
	case backend == nil:
		Logc(ctx).WithFields(LogFields{
			"backendUUID": backendUUID,
			"volume":      volumeConfig.Name,
		}).Error("Backend for expired transaction not found. Volume may have to be removed manually.")
		return false
	}

	if err := backend.RemoveVolume(ctx, volumeConfig); err != nil {
		Logc(ctx).WithFields(LogFields{
			"backendUUID": backendUUID,
			"volume":      volumeConfig.Name,
			"error":       err,
		}).Error("Volume for expired transaction not deleted. Volume may have to be removed manually.")
		return false
	}
	return true
}

func deleteReapedTransaction(
	ctx context.Context, txn *storage.VolumeTransaction,
	deleteTxn func(context.Context, *storage.VolumeTransaction) error,
) {
	if err := deleteTxn(ctx, txn); err != nil {
		Logc(ctx).WithFields(LogFields{
			"op":   txn.Op,
			"name": txn.Name(),
		}).Error("Could not delete expired transaction. Transaction record may have to be removed manually.")
	}
}
