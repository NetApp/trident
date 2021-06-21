.. _sf_vol_opts:

Element Software Volume Options
===============================

The Element Software driver options expose the size and quality of service (QoS) policies associated with the volume. When the volume is created, the QoS policy associated with it is specified using the ``-o type=service_level`` nomenclature.

The first step to defining a QoS service level with the Element driver is to create at least one type and specify the minimum, maximum, and burst IOPS associated with a name in the configuration file.

**Example Configuration File with QoS Definitions**

.. code-block:: json

   {
       "...": "..."
       "Types": [
           {
               "Type": "Bronze",
               "Qos": {
                   "minIOPS": 1000,
                   "maxIOPS": 2000,
                   "burstIOPS": 4000
               }
           },
           {
               "Type": "Silver",
               "Qos": {
                   "minIOPS": 4000,
                   "maxIOPS": 6000,
                   "burstIOPS": 8000
               }
           },
           {
               "Type": "Gold",
               "Qos": {
                   "minIOPS": 6000,
                   "maxIOPS": 8000,
                   "burstIOPS": 10000
               }
           }
       ]
   }

In the above configuration we have three policy definitions: *Bronze*, *Silver*, and *Gold*. These names are well known and fairly common, but we could have just as easily chosen: *pig*, *horse*, and *cow*, the names are arbitrary.

.. code-block:: bash

   # create a 10GiB Gold volume
   docker volume create -d solidfire --name sfGold -o type=Gold -o size=10G

   # create a 100GiB Bronze volume
   docker volume create -d solidfire --name sfBronze -o type=Bronze -o size=100G

Other Element Software Create Options
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Volume create options for Element include:

* ``size`` - the size of the volume, defaults to 1GiB or config entry ``... "defaults": {"size": "5G"}``
* ``blocksize`` - use either ``512`` or ``4096``, defaults to 512 or config entry ``DefaultBlockSize``
