#####################
SANtricity (E-Series)
#####################

To create and use an E-Series backend, you will need:

* A :ref:`supported E-Series storage system <Supported backends (storage)>`
* Complete `E-Series backend preparation`_
* Credentials to the E-Series storage system

.. _E-Series backend preparation:

Preparation
-----------

All of your Kubernetes worker nodes must have the appropriate iSCSI tools
installed. See the :ref:`worker configuration guide <iSCSI>` for more details.

Trident uses host groups to control access to the volumes (LUNs) that it
provisions. It expects to find a host group called ``trident`` unless a
different host group name is specified in the configuration.

While Trident associates new volumes with the configured host group, it does
not create or otherwise manage host groups themselves. The host group must
exist before the storage backend is added to Trident, and it needs to contain
a host definition with an iSCSI IQN for every worker node in the Kubernetes
cluster.

..
  The E-Series driver can provision volumes in any storage pool on the array,
  including volume groups and DDP pools. To limit the driver to a subset of the
  storage pools, set the ``poolNameSearchPattern`` in the configuration file to a
  regular expression that matches the desired pools.

  The E-series driver will detect and use any pre-existing Host definitions that
  the array is aware of without modification, and the driver will automatically
  define Host and Host Group objects as needed. The host type for hosts created
  by the driver defaults to ``linux_dm_mp``, the native DM-MPIO multipath driver
  in Linux.

Backend configuration options
-----------------------------

===================== =============================================================== ================================================
Parameter             Description                                                     Default
===================== =============================================================== ================================================
version               Always 1
storageDriverName     Always "eseries-iscsi"
backendName           Custom name for the storage backend                             "eseries\_" + hostDataIP
webProxyHostname      Hostname or IP address of the web services proxy
webProxyPort          Port number of the web services proxy                           80 for HTTP, 443 for HTTPS
webProxyUseHTTP       Use HTTP instead of HTTPS to communicate to the proxy           false
webProxyVerifyTLS     Verify certificate chain and hostname                           false
username              Username for the web services proxy
password              Password for the web services proxy
controllerA           IP address for controller A
controllerB           IP address for controller B
passwordArray         Password for the storage array, if set                          ""
hostDataIP            Host iSCSI IP address
poolNameSearchPattern Regular expression for matching available storage pools         ".+" (all)
hostType              E-Series Host types created by the driver                       "linux_dm_mp"
accessGroupName       E-Series Host Group used by the driver                          "trident"
limitVolumeSize       Fail provisioning if requested volume size is above this value  "" (not enforced by default)
===================== =============================================================== ================================================

Example configuration
---------------------

**Example 1 - Minimal backend configuration for eseries-iscsi driver**

.. code-block:: json

  {
    "version": 1,
    "storageDriverName": "eseries-iscsi",
    "webProxyHostname": "localhost",
    "webProxyPort": "8443",
    "username": "rw",
    "password": "rw",
    "controllerA": "10.0.0.5",
    "controllerB": "10.0.0.6",
    "passwordArray": "",
    "hostDataIP": "10.0.0.101"
  }

**Example 2 - Backend and storage class configuration for eseries-iscsi driver with virtual storage pools**

This example shows the backend definition file configured with virtual storage pools along with StorageClasses that refer back to them.

In the sample backend definition file shown below, the virtual storage pools are defined in the ``storage`` section.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "eseries-iscsi",
        "controllerA": "10.0.0.1",
        "controllerB": "10.0.0.2",
        "hostDataIP": "10.0.1.1",
        "username": "user",
        "password": "password",
        "passwordArray": "password",
        "webProxyHostname": "10.0.2.1",

        "labels":{"store":"eseries"},
        "region":"us-east",

        "storage":[
            {
                "labels":{"performance":"gold", "cost":"4"},
                "zone":"us-east-1a"
            },
            {
                "labels":{"performance":"silver", "cost":"3"},
                "zone":"us-east-1b"
            },
            {
                "labels":{"performance":"bronze", "cost":"2"},
                "zone":"us-east-1c"
            },
            {
                "labels":{"performance":"bronze", "cost":"1"},
                "zone":"us-east-1d"
            }
        ]
    }

The following StorageClass definitions refer to the above virtual storage pools. Using the ``parameters.selector`` field, each StorageClass calls out which virtual pool(s) may be used to host a volume. The volume will have the aspects defined in the chosen virtual pool.

The first StorageClass (``eseries-gold-four``) will map to the first virtual storage pool. This is the only pool offering gold performance in zone ``us-east-1a``. The last StorageClass (``eseries-bronze``) calls out any storage pool which offers a bronze performance. Trident will decide which virtual storage pool is selected and will ensure the storage requirement is met.

.. code-block:: yaml

    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: eseries-gold-four
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=gold; cost=4"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: eseries-silver-three
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=silver; cost=3"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: eseries-bronze-two
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=bronze; cost=2"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: eseries-bronze-one
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=bronze; cost=1"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: eseries-bronze
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=bronze"

