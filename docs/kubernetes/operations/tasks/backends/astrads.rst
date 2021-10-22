################
Astra Data Store
################

To create and use an Astra Data Store (ADS) backend, you will need:

* A supported ADS storage system
* Complete `ADS backend preparation`_
* Credentials for the Kubernetes cluster hosting ADS

.. _ADS backend preparation:

Preparation
-----------

The Kubernetes cluster hosting ADS must have a namespace dedicated to the volume, snapshot, and export policy
resources that this Trident backend will create and manage.

A kubeconfig must be available for the Kubernetes cluster hosting ADS that supports reading all objects in the
astrads-system namespace, reading/writing objects in the namespace created for this Trident backend's use, and
listing all cluster namespaces.

Backend configuration options
-----------------------------

========================= ===============================================================                         ================================================
Parameter                 Description                                                                             Default
========================= ===============================================================                         ================================================
version                   Always 1
storageDriverName         "astrads-nas"
cluster                   The name of the AstraDSCluster resource
namespace                 The namespace where Trident will create all ADS custom resources
kubeconfig                Credentials for ADS Kubernetes cluster (Base64 compact JSON)
backendName               Custom name for the storage backend                                                     ADS cluster name
nfsMountOptions           Fine-grained control of NFS mount options                                               "vers=4.1"
autoExportPolicy          Enable automatic export policy creation and updating [Boolean]                          false
autoExportCIDRs           List of CIDRs to filter Kubernetes' node IPs against when autoExportPolicy is enabled   ["0.0.0.0/0", "::/0"]
limitVolumeSize           Fail provisioning if requested volume size is above this value                          "" (not enforced by default)
debugTraceFlags           Debug flags to use when troubleshooting.  E.g.: {"api":false, "method":true}            null
labels                    Set of arbitrary JSON-formatted labels to apply on volumes.                             ""
========================= ===============================================================                         ================================================

.. warning::

  Do not use ``debugTraceFlags`` unless you are troubleshooting and require a detailed log dump.

The required value ``kubeconfig`` must be converted from YAML to compact JSON format, and then
converted to Base64 format before being included in the backend configuration.

Each backend provisions volumes in a single namespace on the Kubernetes cluster that hosts ADS. To create volumes
in other namespaces, you can define additional backends.  Note that ADS volumes may be attached to any namespace
in the hosting cluster, any other Kubernetes cluster, or anywhere else that may mount NFS shares.

You can control how each volume is provisioned by default using these options in a special section of the configuration.
For an example, see the configuration examples below.

========================= =============================================================== ================================================
Parameter                 Description                                                     Default
========================= =============================================================== ================================================
exportPolicy              Export policy to use                                            "default"
unixPermissions           Mode for new volumes, must be octal and begin with "0"          "0777"
snapshotReserve           Percentage of volume reserved for snapshots                     "5"
snapshotDir               Controls visibility of the .snapshot directory                  "false"
qosPolicy                 QoS policy to assign for volumes created.                       ""
========================= =============================================================== ================================================

.. note::

  For all volumes created on an ADS backend, Trident will copy all labels present
  on a :ref:`storage pool <ads-virtual-storage-pool>` to the storage volume at
  the time it is provisioned. Storage admins can define labels per storage pool
  and group all volumes created per storage pool. This provides a convenient way
  of differentiating volumes based on a set of customizable labels that are provided
  in the backend configuration.

Example configurations
----------------------

