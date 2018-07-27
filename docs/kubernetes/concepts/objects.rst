##############################
Kubernetes and Trident objects
##############################

Both Kubernetes and Trident are designed to be interacted with through REST
APIs by reading and writing resource objects.

Object overview
---------------

There are several different resource objects in play here, some that are
managed through Kubernetes and others that are managed through Trident, that
dictate the relationship between Kubernetes and Trident, Trident and storage,
and Kubernetes and storage.

Perhaps the easiest way to understand these objects, what they are for and how
they interact, is to follow a single request for storage from a Kubernetes user:

#. A user creates a
   :ref:`PersistentVolumeClaim <Kubernetes PersistentVolumeClaim objects>`
   requesting a new :ref:`PersistentVolume <Kubernetes PersistentVolume objects>`
   of a particular size from a
   :ref:`Kubernetes StorageClass <Kubernetes StorageClass objects>`
   that was previously configured by the administrator.
#. The :ref:`Kubernetes StorageClass <Kubernetes StorageClass objects>`
   identifies Trident as its provisioner and includes parameters that tell
   Trident how to provision a volume for the requested class.
#. Trident looks at its own
   :ref:`Trident StorageClass <Trident StorageClass objects>` with the same
   name that identifies the matching
   :ref:`Backends <Trident Backend objects>` and
   :ref:`StoragePools <Trident StoragePool objects>` that it can use to
   provision volumes for the class.
#. Trident provisions storage on a matching backend and creates two objects: a
   :ref:`PersistentVolume <Kubernetes PersistentVolume objects>` in Kubernetes
   that tells Kubernetes how to find, mount and treat the volume, and a
   :ref:`Volume <Trident Volume objects>` in Trident that retains the
   relationship between the
   :ref:`PersistentVolume <Kubernetes PersistentVolume objects>` and the
   actual storage.
#. Kubernetes binds the
   :ref:`PersistentVolumeClaim <Kubernetes PersistentVolumeClaim objects>` to
   the new :ref:`PersistentVolume <Kubernetes PersistentVolume objects>`. Pods
   that include the
   :ref:`PersistentVolumeClaim <Kubernetes PersistentVolumeClaim objects>` will
   mount that :ref:`PersistentVolume <Kubernetes PersistentVolume objects>` on
   any host that it runs on.

Throughout the rest of this guide, we will describe the different Trident
objects and details about how Trident crafts the Kubernetes PersistentVolume
object for storage that it provisions.

For further reading about the Kubernetes objects, we highly recommend that you
read the `Persistent Volumes`_ section of the Kubernetes documentation.

Kubernetes PersistentVolumeClaim objects
----------------------------------------

A `Kubernetes PersistentVolumeClaim`_ object is a request for storage made by a
Kubernetes cluster user.

In addition to the `standard specification`_, Trident allows users to specify
the following volume-specific annotations if they want to override the
defaults that you set in the backend configuration:

=================================== ================= ======================================================
Annotation                          Volume Option     Supported Drivers
=================================== ================= ======================================================
trident.netapp.io/fileSystem        fileSystem        ontap-san, solidfire-san, eseries-iscsi
trident.netapp.io/reclaimPolicy     N/A               any
trident.netapp.io/cloneFromPVC      cloneSourceVolume ontap-nas, ontap-san, solidfire-san
trident.netapp.io/splitOnClone      splitOnClone      ontap-nas, ontap-san
trident.netapp.io/protocol          protocol          any
trident.netapp.io/exportPolicy      exportPolicy      ontap-nas, ontap-nas-economy, ontap-nas-flexgroup
trident.netapp.io/snapshotPolicy    snapshotPolicy    ontap-nas, ontap-nas-economy, ontap-nas-flexgroup, ontap-san
trident.netapp.io/snapshotDirectory snapshotDirectory ontap-nas, ontap-nas-economy, ontap-nas-flexgroup
trident.netapp.io/unixPermissions   unixPermissions   ontap-nas, ontap-nas-economy, ontap-nas-flexgroup
trident.netapp.io/blockSize         blockSize         solidfire-san
=================================== ================= ======================================================

