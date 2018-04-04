.. _es_vol_opts:

E-Series Volume Options
=======================

Media Type
----------

The E-Series driver offers the ability to specify the type of disk which will be used to back the volume and,
like the other drivers, the ability to set the size of the volume at creation time.

Currently only two values for ``mediaType`` are supported:  ``ssd`` and ``hdd``.

.. code-block:: bash

   # create a 10GiB SSD backed volume
   docker volume create -d eseries --name eseriesSsd -o mediaType=ssd -o size=10G

   # create a 100GiB HDD backed volume
   docker volume create -d eseries --name eseriesHdd -o mediaType=hdd -o size=100G

File System Type
----------------

The user can specify the file system type to use to format the volume.  The default for ``fileSystemType``
is ``ext4``.  Valid values are ``ext3``, ``ext4``, and ``xfs``.

.. code-block:: bash

  # create a volume using xfs
  docker volume create -d eseries --name xfsVolume -o fileSystemType=xfs
