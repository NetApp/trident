// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"time"

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

	// Build map of long-running transactions
	txnMap := make(map[*storage.VolumeTransaction]time.Time)

	for _, txn := range txns {
		switch txn.Op {
		case storage.VolumeCreating:
			txnMap[txn] = txn.VolumeCreatingConfig.StartTime
		default:
			continue
		}
	}

	// Reap each long-running transaction that has expired
	for txn, startTime := range txnMap {

		expirationTime := startTime.Add(txnMaxAge)

		Logc(ctx).WithFields(LogFields{
			"started": startTime,
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
	o.mutex.Lock()
	defer o.mutex.Unlock()

	Logc(ctx).WithFields(LogFields{
		"op":   txn.Op,
		"name": txn.Name(),
	}).Debug("Transaction monitor reaping transaction.")

	// Clean up any resources associated with the transaction.
	switch txn.Op {
	case storage.VolumeCreating:

		// If the volume was somehow fully created and the transaction was left around, don't delete the volume!
		if _, found := o.volumes[txn.VolumeCreatingConfig.Name]; found {

			Logc(ctx).WithFields(LogFields{
				"volume": txn.VolumeCreatingConfig.Name,
			}).Warning("Volume for expired transaction is known to Trident and will not be reaped.")
			break
		}

		// Get the backend where this abandoned volume may still exist
		backend, found := o.backends[txn.VolumeCreatingConfig.BackendUUID]
		if !found {

			Logc(ctx).WithFields(LogFields{
				"backendUUID": txn.VolumeCreatingConfig.BackendUUID,
				"volume":      txn.VolumeCreatingConfig.Name,
			}).Error("Backend for expired transaction not found. Volume may have to be removed manually.")
			break
		}

		// Delete the volume.  This should be safe since the transaction was left around and Trident doesn't
		// know anything about the volume.
		if err := backend.RemoveVolume(ctx, &txn.VolumeCreatingConfig.VolumeConfig); err != nil {

			Logc(ctx).WithFields(LogFields{
				"backendUUID": txn.VolumeCreatingConfig.BackendUUID,
				"volume":      txn.VolumeCreatingConfig.Name,
				"error":       err,
			}).Error("Volume for expired transaction not deleted. Volume may have to be removed manually.")
			break
		}

	default:
		break
	}

	// Delete the transaction record in all cases.
	if err := o.DeleteVolumeTransaction(ctx, txn); err != nil {
		Logc(ctx).WithFields(LogFields{
			"op":   txn.Op,
			"name": txn.Name(),
		}).Error("Could not delete expired transaction. Transaction record may have to be removed manually.")
	}
}
