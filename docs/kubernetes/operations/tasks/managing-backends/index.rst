.. _backend-management:

#################
Managing backends
#################

Welcome to the backend management guide. This walks you through the operations
that can be performed on Trident backends and how they can be handled.

You have a couple of options when it comes to backend management:

* Using the Kubernetes CLI tool, such as ``kubectl``. See :ref:`Managing backends using kubectl <manage_tbc_backend>`.
* Using ``tridentctl``. The ``tridentctl create backend`` command takes a JSON file as an argument.

.. toctree::
   :maxdepth: 3

   tbc.rst
   backend-operations.rst
   moving-between-options.rst
