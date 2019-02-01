.. _deploying-in-docker:

Deploying
=========

#. Verify that your deployment meets all of the :ref:`requirements <Requirements>`.

#. Ensure that you have a supported version of Docker installed.

   .. code-block:: bash
   
     docker --version
   
   If your version is out of date, `follow the instructions for your distribution <https://docs.docker.com/engine/installation/>`_
   to install or update.

#. Verify that the protocol prerequisites are installed and configured on your host.  See :ref:`host-configuration`.

#. Create a configuration file.  The default location is ``/etc/netappdvp/config.json``.  Be sure to use the correct
   options for your storage system.

   .. code-block:: bash
   
     # create a location for the config files
     sudo mkdir -p /etc/netappdvp
 
     # create the configuration file, see below for more configuration examples
     cat << EOF > /etc/netappdvp/config.json
     {
         "version": 1,
         "storageDriverName": "ontap-nas",
         "managementLIF": "10.0.0.1",
         "dataLIF": "10.0.0.2",
         "svm": "svm_nfs",
         "username": "vsadmin",
         "password": "secret",
         "aggregate": "aggr1"
     }
     EOF

#. Start Trident using the managed plugin system.

   .. code-block:: bash
   
     docker plugin install netapp/trident-plugin:19.04 --alias netapp --grant-all-permissions

#. Begin using Trident to consume storage from the configured system.

   .. code-block:: bash
   
     # create a volume named "firstVolume"
     docker volume create -d netapp --name firstVolume
     
     # create a default volume at container instantiation
     docker run --rm -it --volume-driver netapp --volume secondVolume:/my_vol alpine ash
     
     # remove the volume "firstVolume"
     docker volume rm firstVolume
