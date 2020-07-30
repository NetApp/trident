#####################
Backend configuration
#####################

A Trident backend defines the relationship between Trident and a storage system.
It tells Trident how to communicate with that storage system and how Trident
should provision volumes from it.

Trident will automatically offer up storage pools from backends that together
match the requirements defined by a storage class.

To get started, choose the storage system type that you will be using as a
backend:

.. toctree::
   :maxdepth: 2

   anf.rst
   cvs_aws.rst
   cvs_gcp.rst
   element.rst
   ontap/index.rst
   santricity.rst