The reclaim policy for the created PV can be determined by setting the
annotation ``trident.netapp.io/reclaimPolicy`` in the PVC to either ``Delete``
or ``Retain``; this value will then be set in the PV's ``ReclaimPolicy`` field.
When the annotation is left unspecified, Trident will use the ``Delete`` policy.
If the created PV has the ``Delete`` reclaim policy, Trident will delete both
the PV and the backing volume when the PV becomes released (i.e., when the user
deletes the PVC).  Should the delete action fail, Trident will mark the PV
as such and periodically retry the operation until it succeeds or the PV is
manually deleted.  If the PV uses the ``Retain`` policy, Trident ignores it and
assumes the administrator will clean it up from Kubernetes and the backend,
allowing the volume to be backed up or inspected before its removal.  Note that
deleting the PV will not cause Trident to delete the backing volume; it must be
removed manually via the REST API (i.e., ``tridentctl``).

One novel aspect of Trident is that users can provision new volumes by cloning
existing volumes. Trident enables this functionality via the PVC annotation
``trident.netapp.io/cloneFromPVC``. For example, if a user already has a PVC
called ``mysql``, she can create a new PVC called ``mysqlclone`` by referring
to the ``mysql`` PVC: ``trident.netapp.io/cloneFromPVC: mysql``. With this
annotation set, Trident clones the volume corresponding to the ``mysql`` PVC,
instead of provisioning a volume from scratch. A few points worth considering
are the following: (1) We recommend cloning an idle volume, (2) a PVC and its
clone must be in the same Kubernetes namespace and have the same storage class,
and (3) with ``ontap-\*`` drivers, it might be desirable to set the PVC
annotation ``trident.netapp.io/splitOnClone`` in conjunction with
``trident.netapp.io/cloneFromPVC``. With ``trident.netapp.io/splitOnClone`` set
to ``true``, Trident splits the cloned volume from the parent volume; thus,
completely decoupling the life cycle of the cloned volume from its parent at
the expense of losing some storage efficiency. Not setting
``trident.netapp.io/splitOnClone`` or setting it to ``false`` results in
reduced space consumption on the backend at the expense of creating
dependencies between the parent and clone volumes such that the parent volume
cannot be deleted unless the clone is deleted first. A scenario where splitting
the clone makes sense is cloning an empty database volume where it's expected
for the volume and its clone to greatly diverge and not benefit from storage
efficiencies offered by ONTAP.

``sample-input/pvc-basic.yaml``, ``sample-input/pvc-basic-clone.yaml``, and
``sample-input/pvc-full.yaml`` contain examples of PVC definitions for use with
Trident.  See :ref:`Trident Volume objects` for a full description of the
parameters and settings associated with Trident volumes.

Kubernetes PersistentVolume objects
-----------------------------------

A `Kubernetes PersistentVolume`_ object represents a piece of storage that's
been made available to the Kubernetes cluster. They have a lifecycle that's
independent of the pod that uses it.

.. note::
  Trident creates PersistentVolume objects and registers them with the
  Kubernetes cluster automatically based on the volumes that it provisions.
  You are not expected to manage them yourself.

When a user creates a PVC that refers to a Trident-based ``StorageClass``,
Trident will provision a new volume using the corresponding storage class and
register a new PV for that volume.  In configuring the provisioned volume and
corresponding PV, Trident follows the following rules:

* Trident generates a PV name for Kubernetes and an internal name that it uses
  to provision the storage. In both cases it is assuring that the names are
  unique in their scope.
* The size of the volume matches the requested size in the PVC as closely as
  possible, though it may be rounded up to the nearest allocatable quantity,
  depending on the platform.

Kubernetes StorageClass objects
-------------------------------

`Kubernetes StorageClass objects`_ are specified by name in
PersistentVolumeClaims to provision storage with a set of properties. The
storage class itself identifies the provisioner that will be used and defines
that set of properties in terms the provisioner understands.

It is one of two objects that need to be created and managed by you, the
administrator. The other is the
:ref:`Trident Backend object <Trident Backend objects>`.

A Kubernetes StorageClass object that uses Trident looks like this:

.. code-block:: yaml

  apiVersion: storage.k8s.io/v1beta1
  kind: StorageClass
  metadata:
    name: <Name>
  provisioner: netapp.io/trident
  parameters:
    <Trident Parameters>

These parameters are Trident-specific and tell Trident how to provision volumes
for the class.

The storage class parameters are:

======================= ===================== ======== =====================================================
Attribute               Type                  Required Description
======================= ===================== ======== =====================================================
attributes              map[string]string     no       See the attributes section below
storagePools            map[string]StringList no       Map of backend names to lists of storage pools within
additionalStoragePools  map[string]StringList no       Map of backend names to lists of storage pools within
excludeStoragePools     map[string]StringList no       Map of backend names to lists of storage pools within
======================= ===================== ======== =====================================================

