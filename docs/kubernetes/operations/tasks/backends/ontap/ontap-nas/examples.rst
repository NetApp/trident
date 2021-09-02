######################
Example configurations
######################

Here are some examples for backend configurations with the ONTAP
NAS Drivers. These examples are classified into two types:

1. Basic configurations that leave most parameters to default. This
   is the easiest way to define a backend.

2. Backends with :ref:`Virtual Storage Pools <Virtual Storage Pools>`.

.. note::

   If you are using Amazon FSx on ONTAP with Trident, the recommendation is to specify DNS names for LIFs instead of IP addresses. 

Minimal backend configuration for ontap drivers
-----------------------------------------------

ontap-nas driver with certificate-based authentication
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

``clientCertificate``, ``clientPrivateKey`` and ``trustedCACertificate`` (optional,
if using trusted CA) are populated in backend.json and take the Base64-encoded
values of the client certificate, private key and trusted CA certificate respectively.

.. code-block:: json

  {
    "version": 1,
    "backendName": "DefaultNASBackend",
    "storageDriverName": "ontap-nas",
    "managementLIF": "10.0.0.1",
    "dataLIF": "10.0.0.15",
    "svm": "nfs_svm",
    "clientCertificate": "ZXR0ZXJwYXB...ICMgJ3BhcGVyc2",
    "clientPrivateKey": "vciwKIyAgZG...0cnksIGRlc2NyaX",
    "trustedCACertificate": "zcyBbaG...b3Igb3duIGNsYXNz",
    "storagePrefix": "myPrefix_"
  }

ontap-nas driver with auto export policy
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

This example shows you how you can instruct Trident to use dynamic
export policies to create and manage the export policy automatically.
This works the same for the ``ontap-nas-economy`` and ``ontap-nas-flexgroup``
drivers.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "labels": {"k8scluster": "test-cluster-east-1a", "backend": "test1-nasbackend"},
        "autoExportPolicy": true,
        "autoExportCIDRs": ["10.0.0.0/24"],
        "username": "admin",
        "password": "secret",
        "nfsMountOptions": "nfsvers=4",
    }

ontap-nas-flexgroup driver
~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas-flexgroup",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "labels": {"k8scluster": "test-cluster-east-1b", "backend": "test1-ontap-cluster"},
        "svm": "svm_nfs",
        "username": "vsadmin",
        "password": "secret",
    }

ontap-nas driver with IPv6
~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: json

   {
    "version": 1,
    "storageDriverName": "ontap-nas",
    "backendName": "nas_ipv6_backend",
    "managementLIF": "[5c5d:5edf:8f:7657:bef8:109b:1b41:d491]",
    "labels": {"k8scluster": "test-cluster-east-1a", "backend": "test1-ontap-ipv6"},
    "svm": "nas_ipv6_svm",
    "username": "vsadmin",
    "password": "netapp123"
   }

ontap-nas-economy driver
~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas-economy",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "vsadmin",
        "password": "secret"
    }

Backend and storage class configuration for ontap drivers with virtual storage pools
------------------------------------------------------------------------------------

This example shows the backend definition file configured with virtual storage pools along with StorageClasses that
refer back to them.

In the sample backend definition file shown below, specific defaults are set for all storage pools, such as
``spaceReserve`` at ``none``, ``spaceAllocation`` at ``false``, and ``encryption`` at ``false``. The virtual storage
pools are defined in the ``storage`` section. In this example, some of the storage pool sets their own
``spaceReserve``, ``spaceAllocation``, and ``encryption`` values, and some pools overwrite the default values set above.

