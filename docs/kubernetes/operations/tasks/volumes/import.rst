##################
Importing a volume
##################

Beginning with the 19.04 release of Trident, you can import existing storage
volumes as a Kubernetes PV using ``tridentctl import``. This section explains
why importing volumes is an important requirement for Kubernetes environments,
provides information about the drivers that support volume imports, and shows examples of how it's done.

Volume Import Driver Support
----------------------------

This table depicts the drivers that support importing volumes and the release
they were introduced in.

.. table:: Trident drivers that support volume import
   :align: center

   +---------------------------+--------------+
   | Driver                    | Release      |
   +===========================+==============+
   | ``ontap-nas``             | 19.04        |
   +---------------------------+--------------+
   | ``ontap-nas-flexgroup``   | 19.04        |
   +---------------------------+--------------+
   | ``solidfire-san``         | 19.04        |
   +---------------------------+--------------+
   | ``aws-cvs``               | 19.04        |
   +---------------------------+--------------+
   | ``azure-netapp-files``    | 19.07        |
   +---------------------------+--------------+
   | ``gcp-cvs``               | 19.10        |
   +---------------------------+--------------+
   | ``ontap-san``             | 20.07        |
   +---------------------------+--------------+


Why Volume Import
-----------------

There are several use cases for importing a volume into Trident:

         * Containerizing an application and reusing its existing data set
         * Using a clone of a data set for an ephemeral application
         * Rebuilding a failed Kubernetes cluster
         * Migrating application data during disaster recovery

How do I import volumes?
------------------------

The ``tridentctl`` client is used to import an existing storage volume. Trident
imports the volume by persisting volume metadata and creating the PVC and PV.

.. code-block:: bash

  $ tridentctl import volume <backendName> <volumeName> -f <path-to-pvc-file>

