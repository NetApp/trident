.. _backup_disaster_recovery:

****************************
Backup and Disaster Recovery
****************************

Protecting application data is one of the fundamental purposes of any storage system. Regardless of an application being cloud native, 12 factor, microservice or any other architecture, the data still needs to be protected for the application to continue to function and be valuable to the organization.

NetApp's storage platforms provide data protection and recoverability options which vary based on recovery time and acceptable data loss requirements. Trident can provision volumes which can take advantage of some of these features, however a full data protection and recovery strategy should be evaluated for each application with a persistence requirement.

ONTAP snapshots
===============

Snapshots play an important role by providing point-in-time recovery options for application data. It's important to remember, however, that snapshots are not backups by themselves. They will not protect against storage system failure or other catastrophes which result in the storage system failing. Snapshots are a convenient, quick, and easy way to recover data for most scenarios.

Using ONTAP snapshots with containers
-------------------------------------

A backend which has not explicitly set a snapshot policy will use the "``none``" policy.  This means that ONTAP will not take any snapshots of the volume automatically. If the storage administrator takes manual snapshots or changes the snapshot policy via the ONTAP management interface, this will not affect Trident operation.

Note that the snapshot directory is hidden by default. This is to facilitate maximum compatibility of volumes provisioned using the ontap-nas and ontap-nas-economy drivers as some applications, such as MySQL, could experience issues.

Accessing the snapshot directory
--------------------------------

The ``.snapshot`` directory is a mechanism which can be enabled when using the ``ontap-nas`` and ``ontap-nas-economy`` drivers to allow applications to recover data from snapshots directly. More information about how to enable access and enable self-service data recovery can be found in `this blog post <https://netapp.io/2018/04/03/self-service-data-recovery-using-trident-nfs/>`_ on `netapp.io <https://netapp.io/>`_.

Restoring the snapshots
-----------------------

It is possible to restore a volume to a state recorded in a previously created snapshot copy to retrieve lost information using the “volume snapshot restore” ONTAP CLI command . When you restore a Snapshot copy, the restore operation overwrites the existing volume configuration. Any changes made to the data in the volume after the Snapshot copy was created are lost.

.. code-block:: console

   cluster1::*> volume snapshot restore -vserver vs0 -volume vol3 -snapshot vol3_snap_archive



SolidFire snapshots
===================

It is possible to backup data on a SolidFire Volume by setting a snapshot schedule to a SolidFire volume. This would make sure that the snapshots of the volume are taken at the required interval. However, it is not possible to set a snapshot schedule to a volume through the solidfire-san driver. This would have to be set manually using the Element OS Web UI or Element OS APIs.

In the event of a data corruption, we can choose a particular snapshot and rollback the volume to the snapshot manually using the Element OS Web UI or Element OS APIs. This reverts any changes made to the volume since the snapshot was created.



Etcd snapshots using ``etcdctl`` command line utility
=====================================================


The etcdctl command line utility offers the provision to take snapshot of an etcd cluster. It also enables us to restore the previously taken snapshot.

etcdctl snapshot backup
-----------------------

The etcdctl command ``etcdctl snapshot save /var/etcd/data/snapshot.db`` enables us to take a point-in-time snapshot of the etcd cluster. NetApp recommends using a script to take timely backups. This command can be deployed from within the etcd container or the command can be deployed using the ``kubectl exec`` command directly. Store the periodic snapshots under the persistent Trident NetApp volume `/var/etcd/data` so that snapshots are stored securely and can safely be recovered should the trident pod be lost. Periodically check the volume to be sure it does not run out of space.

etcdctl snapshot restore
------------------------

In the event of an accidental deletion or corruption of Trident etcd data, we can choose the appropriate snapshot and restore it back using the command ``etcdctl snapshot restore snapshot.db --data-dir /var/etcd/data/etcd-test2 --name etcd1``.  Take note to restore the snapshot on to a different folder [shown in the above command as /var/etcd/data/etcd-test2 which is on the mount] inside the Trident NetApp volume.

After the restore is complete, uninstall Trident. Take note not to use the "-a" flag during uninstallation. Mount the Trident volume manually on the host and make sure that the current "member" folder under /var/etcd/data is deleted. Copy the "member" folder from the restored folder /var/etcd/data/etcd-test2 to /var/etcd/data. After the copy is complete, re-install Trident. Verify if the restore and recovery has been completed successfully by making sure all the required data is present.

Data replication using ONTAP
============================

Replicating data can play an important role in protecting against data loss due to storage array failure. Snapshots are a point-in-time recovery which provide a very quick and easy method of recovering data which has been corrupted or accidentally lost as a result of human or technological error. However, they cannot protect against catastrophic failure of the storage array itself.
Trident is unable to configure replication relationships itself, however the storage administrator can use ONTAP’s SVM-DR function to automatically replicate volumes to a DR destination. If this method is used to automatically protect Trident provisioned volumes, there are some considerations to take into account.

*	A distinct backend should be created for each SVM which has SVM-DR enabled.

*	Storage classes should be crafted so as to not select the replicated backends except when desired. This is important to avoid having volumes which do not need the protection of a replication relationship be provisioned onto the backend.

*	Application administrators should understand the additional cost and complexity associated with replicating the data and a plan for recovery should be determined before they leverage replication.

*       Trident does not automatically detect SVM failures. Therefore, upon a failure, the administrator needs to run the command `tridentctl backend update` to trigger Trident's failover to the new backend. 
         
