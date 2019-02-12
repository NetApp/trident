.. _cvs_aws_vol_opts:

Cloud Volumes Service (CVS) on AWS Volume Options
=================================================

Volume create options for the CVS on AWS driver:

* ``size`` - the size of the volume, defaults to 100 GB
* ``serviceLevel`` - the CVS service level of the volume, defaults to ``standard``. Valid values are ``standard``, ``premium``, and ``extreme``.
* ``snapshotReserve`` - this will set the snapshot reserve to the desired percentage. The default is no value, meaning CVS will select the snapshot reserve (usually 0%).

Using these options during the docker volume create operation is super simple, just provide the option and the value
using the ``-o`` operator during the CLI operation.  These override any equivalent values from the JSON configuration file.

.. code-block:: bash

   # create a 200GiB volume
   docker volume create -d netapp --name demo -o size=200G

   # create a 500GiB premium volume
   docker volume create -d netapp --name demo -o size=500G -o serviceLevel=premium

The minimum volume size is 100 GB.