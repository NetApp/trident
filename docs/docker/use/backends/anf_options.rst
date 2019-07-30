.. _anf_vol_opts:

Azure NetApp Files Volume Options
=================================

Volume create options for the Azure NetApp Files driver:

* ``size`` - the size of the volume, defaults to 100 GB

Using these options during the docker volume create operation is super simple, just provide the option and the value
using the ``-o`` operator during the CLI operation.  These override any equivalent values from the JSON configuration
file.

.. code-block:: bash

   # create a 200GiB volume
   docker volume create -d netapp --name demo -o size=200G

The minimum volume size is 100 GB.