ontap-nas driver with Virtual Pools
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "admin",
        "password": "secret",
        "nfsMountOptions": "nfsvers=4",

        "defaults": {
              "spaceReserve": "none",
              "encryption": "false",
              "qosPolicy": "standard"
        },
        "labels":{"store":"nas_store", "k8scluster": "prod-cluster-1"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"app":"msoffice", "cost":"100"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "true",
                    "unixPermissions": "0755",
                    "adaptiveQosPolicy": "adaptive-premium"
                }
            },
            {
                "labels":{"app":"slack", "cost":"75"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"app":"wordpress", "cost":"50"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0775"
                }
            },
            {
                "labels":{"app":"mysqldb", "cost":"25"},
                "zone":"us_east_1d",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "false",
                    "unixPermissions": "0775"
                }
            }
        ]
    }

ontap-nas-flexgroup driver with Virtual Storage Pools
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas-flexgroup",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "vsadmin",
        "password": "secret",

        "defaults": {
              "spaceReserve": "none",
              "encryption": "false"
        },
        "labels":{"store":"flexgroup_store", "k8scluster": "prod-cluster-1"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"protection":"gold", "creditpoints":"50000"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"protection":"gold", "creditpoints":"30000"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"protection":"silver", "creditpoints":"20000"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0775"
                }
            },
            {
                "labels":{"protection":"bronze", "creditpoints":"10000"},
                "zone":"us_east_1d",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "false",
                    "unixPermissions": "0775"
                }
            }
        ]
    }


ontap-nas-economy driver with Virtual Storage Pools
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas-economy",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "vsadmin",
        "password": "secret",

        "defaults": {
              "spaceReserve": "none",
              "encryption": "false"
        },
        "labels":{"store":"nas_economy_store"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"department":"finance", "creditpoints":"6000"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"department":"legal", "creditpoints":"5000"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"department":"engineering", "creditpoints":"3000"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0775"
                }
            },
            {
                "labels":{"department":"humanresource", "creditpoints":"2000"},
                "zone":"us_east_1d",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "false",
                    "unixPermissions": "0775"
                }
            }
        ]
    }

Mapping backends to StorageClasses
----------------------------------

The following StorageClass definitions refer to the above virtual storage pools. Using the ``parameters.selector`` field, each StorageClass calls out which virtual pool(s) may be used to host a volume. The volume will have the aspects defined in the chosen virtual pool.

* The first StorageClass (``protection-gold``) will map to the first, second virtual storage pool in ``ontap-nas-flexgroup`` backend and the first virtual storage pool in ``ontap-san`` backend . These are the only pool offering gold level protection.
* The second StorageClass (``protection-not-gold``) will map to the third, fourth virtual storage pool in ``ontap-nas-flexgroup`` backend and the second, third virtual storage pool in ``ontap-san`` backend . These are the only pool offering protection level other than gold.
* The third StorageClass (``app-mysqldb``) will map to the fourth virtual storage pool in ``ontap-nas`` backend and the third virtual storage pool in ``ontap-san-economy`` backend . These are the only pool offering storage pool configuration for mysqldb type app.
* The fourth StorageClass (``protection-silver-creditpoints-20k``) will map to the third virtual storage pool in ``ontap-nas-flexgroup`` backend and the second virtual storage pool in ``ontap-san`` backend . These are the only pool offering gold level protection at 20000 creditpoints.
* The fifth StorageClass (``creditpoints-5k``) will map to the second virtual storage pool in ``ontap-nas-economy`` backend and the third virtual storage pool in ``ontap-san`` backend. These are the only pool offerings at 5000 creditpoints.

Trident will decide which virtual storage pool is selected and will ensure the storage requirement is met.

.. code-block:: yaml

    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: protection-gold
    provisioner: netapp.io/trident
    parameters:
      selector: "protection=gold"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: protection-not-gold
    provisioner: netapp.io/trident
    parameters:
      selector: "protection!=gold"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: app-mysqldb
    provisioner: netapp.io/trident
    parameters:
      selector: "app=mysqldb"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: protection-silver-creditpoints-20k
    provisioner: netapp.io/trident
    parameters:
      selector: "protection=silver; creditpoints=20000"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: creditpoints-5k
    provisioner: netapp.io/trident
    parameters:
      selector: "creditpoints=5000"
