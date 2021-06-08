Managing Trident
################

Installing Trident
------------------

Follow the extensive :ref:`deployment <deploying-in-docker>` guide.

Updating Trident
----------------

The plugin is not in the data path, therefore you can safely upgrade it without any impact to volumes that are
in use. As with any plugin, during the upgrade process there will be a brief period where 'docker volume' commands
directed at the plugin will not succeed, and applications will be unable to mount volumes until the plugin is running
again. Under most circumstances, this is a matter of seconds.

#. List the existing volumes:

   .. code-block:: bash

     docker volume ls
     DRIVER              VOLUME NAME
     netapp:latest       my_volume

#. Disable the plugin:

   .. code-block:: bash

     docker plugin disable -f netapp:latest
     docker plugin ls
     ID                  NAME                DESCRIPTION                          ENABLED
     7067f39a5df5        netapp:latest       nDVP - NetApp Docker Volume Plugin   false

#. Upgrade the plugin:

   .. code-block:: bash

     docker plugin upgrade --skip-remote-check --grant-all-permissions netapp:latest netapp/trident-plugin:21.04

   .. note::

     The 18.01 release of Trident replaces the nDVP. You should upgrade directly from the netapp/ndvp-plugin image to
     the netapp/trident-plugin image.

#. Enable the plugin:

   .. code-block:: bash

     docker plugin enable netapp:latest

#. Verify that the plugin is enabled:

   .. code-block:: bash

     docker plugin ls
     ID                  NAME                DESCRIPTION                             ENABLED
     7067f39a5df5        netapp:latest       Trident - NetApp Docker Volume Plugin   true

#. Verify that the volumes are visible:

   .. code-block:: bash

     docker volume ls
     DRIVER              VOLUME NAME
     netapp:latest       my_volume


   .. note::

      If you are upgrading from an old version of Trident (pre-20.10) to Trident 20.10 or later, you might run into an error. For more information, see :ref:`Known Issues <known-issues>`. If you run into the error, you should first disable the plugin, then remove the plugin, and then install the required Trident version by passing an extra config parameter:

      .. code-block:: bash

       docker plugin install netapp/trident-plugin:20.10 --alias netapp --grant-all-permissions config=config.json

Uninstalling Trident
--------------------

#. Remove any volumes that the plugin created.

#. Disable the plugin:

   .. code-block:: bash

     docker plugin disable netapp:latest
     docker plugin ls
     ID                  NAME                DESCRIPTION                          ENABLED
     7067f39a5df5        netapp:latest       nDVP - NetApp Docker Volume Plugin   false

#. Remove the plugin:

   .. code-block:: bash

     docker plugin rm netapp:latest


  .. note::

    If you are upgrading from an old version of Trident (pre-20.10) to Trident 20.10 or later, you might run into an error. For more information, see :ref:`Known Issues <known-issues>`. If you run into the error, you should first disable the plugin, then remove the plugin, and then install the required Trident version by passing an extra config parameter:

    .. code-block:: bash

     docker plugin install netapp/trident-plugin:20.10 --alias netapp --grant-all-permissions config=config.json
