.. _backup_disaster_recovery:

****************************
Backup and Disaster Recovery
****************************

Protecting application data is one of the fundamental purposes of any storage system. Whether an application is cloud native, 12 factor, a microservice, or any other architecture, the application data still needs to be protected.

NetApp's storage platforms provide data protection and recoverability options which vary based on recovery time and acceptable data loss requirements. Trident can provision volumes which can take advantage of some of these features, however, a full data protection and recovery strategy should be evaluated for each application with a persistence requirement.

ONTAP Snapshots
===============

Snapshots play an important role by providing point-in-time recovery options for application data. However, snapshots are not backups by themselves, they will not protect against storage system failure or other catastrophes. But, they are a convenient, quick, and easy way to recover data in most scenarios.

Using ONTAP snapshots with containers
-------------------------------------

If the snapshot policy has not been defined in the backend, it defaults to using the ``none`` policy. This results in ONTAP taking no automatic snapshots. However, the storage administrator can take manual snapshots or change the snapshot policy via the ONTAP management interface. This will not affect Trident operation.

The snapshot directory is hidden by default. This helps facilitate maximum compatibility of volumes provisioned using the ``ontap-nas`` and ``ontap-nas-economy`` drivers.

Accessing the snapshot directory
--------------------------------

Enable the ``.snapshot`` directory when using the ``ontap-nas`` and ``ontap-nas-economy`` drivers to allow applications to recover data from snapshots directly. More information about how to enable access and enable self-service data recovery can be found in `this blog post <https://netapp.io/2018/04/03/self-service-data-recovery-using-trident-nfs/>`_ on `netapp.io <https://netapp.io/>`_.

Restoring the snapshots
-----------------------

Restore a volume to a state recorded in a prior snapshot using the ``volume snapshot restore`` ONTAP CLI command. When you restore a Snapshot copy, the restore operation overwrites the existing volume configuration. Any changes made to the data in the volume after the Snapshot copy was created are lost.

 .. code-block:: python 

   cluster1::*> volume snapshot restore -vserver vs0 -volume vol3 -snapshot vol3_snap_archive
   

SolidFire snapshots
===================

Backup data on a SolidFire Volume by setting a snapshot schedule to a SolidFire volume, ensuring the snapshots are taken at the required intervals. Currently, it is not possible to set a snapshot schedule to a volume through the ``solidfire-san`` driver. Set it using the Element OS Web UI or Element OS APIs.

In the event of data corruption, we can choose a particular snapshot and rollback the volume to the snapshot manually using the Element OS Web UI or Element OS APIs. This reverts any changes made to the volume since the snapshot was created.


Trident etcd snapshots using ``etcdctl`` command line utility
=============================================================

The etcdctl command line utility offers the provision to take snapshots of an etcd cluster. It also enables us to restore the previously taken snapshot.

etcdctl snapshot backup
-----------------------

The command ``etcdctl snapshot save /var/etcd/data/snapshot.db`` enables us to take a point-in-time snapshot of the etcd cluster. NetApp recommends using a script to take timely backups. This command can be executed from within the etcd container or the command can be deployed using the ``kubectl exec`` command directly. Store the periodic snapshots under the persistent Trident NetApp volume `/var/etcd/data` so that snapshots are stored securely and can safely be recovered should the trident pod be lost. Periodically check the volume to be sure it does not run out of space.

etcdctl snapshot restore
------------------------

In the event of accidental deletion or corruption of Trident etcd data, we can choose the appropriate snapshot and restore it back using the command ``etcdctl snapshot restore snapshot.db --data-dir /var/etcd/data/etcd-test2 --name etcd1``.  Take note to restore the snapshot on to a different folder [shown in the above command as /var/etcd/data/etcd-test2 which is on the mount] inside the Trident NetApp volume.

After the restore is complete, uninstall Trident. Take note not to use the "-a" flag during uninstallation. Mount the Trident volume manually on the host and make sure that the current "member" folder under /var/etcd/data is deleted. Copy the "member" folder from the restored folder /var/etcd/data/etcd-test2 to /var/etcd/data. After the copy is complete, re-install Trident. Verify if the restore and recovery has been completed successfully by making sure all the required data is present.

Data replication using ONTAP
============================

Replicating data can play an important role in protecting against data loss due to storage array failure. Snapshots are a point-in-time recovery which provides a very quick and easy method of recovering data which has been corrupted or accidentally lost as a result of human or technological error. However, they cannot protect against catastrophic failure of the storage array itself. 

ONTAP SnapMirror SVM Replication
--------------------------------

SnapMirror can be used to replicate a complete SVM which includes its configuration settings and its volumes. In the event of a disaster, SnapMirror destination SVM can be activated to start serving data and switch back to the primary when the systems are restored.
Since Trident is unable to configure replication relationships itself, the storage administrator can use ONTAP’s SnapMirror SVM Replication feature to automatically replicate volumes to a Disaster Recovery (DR) destination. 

