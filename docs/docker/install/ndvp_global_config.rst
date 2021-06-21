.. _ndvp-global-config:

Global Configuration
====================

These configuration variables apply to all Trident configurations, regardless of the storage platform being used.

+-----------------------+----------------------------------------------------------------------------------------------+-------------+
| Option                | Description                                                                                  | Example     |
+=======================+==============================================================================================+=============+
| ``version``           | Config file version number                                                                   | 1           |
+-----------------------+----------------------------------------------------------------------------------------------+-------------+
| ``storageDriverName`` | | ``ontap-nas``, ``ontap-san``, ``ontap-nas-economy``,                                       | ontap-nas   |
|                       | | ``ontap-nas-flexgroup``, ``eseries-iscsi``,                                                |             |
|                       | | ``solidfire-san``, ``azure-netapp-files``, ``aws-cvs``, or ``gcp-cvs``.                    |             |
+-----------------------+----------------------------------------------------------------------------------------------+-------------+
| ``storagePrefix``     | Optional prefix for volume names.  Default: "netappdvp\_".                                   | staging\_   |
+-----------------------+----------------------------------------------------------------------------------------------+-------------+
| ``limitVolumeSize``   | Optional restriction on volume sizes.  Default: "" (not enforced)                            | 10g         |
+-----------------------+----------------------------------------------------------------------------------------------+-------------+

Also, default option settings are available to avoid having to specify them on every volume create.  The ``size``
option is available for all controller types.  See the ONTAP config section for an example of how to set the default
volume size.

+-----------------------+--------------------------------------------------------------------------+------------+
| Defaults Option       | Description                                                              | Example    |
+=======================+==========================================================================+============+
| ``size``              | Optional default size for new volumes.  Default: "1G"                    | 10G        |
+-----------------------+--------------------------------------------------------------------------+------------+

**Storage Prefix**


Do not use a storagePrefix (including the default) for Element backends.  By default the ``solidfire-san`` driver will ignore this setting and not use a prefix. We recommend using either a specific tenantID for docker volume mapping or using the attribute data which is populated with the docker version, driver info and raw name from docker in cases where any name munging may have been used.

**A note of caution**: `docker volume rm` will *delete* these volumes just as it does volumes created by the plugin using the default prefix.  Be very careful when using pre-existing volumes!
