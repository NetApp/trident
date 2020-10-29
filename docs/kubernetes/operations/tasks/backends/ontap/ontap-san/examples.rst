######################
Example configurations
######################

Here are some examples for backend configurations with the ONTAP SAN Drivers.
These examples are classified into two types:

1. Basic configurations that leave most parameters to default.
   This is the easiest way to define a backend.

2. Backends with Virtual Storage Pools.

Minimal backend configuration for ontap drivers
-----------------------------------------------

ontap-san driver with bidirectional CHAP
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

This basic configuration creates a ``ontap-san`` backend
with ``useCHAP`` set to ``true``.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.3",
        "svm": "svm_iscsi",
        "useCHAP": true,
        "igroupName": "trident",
        "username": "vsadmin",
        "password": "secret"
    }

ontap-san-economy driver
~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san-economy",
        "managementLIF": "10.0.0.1",
        "svm": "svm_iscsi_eco",
        "useCHAP": true,
        "igroupName": "trident",
        "username": "vsadmin",
        "password": "secret"
    }

Backend and storage class configuration for ontap drivers with virtual storage pools
------------------------------------------------------------------------------------

These examples show the backend definition file configured with virtual storage pools along with StorageClasses that
refer back to them.

In the sample backend definition file shown below, specific defaults are set for all storage pools, such as
``spaceReserve`` at ``none``, ``spaceAllocation`` at ``false``, and ``encryption`` at ``false``. The virtual storage
pools are defined in the ``storage`` section. In this example, some of the storage pool sets their own
``spaceReserve``, ``spaceAllocation``, and ``encryption`` values, and some pools overwrite the default values set above.

ontap-san driver
~~~~~~~~~~~~~~~~

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.3",
        "svm": "svm_iscsi",
        "useCHAP": true,
        "igroupName": "trident",
        "username": "vsadmin",
        "password": "secret",

        "defaults": {
              "spaceAllocation": "false",
              "encryption": "false"
        },
        "labels":{"store":"san_store"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"protection":"gold", "creditpoints":"40000"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceAllocation": "true",
                    "encryption": "true"
                }
            },
            {
                "labels":{"protection":"silver", "creditpoints":"20000"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceAllocation": "false",
                    "encryption": "true"
                }
            },
            {
                "labels":{"protection":"bronze", "creditpoints":"5000"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceAllocation": "true",
                    "encryption": "false"
                }
            }
        ]
    }

iSCSI Example for ontap-san-economy driver
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san-economy",
        "managementLIF": "10.0.0.1",
        "svm": "svm_iscsi_eco",
        "useCHAP": true,
        "igroupName": "trident",
        "username": "vsadmin",
        "password": "secret",

        "defaults": {
              "spaceAllocation": "false",
              "encryption": "false"
        },
        "labels":{"store":"san_economy_store"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"app":"oracledb", "cost":"30"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceAllocation": "true",
                    "encryption": "true"
                }
            },
            {
                "labels":{"app":"postgresdb", "cost":"20"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceAllocation": "false",
                    "encryption": "true"
                }
            },
            {
                "labels":{"app":"mysqldb", "cost":"10"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceAllocation": "true",
                    "encryption": "false"
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
      fsType: "ext4"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: protection-not-gold
    provisioner: netapp.io/trident
    parameters:
      selector: "protection!=gold"
      fsType: "ext4"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: app-mysqldb
    provisioner: netapp.io/trident
    parameters:
      selector: "app=mysqldb"
      fsType: "ext4"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: protection-silver-creditpoints-20k
    provisioner: netapp.io/trident
    parameters:
      selector: "protection=silver; creditpoints=20000"
      fsType: "ext4"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: creditpoints-5k
    provisioner: netapp.io/trident
    parameters:
      selector: "creditpoints=5000"
      fsType: "ext4"