**Example 1 - Minimal backend configuration for astrads-nas driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "cluster": "astrads-sti-c6220-09-10-11-12",
        "namespace": "test",
        "kubeconfig": "eyJjdXJyZW50LWNvbnRleHQiOiJmZWRlcmFsLWNvbnRleHQiLCJhcGlWZXJzaW9uIjoidjEiLCJjbHVzdGVycyI6W3siY2x1c3RlciI6eyJhcGktdmVyc2lvbiI6InYxIiwic2VydmVyIjoiaHR0cDovL2Nvdy5vcmc6ODA4MCJ9LCJuYW1lIjoiY293LWNsdXN0ZXIifSx7ImNsdXN0ZXIiOnsiY2VydGlmaWNhdGUtYXV0aG9yaXR5IjoicGF0aC90by9teS9jYWZpbGUiLCJzZXJ2ZXIiOiJodHRwczovL2hvcnNlLm9yZzo0NDQzIn0sIm5hbWUiOiJob3JzZS1jbHVzdGVyIn0seyJjbHVzdGVyIjp7Imluc2VjdXJlLXNraXAtdGxzLXZlcmlmeSI6dHJ1ZSwic2VydmVyIjoiaHR0cHM6Ly9waWcub3JnOjQ0MyJ9LCJuYW1lIjoicGlnLWNsdXN0ZXIifV0sImNvbnRleHRzIjpbeyJjb250ZXh0Ijp7ImNsdXN0ZXIiOiJob3JzZS1jbHVzdGVyIiwibmFtZXNwYWNlIjoiY2hpc2VsLW5zIiwidXNlciI6ImdyZWVuLXVzZXIifSwibmFtZSI6ImZlZGVyYWwtY29udGV4dCJ9LHsiY29udGV4dCI6eyJjbHVzdGVyIjoicGlnLWNsdXN0ZXIiLCJuYW1lc3BhY2UiOiJzYXctbnMiLCJ1c2VyIjoiYmxhY2stdXNlciJ9LCJuYW1lIjoicXVlZW4tYW5uZS1jb250ZXh0In1dLCJraW5kIjoiQ29uZmlnIiwicHJlZmVyZW5jZXMiOnsiY29sb3JzIjp0cnVlfSwidXNlcnMiOlt7Im5hbWUiOiJibHVlLXVzZXIiLCJ1c2VyIjp7InRva2VuIjoiYmx1ZS10b2tlbiJ9fSx7Im5hbWUiOiJncmVlbi11c2VyIiwidXNlciI6eyJjbGllbnQtY2VydGlmaWNhdGUiOiJwYXRoL3RvL215L2NsaWVudC9jZXJ0IiwiY2xpZW50LWtleSI6InBhdGgvdG8vbXkvY2xpZW50L2tleSJ9fV19",
    }

**Example 2 -  Backend configuration for astrads-nas driver with single service level**

This example shows a backend file that applies the same aspects to all Trident created storage.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "cluster": "astrads-sti-c6220-09-10-11-12",
        "namespace": "test",
        "kubeconfig": "eyJjdXJyZW50LWNvbnRleHQiOiJmZWRlcmFsLWNvbnRleHQiLCJhcGlWZXJzaW9uIjoidjEiLCJjbHVzdGVycyI6W3siY2x1c3RlciI6eyJhcGktdmVyc2lvbiI6InYxIiwic2VydmVyIjoiaHR0cDovL2Nvdy5vcmc6ODA4MCJ9LCJuYW1lIjoiY293LWNsdXN0ZXIifSx7ImNsdXN0ZXIiOnsiY2VydGlmaWNhdGUtYXV0aG9yaXR5IjoicGF0aC90by9teS9jYWZpbGUiLCJzZXJ2ZXIiOiJodHRwczovL2hvcnNlLm9yZzo0NDQzIn0sIm5hbWUiOiJob3JzZS1jbHVzdGVyIn0seyJjbHVzdGVyIjp7Imluc2VjdXJlLXNraXAtdGxzLXZlcmlmeSI6dHJ1ZSwic2VydmVyIjoiaHR0cHM6Ly9waWcub3JnOjQ0MyJ9LCJuYW1lIjoicGlnLWNsdXN0ZXIifV0sImNvbnRleHRzIjpbeyJjb250ZXh0Ijp7ImNsdXN0ZXIiOiJob3JzZS1jbHVzdGVyIiwibmFtZXNwYWNlIjoiY2hpc2VsLW5zIiwidXNlciI6ImdyZWVuLXVzZXIifSwibmFtZSI6ImZlZGVyYWwtY29udGV4dCJ9LHsiY29udGV4dCI6eyJjbHVzdGVyIjoicGlnLWNsdXN0ZXIiLCJuYW1lc3BhY2UiOiJzYXctbnMiLCJ1c2VyIjoiYmxhY2stdXNlciJ9LCJuYW1lIjoicXVlZW4tYW5uZS1jb250ZXh0In1dLCJraW5kIjoiQ29uZmlnIiwicHJlZmVyZW5jZXMiOnsiY29sb3JzIjp0cnVlfSwidXNlcnMiOlt7Im5hbWUiOiJibHVlLXVzZXIiLCJ1c2VyIjp7InRva2VuIjoiYmx1ZS10b2tlbiJ9fSx7Im5hbWUiOiJncmVlbi11c2VyIiwidXNlciI6eyJjbGllbnQtY2VydGlmaWNhdGUiOiJwYXRoL3RvL215L2NsaWVudC9jZXJ0IiwiY2xpZW50LWtleSI6InBhdGgvdG8vbXkvY2xpZW50L2tleSJ9fV19",
        "defaults": {
            "exportPolicy": "myexportpolicy1",
            "qosPolicy": "bronze",
            "snapshotReserve": "10",
        },
        "labels": {"cloud": "on-prem", "creator": "ads-cluster-1", "performance": "bronze"},
    }

