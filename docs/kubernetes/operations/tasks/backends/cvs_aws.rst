#############################
Cloud Volumes Service for AWS
#############################

.. note::
  The NetApp Cloud Volumes Service for AWS does not support volumes less than 100 GB in size. To
  make it easier to deploy applications, Trident automatically creates 100 GB volumes if a
  smaller volume is requested. Future releases of the Cloud Volumes Service may remove this restriction.


Preparation
-----------

To create and use a Cloud Volumes Service (CVS) for AWS backend, you will need:

* An `AWS account configured with NetApp CVS`_
* API region, URL, and keys for your CVS account

Backend configuration options
-----------------------------

========================= =============================================================== ================================================
Parameter                 Description                                                     Default
========================= =============================================================== ================================================
version                   Always 1
storageDriverName         "aws-cvs"
backendName               Custom name for the storage backend                             Driver name + "_" + part of API key
apiRegion                 CVS account region
apiURL                    CVS account API URL
apiKey                    CVS account API key
secretKey                 CVS account secret key
nfsMountOptions           Fine-grained control of NFS mount options                       "-o nfsvers=3"
limitVolumeSize           Fail provisioning if requested volume size is above this value  "" (not enforced by default)
serviceLevel              The CVS service level for new volumes                           "standard"
========================= =============================================================== ================================================

The required values ``apiRegion``, ``apiURL``, ``apiKey``, and ``secretKey``
may be found in the CVS web portal in Account settings / API access.

Each backend provisions volumes in a single AWS region. To create volumes in
other regions, you can define additional backends.

The serviceLevel values for CVS on AWS are ``standard``, ``premium``, and ``extreme``.

You can control how each volume is provisioned by default using these options
in a special section of the configuration. For an example, see the
configuration examples below.

========================= =============================================================== ================================================
Parameter                 Description                                                     Default
========================= =============================================================== ================================================
exportRule                The export rule(s) for new volumes                              "0.0.0.0/0"
snapshotReserve           Percentage of volume reserved for snapshots                     "" (accept CVS default of 0)
size                      The size of new volumes                                         "100G"
========================= =============================================================== ================================================

The ``exportRule`` value must be a comma-separated list of any combination of
IPv4 addresses or IPv4 subnets in CIDR notation.

Example configurations
----------------------

**Example 1 - Minimal backend configuration for aws-cvs driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "aws-cvs",
        "apiRegion": "us-east-1",
        "apiURL": "https://cds-aws-bundles.netapp.com:8080/v1",
        "apiKey": "znHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE",
        "secretKey": "rR0rUmWXfNioN1KhtHisiSAnoTherboGuskey6pU"
    }

**Example 2 -  Backend configuration for aws-cvs driver with single service level**

This example shows a backend file that applies the same aspects to all Trident created storage in the AWS us-east-1 region.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "aws-cvs",
        "backendName": "cvs-aws-us-east",
        "apiRegion": "us-east-1",
        "apiURL": "https://cds-aws-bundles.netapp.com:8080/v1",
        "apiKey": "znHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE",
        "secretKey": "rR0rUmWXfNioN1KhtHisiSAnoTherboGuskey6pU",
        "nfsMountOptions": "vers=3,proto=tcp,timeo=600",
        "limitVolumeSize": "50Gi",
        "serviceLevel": "premium",
        "defaults": {
            "snapshotReserve": "5",
            "exportRule": "10.0.0.0/24,10.0.1.0/24,10.0.2.100",
            "size": "200Gi"
        }
    }

**Example 3 - Backend and storage class configuration for aws-cvs driver with virtual storage pools**

This example shows the backend definition file configured with :ref:`Virtual Storage Pools <Virtual Storage Pools>`
along with StorageClasses that refer back to them.

In the sample backend definition file shown below, specific defaults are set for all storage pools, which set the ``snapshotReserve`` at 5% and the ``exportRule`` to 0.0.0.0/0. The virtual storage pools are defined in the ``storage`` section. In this example, each individual storage pool sets its own ``serviceLevel``, and some pools overwrite the default values set above.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "aws-cvs",
        "apiRegion": "us-east-1",
        "apiURL": "https://cds-aws-bundles.netapp.com:8080/v1",
        "apiKey": "EnterYourAPIKeyHere***********************",
        "secretKey": "EnterYourSecretKeyHere******************",
        "nfsMountOptions": "vers=3,proto=tcp,timeo=600",

        "defaults": {
            "snapshotReserve": "5",
            "exportRule": "0.0.0.0/0"
        },

        "labels": {
            "cloud": "aws"
        },
        "region": "us-east-1",

        "storage": [
            {
                "labels": {
                    "performance": "extreme",
                    "protection": "extra"
                },
                "serviceLevel": "extreme",
                "defaults": {
                    "snapshotReserve": "10",
                    "exportRule": "10.0.0.0/24"
                }
            },
            {
                "labels": {
                    "performance": "extreme",
                    "protection": "standard"
                },
                "serviceLevel": "extreme"
            },
            {
                "labels": {
                    "performance": "premium",
                    "protection": "extra"
                },
                "serviceLevel": "premium",
                "defaults": {
                    "snapshotReserve": "10"
                }
            },

            {
                "labels": {
                    "performance": "premium",
                    "protection": "standard"
                },
                "serviceLevel": "premium"
            },

            {
                "labels": {
                    "performance": "standard"
                },
                "serviceLevel": "standard"
            }
        ]
    }

The following StorageClass definitions refer to the above Virtual Storage Pools. Using the ``parameters.selector`` field, each StorageClass calls out which virtual pool(s) may be used to host a volume. The volume will have the aspects defined in the chosen virtual pool.

The first StorageClass (``cvs-extreme-extra-protection``) will map to the first Virtual Storage Pool. This is the only pool offering extreme performance with a snapshot reserve of 10%. The last StorageClass (``cvs-extra-protection``) calls out any storage pool which provides a snapshot reserve of 10%. Trident will decide which Virtual Storage Pool is selected and will ensure the snapshot reserve requirement is met.

.. code-block:: yaml

    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-extreme-extra-protection
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=extreme; protection=extra"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-extreme-standard-protection
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=premium; protection=standard"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-premium-extra-protection
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=premium; protection=extra"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-premium
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=premium; protection=standard"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-standard
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=standard"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-extra-protection
    provisioner: netapp.io/trident
    parameters:
      selector: "protection=extra"
    allowVolumeExpansion: true
 
.. _AWS account configured with NetApp CVS: https://cloud.netapp.com/cloud-volumes-service-for-aws?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
