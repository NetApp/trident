#############################
Cloud Volumes Service for AWS
#############################

.. warning::
  The NetApp Cloud Volumes Service for AWS does not support volumes less than 100 GB in size. To
  make it easier to deploy applications, Trident automatically creates 100 GB volumes if a
  smaller volume is requested. Future releases of the Cloud Volumes Service may remove this restriction.

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

Example configuration
---------------------

**Minimal example backend configuration for aws-cvs driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "aws-cvs",
        "apiRegion": "us-east-1",
        "apiURL": "https://cds-aws-bundles.netapp.com:8080/v1",
        "apiKey": "znHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE",
        "secretKey": "rR0rUmWXfNioN1KhtHisiSAnoTherboGuskey6pU"
    }

**Example backend configuration for aws-cvs driver with single service level**

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

**Example backend configuration for aws-cvs driver with multiple service levels**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "aws-cvs",
        "apiRegion": "us-east-1",
        "apiURL": "https://cds-aws-bundles.netapp.com:8080/v1",
        "apiKey": "znHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE",
        "secretKey": "rR0rUmWXfNioN1KhtHisiSAnoTherboGuskey6pU",
        "nfsMountOptions": "vers=3,proto=tcp,timeo=600",

        "defaults": {
            "snapshotReserve": "10",
            "exportRule": "0.0.0.0/0,10.0.0.0/24",
            "size": "200Gi"
        },

        "labels": {"cloud": "aws"},
        "region": "us-east-1",

        "storage": [
            {
                "labels": {"performance": "extreme"},
                "serviceLevel": "extreme",
                "defaults": {
                    "snapshotReserve": "5",
                    "exportRule": "0.0.0.0/0",
                    "size": "100Gi"
                }
            },
            {
                "labels": {"performance": "premium"},
                "serviceLevel": "premium"
            },
            {
                "labels": {"performance": "standard"},
                "serviceLevel": "standard"
            }
        ]
    }

**Example storage class definitions for aws-cvs driver with multiple service levels**

.. code-block:: yaml

    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-extreme
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=extreme"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-premium
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=premium"
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

.. _AWS account configured with NetApp CVS: https://cloud.netapp.com/cloud-volumes-service-for-aws?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident