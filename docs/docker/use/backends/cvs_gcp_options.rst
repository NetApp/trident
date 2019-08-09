.. _cvs_gcp_vol_opts:

Cloud Volumes Service (CVS) on GCP Volume Options
=================================================

Volume create options for the CVS on GCP driver:

* ``size`` - the size of the volume, defaults to 1 TiB
* ``serviceLevel`` - the CVS service level of the volume, defaults to ``standard``. Valid values are ``standard``, ``premium``, and ``extreme``.
* ``snapshotReserve`` - this will set the snapshot reserve to the desired percentage. The default is no value, meaning CVS will select the snapshot reserve (usually 0%).

Using these options during the docker volume create operation is super simple, just provide the option and the value
using the ``-o`` operator during the CLI operation.  These override any equivalent values from the JSON configuration file.

.. code-block:: bash

   # create a 2TiB volume
   docker volume create -d netapp --name demo -o size=2T

   # create a 5TiB premium volume
   docker volume create -d netapp --name demo -o size=5T -o serviceLevel=premium

The minimum volume size is 1 TiB.