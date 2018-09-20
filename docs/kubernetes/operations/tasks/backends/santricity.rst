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

.. code-block:: json

  {
    "version": 1,
    "storageDriverName": "eseries-iscsi",
    "webProxyHostname": "localhost",
    "webProxyPort": "8443",
    "webProxyUseHTTP": false,
    "webProxyVerifyTLS": true,
    "username": "rw",
    "password": "rw",
    "controllerA": "10.0.0.5",
    "controllerB": "10.0.0.6",
    "passwordArray": "",
    "hostDataIP": "10.0.0.101"
  }