.. _ads-virtual-storage-pool:

**Example 3 - Backend and storage class configuration for astrads-nas driver with virtual storage pools**

This example shows the backend definition file configured with :ref:`Virtual Storage Pools <Virtual Storage Pools>`
along with StorageClasses that refer back to them.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "cluster": "astrads-sti-c6220-09-10-11-12",
        "namespace": "test",
        "kubeconfig": "eyJjdXJyZW50LWNvbnRleHQiOiJmZWRlcmFsLWNvbnRleHQiLCJhcGlWZXJzaW9uIjoidjEiLCJjbHVzdGVycyI6W3siY2x1c3RlciI6eyJhcGktdmVyc2lvbiI6InYxIiwic2VydmVyIjoiaHR0cDovL2Nvdy5vcmc6ODA4MCJ9LCJuYW1lIjoiY293LWNsdXN0ZXIifSx7ImNsdXN0ZXIiOnsiY2VydGlmaWNhdGUtYXV0aG9yaXR5IjoicGF0aC90by9teS9jYWZpbGUiLCJzZXJ2ZXIiOiJodHRwczovL2hvcnNlLm9yZzo0NDQzIn0sIm5hbWUiOiJob3JzZS1jbHVzdGVyIn0seyJjbHVzdGVyIjp7Imluc2VjdXJlLXNraXAtdGxzLXZlcmlmeSI6dHJ1ZSwic2VydmVyIjoiaHR0cHM6Ly9waWcub3JnOjQ0MyJ9LCJuYW1lIjoicGlnLWNsdXN0ZXIifV0sImNvbnRleHRzIjpbeyJjb250ZXh0Ijp7ImNsdXN0ZXIiOiJob3JzZS1jbHVzdGVyIiwibmFtZXNwYWNlIjoiY2hpc2VsLW5zIiwidXNlciI6ImdyZWVuLXVzZXIifSwibmFtZSI6ImZlZGVyYWwtY29udGV4dCJ9LHsiY29udGV4dCI6eyJjbHVzdGVyIjoicGlnLWNsdXN0ZXIiLCJuYW1lc3BhY2UiOiJzYXctbnMiLCJ1c2VyIjoiYmxhY2stdXNlciJ9LCJuYW1lIjoicXVlZW4tYW5uZS1jb250ZXh0In1dLCJraW5kIjoiQ29uZmlnIiwicHJlZmVyZW5jZXMiOnsiY29sb3JzIjp0cnVlfSwidXNlcnMiOlt7Im5hbWUiOiJibHVlLXVzZXIiLCJ1c2VyIjp7InRva2VuIjoiYmx1ZS10b2tlbiJ9fSx7Im5hbWUiOiJncmVlbi11c2VyIiwidXNlciI6eyJjbGllbnQtY2VydGlmaWNhdGUiOiJwYXRoL3RvL215L2NsaWVudC9jZXJ0IiwiY2xpZW50LWtleSI6InBhdGgvdG8vbXkvY2xpZW50L2tleSJ9fV19",

        "autoExportPolicy": true,
        "autoExportCIDRs": ["10.211.55.0/24"],

        "labels": {"cloud": "on-prem", "creator": "ads-cluster-1"},
        "defaults": {"snapshotReserve": "5"},

        "storage": [
            {
                "labels": {"performance": "gold", "cost": "3"},
                "defaults": {
                    "qosPolicy": "gold",
                    "snapshotReserve": "10"
                }
            },
            {
                "labels": {"performance": "silver", "cost": "2"},
                "defaults": {"qosPolicy": "silver"}
            },
            {
                "labels": {"performance": "bronze", "cost": "1"},
                "defaults": {"qosPolicy": "bronze"}
            },
        ]
    }

The following StorageClass definitions refer to the above Virtual Storage Pools. Using the ``parameters.selector``
field, each StorageClass calls out which virtual pool(s) may be used to host a volume. The volume will have the
aspects defined in the chosen virtual pool.

.. code-block:: yaml

  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: ads
  provisioner: csi.trident.netapp.io
  parameters:
    backendType: astrads-nas
  allowVolumeExpansion: true

  ---
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: ads-gold
  provisioner: csi.trident.netapp.io
  parameters:
    backendType: astrads-nas
    selector: performance=gold
  allowVolumeExpansion: true

  ---
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: ads-silver
  provisioner: csi.trident.netapp.io
  parameters:
    backendType: astrads-nas
    selector: performance=silver
  allowVolumeExpansion: true

  ---
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: ads-bronze
  provisioner: csi.trident.netapp.io
  parameters:
    backendType: astrads-nas
    selector: performance=bronze
  allowVolumeExpansion: true