Storage attributes and their possible values can be classified into two groups:

1. Storage pool selection attributes: These parameters determine which
   Trident-managed storage pools should be utilized to provision volumes of a
   given type.

================= ====== ======================================= ========================================================== ============================== =============================================================
Attribute         Type   Values                                  Offer                                                      Request                        Supported by
================= ====== ======================================= ========================================================== ============================== =============================================================
media             string hdd, hybrid, ssd                        Pool contains media of this type; hybrid means both        Media type specified           All drivers
provisioningType  string thin, thick                             Pool supports this provisioning method                     Provisioning method specified  thick: all but solidfire-san, thin: all but eseries-iscsi
backendType       string ontap-nas, ontap-nas-economy,           Pool belongs to this type of backend                       Backend specified              All drivers
                         ontap-nas-flexgroup, ontap-san,
                         solidfire-san, eseries-iscsi
snapshots         bool   true, false                             Pool supports volumes with snapshots                       Volume with snapshots enabled  ontap-nas, ontap-san, solidfire-san
clones            bool   true, false                             Pool supports cloning volumes                              Volume with clones enabled     ontap-nas, ontap-san, solidfire-san
encryption        bool   true, false                             Pool supports encrypted volumes                            Volume with encryption enabled ontap-nas, ontap-nas-economy, ontap-nas-flexgroups, ontap-san
IOPS              int    positive integer                        Pool is capable of guaranteeing IOPS in this range         Volume guaranteed these IOPS   solidfire-san
================= ====== ======================================= ========================================================== ============================== =============================================================

In most cases, the values requested will directly influence provisioning; for
instance, requesting thick provisioning will result in a thickly provisioned
volume.  However, a SolidFire storage pool will use its offered IOPS
minimum and maximum to set QoS values, rather than the requested value.  In
this case, the requested value is used only to select the storage pool.

Ideally you will be able to use ``attributes`` alone to model the qualities of
the storage you need to satisfy the needs of a particular class. Trident will
automatically discover and select storage pools that match *all* of the
``attributes`` that you specify.

If you find yourself unable to use ``attributes`` to automatically select the
right pools for a class, you can use the ``storagePools`` and
``additionalStoragePools`` parameters to further refine the pools or even
to select a specific set of pools manually.

The ``storagePools`` parameter is used to further restrict the set of pools
that match any specified ``attributes``.  In other words, Trident will use the
intersection of pools identified by the ``attributes`` and ``storagePools``
parameters for provisioning.  You can use either parameter alone or both
together.

The ``additionalStoragePools`` parameter is used to extend the set of pools
that Trident will use for provisioning, regardless of any pools selected by
the ``attributes`` and ``storagePools`` parameters.

The ``excludeStoragePools`` parameter is used to filter the set of pools
that Trident will use for provisioning and will remove any pools that match.

In the ``storagePools`` and ``additionalStoragePools`` parameters, each entry
takes the form ``<backend>:<storagePoolList>``, where ``<storagePoolList>`` is
a comma-separated list of storage pools for the specified backend. For example,
a value for ``additionalStoragePools`` might look like
``ontapnas_192.168.1.100:aggr1,aggr2;solidfire_192.168.1.101:bronze``. These lists
will accept regex values for both the backend and list values. You can
use ``tridentctl get backend`` to get the list of backends and their pools.

2. Kubernetes attributes: These attributes have no impact on the selection of
   storage pools/backends by Trident during dynamic provisioning. Instead,
   these attributes simply supply parameters supported by Kubernetes Persistent
   Volumes.

================= ======= ======================================= ================================================= ======================================================= ===================
Attribute         Type    Values                                  Description                                       Relevant Drivers                                        Kubernetes Version
================= ======= ======================================= ================================================= ======================================================= ===================
fsType            string  ext4, ext3, xfs, etc.                   The file system type for block volumes            solidfire-san, ontap-san, eseries-iscsi                 All
================= ======= ======================================= ================================================= ======================================================= ===================

The Trident installer bundle provides several example storage class definitions
for use with Trident in ``sample-input/storage-class-*.yaml``. Deleting a
Kubernetes storage class will cause the corresponding Trident storage class
to be deleted as well.

Trident StorageClass objects
----------------------------

