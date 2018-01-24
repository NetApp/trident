E-Series Configuration
======================

In addition to the global configuration values above, when using E-Series, these options are available.

+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| Option                    | Description                                                                                | Example       |
+===========================+============================================================================================+===============+
| ``webProxyHostname``      | Hostname or IP address of Web Services Proxy                                               | localhost     |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``webProxyPort``          | Port number of the Web Services Proxy (optional)                                           | 8443          |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``webProxyUseHTTP``       | Use HTTP instead of HTTPS for Web Services Proxy (default = false)                         | true          |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``webProxyVerifyTLS``     | Verify server's certificate chain and hostname (default = false)                           | true          |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``username``              | Username for Web Services Proxy                                                            | rw            |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``password``              | Password for Web Services Proxy                                                            | rw            |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``controllerA``           | IP address of controller A                                                                 | 10.0.0.5      |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``controllerB``           | IP address of controller B                                                                 | 10.0.0.6      |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``passwordArray``         | Password for storage array if set                                                          | blank/empty   |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``hostDataIP``            | Host iSCSI IP address (if multipathing just choose either one)                             | 10.0.0.101    |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``poolNameSearchPattern`` | Regular expression for matching storage pools available for Trident volumes (default = .+) | docker.*      |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``hostType``              | Type of E-series Host created by Trident (default = linux_dm_mp)                           | linux_dm_mp   |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+
| ``accessGroupName``       | Name of E-series Host Group to contain Hosts defined by Trident (default = netappdvp)      | DockerHosts   |
+---------------------------+--------------------------------------------------------------------------------------------+---------------+

Example E-Series Config File
----------------------------

**Example for eseries-iscsi driver**

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

E-Series Array Setup Notes
--------------------------

The E-Series Docker driver can provision Docker volumes in any storage pool on the array, including volume groups
and DDP pools. To limit the Docker driver to a subset of the storage pools, set the ``poolNameSearchPattern`` in the
configuration file to a regular expression that matches the desired pools.

When creating a docker volume you can specify the volume size as well as the disk media type using the
``-o`` option and the tags ``size`` and ``mediaType``. Valid values for media type are ``hdd`` and ``ssd``. Note that
these are optional; if unspecified, the defaults will be a *1 GB* volume allocated from an *HDD pool*. An example
of using these tags to create a 2 GiB volume from an SSD-based pool:

  .. code-block:: bash

     docker volume create -d netapp --name my_vol -o size=2G -o mediaType=ssd

The E-series Docker driver will detect and use any preexisting Host definitions without modification, and
the driver will automatically define Host and Host Group objects as needed. The host type for hosts created
by the driver defaults to ``linux_dm_mp``, the native DM-MPIO multipath driver in Linux.

The current E-series Docker driver only supports iSCSI.