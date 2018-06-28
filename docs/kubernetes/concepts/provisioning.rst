###########################
How does provisioning work?
###########################

Provisioning in Trident has two primary phases.  The first of these associates
a storage class with the set of suitable backend storage pools and occurs
as a necessary preparation before provisioning.  The second encompasses the
volume creation itself and requires choosing a storage pool from those
associated with the pending volume's storage class.  This section explains both
of these phases and the considerations involved in them, so that users can
better understand how Trident handles their storage.

Associating backend storage pools with a storage class relies on both the
storage class's requested attributes and its ``storagePools``, 
``additionalStoragePools``, and ``excludeStoragePools`` lists.  When a user 
creates a storage class, Trident compares the attributes and pools offered by 
each of its backends to those requested by the storage class.  If a storage pool's 
attributes and name match all of the requested attributes and pool names, Trident 
adds that storage pool to the set of suitable storage pools for that storage class.  
In addition, Trident adds all storage pools listed in the ``additionalStoragePools`` 
list to that set, even if their attributes do not fulfill all or any of the storage
class's requested attributes.  Use the ``excludeStoragePools`` list to override and 
remove storage pools from use for a storage class. Trident performs a similar process
every time a user adds a new backend, checking whether its storage pools satisfy 
those of the existing storage classes and removing any that have been marked as excluded.

Trident then uses the associations between storage classes and storage pools to
determine where to provision volumes.  When a user creates a volume, Trident
first gets the set of storage pools for that volume's storage class, and, if
the user specifies a protocol for the volume, it removes those storage pools
that cannot provide the requested protocol (a SolidFire backend cannot provide
a file-based volume while an ONTAP NAS backend cannot provide a block-based
volume, for instance).  Trident randomizes the order of this resulting set, to
facilitate an even distribution of volumes, and then iterates through it,
attempting to provision the volume on each storage pool in turn.  If it
succeeds on one, it returns successfully, logging any failures encountered in
the process.  Trident returns a failure if and only if it fails to provision on
**all** the storage pools available for the requested storage class and protocol.