.. note::
  With Kubernetes, these objects are created automatically when a Kubernetes
  StorageClass that uses Trident as a provisioner is registered.

Trident creates matching storage classes for Kubernetes ``StorageClass``
objects that specify ``netapp.io/trident`` in their provisioner field. The
storage class's name will match that of the Kubernetes ``StorageClass`` object
it represents.

Storage classes comprise a set of requirements for volumes. Trident matches
these requirements with the attributes present in each storage pool; if they
match, that storage pool is a valid target for provisioning volumes using that
storage class.

One can create storage class configurations to directly define storage classes
via the :ref:`REST API`. However, for Kubernetes deployments, we expect them to
be created as a side-effect of registering new
:ref:`Kubernetes StorageClass objects`.

Trident Backend objects
-----------------------

Backends represent the storage providers on top of which Trident provisions
volumes; a single Trident instance can manage any number of backends.

This is one of the two object types that you will need to create and manage
yourself. The other is the
:ref:`Kubernetes StorageClass object <Kubernetes StorageClass objects>` below.

For more information about how to construct these objects, visit the
:ref:`backend configuration <Backend configuration>` guide.

Trident StoragePool objects
---------------------------

Storage pools represent the distinct locations available for provisioning on
each backend. For ONTAP, these correspond to aggregates in SVMs; for
SolidFire, these correspond to admin-specified QoS bands. Each storage pool
has a set of distinct storage attributes, which define its performance
characteristics and data protection characteristics.

Unlike the other objects here, storage pool candidates are always discovered and
managed automatically. :ref:`View your backends <Managing backends>` to see the
storage pools associated with them.

Trident Volume objects
----------------------

.. note::
  With Kubernetes, these objects are managed automatically and should not be
  manipulated by hand. You can view them to see what Trident provisioned,
  however.

Volumes are the basic unit of provisioning, comprising backend endpoints such
as NFS shares and iSCSI LUNs. In Kubernetes, these correspond directly to
PersistentVolumes. Each volume must be created with a storage class, which
determines where that volume can be provisioned, along with a size.

A volume configuration defines the properties that a provisioned volume should
have.

================= ====== ======== ================================================================
Attribute         Type   Required Description
================= ====== ======== ================================================================
version           string no       Version of the Trident API ("1")
name              string yes      Name of volume to create
storageClass      string yes      Storage class to use when provisioning the volume
size              string yes      Size of the volume to provision in bytes
protocol          string no       Protocol type to use; "file" or "block"
internalName      string no       Name of the object on the storage system; generated by Trident
snapshotPolicy    string no       ontap-\*: Snapshot policy to use
exportPolicy      string no       ontap-nas\*: Export policy to use
snapshotDirectory bool   no       ontap-nas\*: Whether the snapshot directory is visible
unixPermissions   string no       ontap-nas\*: Initial UNIX permissions
blockSize         string no       solidfire-\*: Block/sector size
fileSystem        string no       File system type
cloneSourceVolume string no       ontap-{nas|san} & solidfire-\*: Name of the volume to clone from
splitOnClone      string no       ontap-{nas|san}: Split the clone from its parent
================= ====== ======== ================================================================

As mentioned, Trident generates ``internalName`` when creating the volume. This
consists of two steps.  First, it prepends the storage prefix -- either the
default, ``trident``, or the prefix in the backend configurationd -- to the
volume name, resulting in a name of the form ``<prefix>-<volume-name>``. It then
proceeds to sanitize the name, replacing characters not permitted in the
backend.  For ONTAP backends, it replaces hyphens with underscores (thus, the
internal name becomes ``<prefix>_<volume-name>``), and for SolidFire, it
replaces underscores with hyphens. For E-Series, which imposes a
30-character limit on all object names, Trident generates a random string for
the internal name of each volume on the array.

One can use volume configurations to directly provision volumes via the
:ref:`REST API`, but in Kubernetes deployments we expect most users to use the
standard `Kubernetes PersistentVolumeClaim`_ method. Trident will create this
volume object automatically as part of the provisioning process in that case.

.. _Kubernetes StorageClass: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#storageclasses
.. _Kubernetes PersistentVolume: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistent-volumes
.. _Kubernetes PersistentVolumeClaim: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims
.. _standard specification: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims
.. _Persistent Volumes: https://kubernetes.io/docs/concepts/storage/persistent-volumes/
.. _external dynamic provisioners: https://github.com/kubernetes/community/blob/master/contributors/design-proposals/volume-provisioning.md