To import an existing storage volume, specify the name of the Trident backend
containing the volume, as well as the name that uniquely identifies the volume
on the storage (i.e. ONTAP FlexVol, Element Volume, CVS Volume path'). The storage
volume must allow read/write access and be accessible by the specified Trident backend.

The ``-f string`` argument is required and specifies the path to the YAML or JSON PVC
file. The PVC file is used by the volume import process to create the PVC. At a
minimum, the PVC file must include the name, namespace, accessModes, and
storageClassName fields as shown in the following example.

.. code-block:: yaml

  kind: PersistentVolumeClaim
  apiVersion: v1
  metadata:
    name: my_claim
    namespace: my_namespace
  spec:
    accessModes:
      - ReadWriteOnce
    storageClassName: my_storage_class

When Trident receives the import volume request the existing volume size is
determined and set in the PVC. Once the volume is imported by the storage
driver the PV is created with a ClaimRef to the PVC. The reclaim policy is initially
set to ``retain`` in the PV. Once Kubernetes successfully binds the PVC and PV the
reclaim policy is updated to match the reclaim policy of the Storage Class. If the
reclaim policy of the Storage Class is ``delete`` then the storage volume will be
deleted when the PV is deleted.

When a volume is imported with the ``--no-manage`` argument, Trident will not
perform any additional operations on the PVC or PV for the lifecycle of the
objects. Since Trident ignores PV and PVC events for ``--no-manage`` objects
the storage volume is not deleted when the PV is deleted. Other operations such as
volume clone and volume resize are also ignored. This option is provided for
those that want to use Kubernetes for containerized workloads but otherwise
want to manage the lifecycle of the storage volume outside of Kubernetes.

An annotation is added to the PVC and PV that serves a dual purpose of
indicating that the volume was imported and if the PVC and PV are managed.
This annotation should not be modified or removed.

Trident ``19.07`` and later handle the attachment of PVs and mounts the volume as
part of importing it. For imports using earlier versions of Trident,
there will not be any operations in the data path and the volume import will
not verify if the volume can be mounted. If a mistake is made with volume
import (e.g. the StorageClass is incorrect), you can recover by changing the
reclaim policy on the PV to "Retain", deleting the PVC and PV, and retrying
the volume import command.

``ontap-nas`` and ``ontap-nas-flexgroup`` imports
-------------------------------------------------

Each volume created with the ``ontap-nas`` driver is a FlexVol on the ONTAP
cluster. Importing FlexVols with the ``ontap-nas`` driver works the same.
A FlexVol that already exists on an ONTAP cluster can be imported as a
``ontap-nas`` PVC. Similarly, FlexGroup vols can be imported as
``ontap-nas-flexgroup`` PVCs.

.. important::

    The ``ontap-nas`` driver cannot import and manage qtrees. The ``ontap-nas``
    and ``ontap-nas-flexgroup`` drivers do not allow duplicate volume names.

For example, to import a volume named ``managed_volume`` on a backend named
``ontap_nas`` use the following command:

.. code-block:: bash

   $ tridentctl import volume ontap_nas managed_volume -f <path-to-pvc-file>

   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-bf5ad463-afbb-11e9-8d9f-5254004dfdb7 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

To import a volume named ``unmanaged_volume`` (on the ``ontap_nas`` backend)
which Trident will not manage, use the following command:

.. code-block:: bash

   $ tridentctl import volume nas_blog unmanaged_volume -f <path-to-pvc-file> --no-manage

   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-df07d542-afbc-11e9-8d9f-5254004dfdb7 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | false   |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

When using the ``--no-manage`` argument, Trident does not rename the volume or validate if the volume was mounted. The volume import operation will fail if the volume was not mounted manually.

.. note::

   A previously existing bug with importing volumes with custom UnixPermissions
   has been fixed. Users can specify ``unixPermissions`` in their PVC definition
   or backend config and instruct Trident to import the volume accordingly.

``ontap-san`` import
--------------------

Trident can also import ONTAP SAN FlexVols that contain a single LUN. This is
consistent with the ``ontap-san`` driver, which creates a FlexVol for each PVC
and a LUN within the FlexVol. The invocation of the ``tridentctl import`` command
is the same as in other cases:

1. Include the name of the ``ontap-san`` backend.
2. Provide the name of the FlexVol that needs to be imported. Remember, this
   FlexVol contains only one LUN that must be imported.
3. Provide the path of the PVC definition that must be used with the ``-f`` flag.
4. Choose between having the PVC managed or unmanaged. By default Trident will
   manage the PVC and rename the FlexVol and LUN on the backend. To import as an
   unmanaged volume pass the ``--no-manage`` flag.

.. warning::

   When importing an unmanaged ``ontap-san`` volume you must make sure that the
   LUN in the FlexVol is named ``lun0`` and is mapped to an igroup with the
   desired initiators. Trident will automatically handle this for a managed
   import.

Trident will then import the FlexVol and associate it with the PVC definition as
well as:

1. renaming the FlexVol to the ``pvc-<uuid>`` format. The FlexVol will be renamed
   on the ONTAP cluster as ``pvc-d6ee4f54-4e40-4454-92fd-d00fc228d74a`` for example.
2. renaming the LUN within the FlexVol to ``lun0``.

Users are advised to import volumes that do not have active connections existing.
If you are looking to import an actively used volume, it is recommended to clone
the volume first and then import.

Here's an example. To import the ``ontap-san-managed`` FlexVol that is present on
the ``ontap_san_default`` backend, you will need to execute the ``tridentctl
import`` command as:

.. code-block:: bash

   $ tridentctl import volume ontapsan_san_default ontap-san-managed -f pvc-basic-import.yaml -n trident -d

   +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE  | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-d6ee4f54-4e40-4454-92fd-d00fc228d74a | 20 MiB | basic         | block    | cd394786-ddd5-4470-adc3-10c5ce4ca757 | online | true    |
   +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+

``element`` import
------------------

You can import Element/HCI volumes to your Kubernetes cluster with Trident. You
will need the name of your Trident backend, the unique name of the volume and the
PVC file as the arguments for the ``tridentctl import`` call.

.. code-block:: bash

   $ tridentctl import volume element_default element-managed -f pvc-basic-import.yaml -n trident -d

   +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE  | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-970ce1ca-2096-4ecd-8545-ac7edc24a8fe | 10 GiB | basic-element | block    | d3ba047a-ea0b-43f9-9c42-e38e58301c49 | online | true    |
   +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+

.. note::

   The Element driver supports duplicate volume names. If there are duplicate
   volume names Trident's volume import process
   will return an error. As a workaround, clone the volume and provide a
   unique volume name. Then import the cloned volume.

``aws-cvs`` import
------------------

To import an ``aws-cvs`` volume on the backend called ``awscvs_YEppr`` with
the volume path of ``adroit-jolly-swift`` use the following command:

.. code-block:: bash

    $ tridentctl import volume awscvs_YEppr adroit-jolly-swift -f <path-to-pvc-file> -n trident

    +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+
    |                   NAME                   |  SIZE  | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
    +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+
    | pvc-a46ccab7-44aa-4433-94b1-e47fc8c0fa55 | 93 GiB | aws-storage   | file     | e1a6e65b-299e-4568-ad05-4f0a105c888f | online | true    |
    +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+

.. note::
  The volume path is the portion of the volume's export path after the `:/`. For example, if the export path is
  ``10.0.0.1:/adroit-jolly-swift`` then the volume path is ``adroit-jolly-swift``.

``gcp-cvs`` import
------------------

Importing a ``gcp-cvs`` volume works the same as importing an ``aws-cvs`` volume.

``azure-netapp-files`` import
-----------------------------

To import an ``azure-netapp-files`` volume on the backend called
``azurenetappfiles_40517`` with the volume path ``importvol1``, you will use
the following command:

.. code-block:: bash

   $ tridentctl import volume azurenetappfiles_40517 importvol1 -f <path-to-pvc-file> -n trident

   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-0ee95d60-fd5c-448d-b505-b72901b3a4ab | 100 GiB | anf-storage   | file     | 1c01274f-d94b-44a3-98a3-04c953c9a51e | online | true    |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

.. note::
   The volume path for the ANF volume is present in the mount path after the `:/`. For example, if the mount path is
   ``10.0.0.2:/importvol1``, the volume path is ``importvol1``.

Behavior of Drivers for Volume Import
-------------------------------------

  * The ``ontap-nas`` and ``ontap-nas-flexgroup`` drivers do not allow
    duplicate volume names.
  * To import a volume backed by the NetApp Cloud Volumes Service in AWS,
    identify the volume by its volume path instead of its name. An example
    is provided in the previous section.
  * An ONTAP volume must be of type `rw` to be imported by Trident. If a
    volume is of type `dp` it is a SnapMirror destination volume; you must
    break the mirror relationship before importing the volume into Trident.
