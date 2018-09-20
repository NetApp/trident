Global Configuration
====================

These configuration variables apply to all Trident configurations, regardless of the storage platform being used.

+-----------------------+-----------------------------------------------------------------------------------------------------------------------+-------------+
| Option                | Description                                                                                                           | Example     |
+=======================+=======================================================================================================================+=============+
| ``version``           | Config file version number                                                                                            | 1           |
+-----------------------+-----------------------------------------------------------------------------------------------------------------------+-------------+
| ``storageDriverName`` | ``ontap-nas``, ``ontap-nas-economy``, ``ontap-nas-flexgroup``, ``ontap-san``, ``eseries-iscsi``, or ``solidfire-san`` | ontap-nas   |
+-----------------------+-----------------------------------------------------------------------------------------------------------------------+-------------+
| ``storagePrefix``     | Optional prefix for volume names.  Default: "netappdvp\_"                                                             | netappdvp\_ |
+-----------------------+-----------------------------------------------------------------------------------------------------------------------+-------------+
| ``limitVolumeSize``   | Optional restriction on volume sizes.  Default: "" (not enforced)                                                     | 10g         |
+-----------------------+-----------------------------------------------------------------------------------------------------------------------+-------------+

Also, default option settings are available to avoid having to specify them on every volume create.  The ``size``
option is available for all controller types.  See the ONTAP config section for an example of how to set the default
volume size.

+-----------------------+--------------------------------------------------------------------------+------------+
| Defaults Option       | Description                                                              | Example    |
+=======================+==========================================================================+============+
| ``size``              | Optional default size for new volumes.  Default: "1G"                    | 10G        |
+-----------------------+--------------------------------------------------------------------------+------------+

**Storage Prefix**

A new config file variable has been added in v1.2 called "storagePrefix" that allows you to modify the prefix applied to volume names by the plugin.  By default, when you run `docker volume create`, the volume name supplied is prepended with "netappdvp\_" *("netappdvp-" for SolidFire)*.

If you wish to use a different prefix, you can specify it with this directive.  Alternatively, you can use *pre-existing* volumes with the volume plugin by setting ``storagePrefix`` to an empty string, "".

*SolidFire specific recommendation* do not use a storagePrefix (including the default).  By default the SolidFire driver will ignore this setting and not use a prefix. We recommend using either a specific tenantID for docker volume mapping or using the attribute data which is populated with the docker version, driver info and raw name from docker in cases where any name munging may have been used.

**A note of caution**: `docker volume rm` will *delete* these volumes just as it does volumes created by the plugin using the default prefix.  Be very careful when using pre-existing volumes!