* A distinct backend should be created for each SVM which has SVM-DR enabled.

* Storage Classes should be crafted so as to not select the replicated backends except when desired. This is important to avoid having volumes which do not need the protection of a replication relationship to be provisioned onto the backend(s) that support SVM-DR.

* Application administrators should understand the additional cost and complexity associated with replicating the data and a plan for recovery should be determined before they leverage data replication.

* Before activating the SnapMirror destination SVM, stop all the scheduled SnapMirror transfers, abort all ongoing SnapMirror transfers, break the replication relationship, stop the source SVM, and then start the SnapMirror destination SVM.

* Trident does not automatically detect SVM failures. Therefore, upon a failure, the administrator needs to run the command tridentctl backend update to trigger Trident’s failover to the new backend.


ONTAP SnapMirror SVM Replication Setup 
--------------------------------------

* Set up peering between the Source and Destination Cluster and SVM.

* Setting up SnapMirror SVM replication involves creating the destination SVM by using the ``-subtype dp-destination`` option.

* Create a replication job schedule to make sure that replication happens in the required intervals.

* Create a SnapMirror replication from destination SVM to the source SVM using the ``-identity-preserve true`` option to make sure that source SVM configurations and source SVM interfaces are copied to the destination. From the destination SVM, initialize the SnapMirror SVM replication relationship.


.. _figSVMDR1:

.. figure:: images/SVMDR1.PNG
     :align: center
     :figclass: align-center

SnapMirror SVM Replication Setup
 



SnapMirror SVM Disaster Recovery Workflow for Trident
-----------------------------------------------------

The following steps describe how Trident can resume functioning during a catastrophe from the secondary site (SnapMirror destination) using the SnapMirror SVM replication. 

1. In the event of the source SVM failure, activate the SnapMirror destination SVM. Activating the destination SVM involves stopping scheduled SnapMirror transfers, aborting ongoing SnapMirror transfers, breaking the replication relationship, stopping the source SVM, and starting the destination SVM.

2. Uninstall Trident from the Kubernetes cluster using the ``tridentctl uninstall -n <namespace>`` command. Don't use the ``-a`` flag during the uninstall.

3. Before re-installing Trident, make sure to change the backend.json file to reflect the new destination SVM name.
 
4. Re-install Trident using “tridentctl install -n <namespace>” command.

5. Update all the required backends to reflect the new destination SVM name using the "./tridentctl update backend <backend-name> -f <backend-json-file> -n <namespace>" command.

6. All the volumes provisioned by Trident will start serving data as soon as the destination SVM is activated. 

ONTAP SnapMirror Volume Replication 
-----------------------------------

ONTAP SnapMirror Volume Replication  is a disaster recovery feature which enables failover to destination storage from primary storage on a volume level. SnapMirror creates a volume replica or mirror of the primary storage on to the secondary storage by syncing snapshots.

ONTAP SnapMirror Volume Replication Setup
-----------------------------------------

* The clusters in which the volumes reside and the SVMs that serve data from the volumes must be peered.

* Create a SnapMirror policy which controls the behavior of the relationship and specifies the configuration attributes for that relationship. 

* Create a SnapMirror relationship between the destination volume and the source volume using the "snapmirror create" volume and assign the appropriate SnapMirror policy. 

* After the SnapMirror relationship is created, initialize the relationship so that a baseline transfer from the source volume to the destination volume will be completed.

.. _figSM1:

.. figure:: images/SM1.PNG
     :align: center
     :figclass: align-center

SnapMirror Volume Replication Setup

SnapMirror Volume Disaster Recovery Workflow for Trident
--------------------------------------------------------

Disaster recovery using SnapMirror Volume Replication is not as seamless as the SnapMirror SVM Replication.

1. In the event of a disaster, stop all the scheduled SnapMirror transfers and abort all ongoing SnapMirror transfers. Break the replication relationship between the destination and source Trident etcd volume so that the destination volume becomes Read/Write.

2. Uninstall Trident from the Kubernetes cluster using the "tridentctl uninstall -n <namespace>" command. Don't use the ``-a`` flag during the uninstall.

3. Create a new backend.json file with the new IP and new SVM name of the destination SVM where the Trident etcd volume resides.

4. Re-install Trident using “tridentctl install -n <namespace>” command along with the --volume-name option.

5. Create new backends on Trident, specify the new IP and the new SVM name of the destination SVM.

6. Clean up the deployments which were consuming PVC bound to volumes on the source SVM.

7. Now import the required volumes as a PV bound to a new PVC using the Trident import feature. 

8. Re-deploy the application deployments with the newly created PVCs.



         
