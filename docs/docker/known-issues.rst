Known issues
^^^^^^^^^^^^

* **Upgrading Trident Docker Volume Plugin to 20.10 and later from older versions results in upgrade failure with the no such file or directory error.**

Workaround:

#. Disable the plugin.

    .. code-block:: bash

       docker plugin disable -f netapp:latest

#. Remove the plugin.

    .. code-block:: bash

       docker plugin rm -f netapp:latest

#. Reinstall the plugin by providing the extra config parameter.

    .. code-block:: bash

       docker plugin install netapp/trident-plugin:20.10 --alias netapp --grant-all-permissions config=config.json

* **Volume names must be a minimum of 2 characters in length**

   .. note::
      This is a Docker client limitation. The client will interpret a single character name as being a Windows path.
      `See bug 25773 <https://github.com/docker/docker/issues/25773>`_.

* **Docker Swarm has certain behaviors that prevent us from supporting it with every storage and driver combination**:

   - Docker Swarm presently makes use of volume name instead of volume ID as its unique volume identifier.
   - Volume requests are simultaneously sent to each node in a Swarm cluster.
   - Volume Plugins (including Trident) must run independently on each node in a Swarm cluster.

   Due to the way ONTAP works and how the ontap-nas and ontap-san drivers function, they are the only ones that
   happen to be able to operate within these limitations.

   The rest of the drivers are subject to issues like race conditions that can result in the creation of a large
   number of volumes for a single request without a clear "winner"; for example, Element has a feature that allows
   volumes to have the same name but different IDs.

   NetApp has provided feedback to the Docker team, but does not have any indication of future recourse.

* **If a FlexGroup is in the process of being provisioned, ONTAP will not provision a second FlexGroup if the second FlexGroup has one or more aggregates in common with the FlexGroup being provisioned.**
