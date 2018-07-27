Managing volumes
################

Creating and consuming storage from ONTAP, SolidFire, and/or E-Series systems is easy with Trident.  Simply use the standard ``docker volume`` commands with the nDVP driver name specified when needed.

Create a Volume
---------------

.. code-block:: bash

   # create a volume with a Trident driver using the default name
   docker volume create -d netapp --name firstVolume

   # create a volume with a specific Trident instance
   docker volume create -d ntap_bronze --name bronzeVolume

If no options are specified, the defaults for the driver are used.  The defaults are documented on the page for the storage driver you're using below.

The default volume size may be overridden per volume as follows:

.. code-block:: bash

   # create a 20GiB volume with a Trident driver
   docker volume create -d netapp --name my_vol --opt size=20G

Volume sizes are expressed as strings containing an integer value with optional units (e.g. "10G", "20GB", "3TiB").
If no units are specified, the default is 'G'.  Size units may be expressed either as powers of 2 (B, KiB, MiB, GiB, TiB)
or powers of 10 (B, KB, MB, GB, TB).  Shorthand units use powers of 2 (G = GiB, T = TiB, ...).

The snapshot reserve will be changed to 0% if you set the snapshot policy to none as follows:

.. code-block:: bash
    # create a volume with no snapshot policy and no snapshot reserve
    docker volume create -d netapp --name my_vol --opt snapshotPolicy=none


Volume Driver CLI Options
-------------------------

Each storage driver has a different set of options which can be provided at volume creation time to customize the outcome.  Refer to the documentation below for your configured storage system to determine which options apply.

.. toctree::
   :maxdepth: 1

   backends/ontap_options
   backends/solidfire_options
   backends/eseries_options

Destroy a Volume
----------------

.. code-block:: bash

   # destroy the volume just like any other Docker volume
   docker volume rm firstVolume

Volume Cloning
--------------

When using the ontap-nas, ontap-san, and solidfire-san storage drivers, the Docker Volume Plugin can clone volumes.
When using the ontap-nas-flexgroup or ontap-nas-economy drivers, cloning is not supported.

.. code-block:: bash

   # inspect the volume to enumerate snapshots
   docker volume inspect <volume_name>

   # create a new volume from an existing volume.  this will result in a new snapshot being created
   docker volume create -d <driver_name> --name <new_name> -o from=<source_docker_volume>

   # create a new volume from an existing snapshot on a volume.  this will not create a new snapshot
   docker volume create -d <driver_name> --name <new_name> -o from=<source_docker_volume> -o fromSnapshot=<source_snap_name>

Here is an example of that in action:

.. code-block:: bash

   [me@host ~]$ docker volume inspect firstVolume

   [
       {
           "Driver": "ontap-nas",
           "Labels": null,
           "Mountpoint": "/var/lib/docker-volumes/ontap-nas/netappdvp_firstVolume",
           "Name": "firstVolume",
           "Options": {},
           "Scope": "global",
           "Status": {
               "Snapshots": [
                   {
                       "Created": "2017-02-10T19:05:00Z",
                       "Name": "hourly.2017-02-10_1505"
                   }
               ]
           }
       }
   ]

   [me@host ~]$ docker volume create -d ontap-nas --name clonedVolume -o from=firstVolume
   clonedVolume

   [me@host ~]$ docker volume rm clonedVolume
   [me@host ~]$ docker volume create -d ontap-nas --name volFromSnap -o from=firstVolume -o fromSnapshot=hourly.2017-02-10_1505
   volFromSnap

   [me@host ~]$ docker volume rm volFromSnap

Access Externally Created Volumes
---------------------------------

Externally created block devices (or their clones) may be accessed by containers using Trident only if they have no partitions and if their filesystem is supported by nDVP (example: an ext4-formatted /dev/sdc1 will not be accessible via nDVP).
