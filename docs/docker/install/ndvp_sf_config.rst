Element Software Configuration
==============================

In addition to the :ref:`global configuration values <ndvp-global-config>`, when using Element software (NetApp HCI/SolidFire), these options are available.

+-----------------------+-------------------------------------------------------------------------------+----------------------------+
| Option                | Description                                                                   | Example                    |
+=======================+===============================================================================+============================+
| ``Endpoint``          | Ex. ``https://<login>:<password>@<mvip>/json-rpc/<element-version>``          |                            |
+-----------------------+-------------------------------------------------------------------------------+----------------------------+
| ``SVIP``              | iSCSI IP address and port                                                     | 10.0.0.7:3260              |
+-----------------------+-------------------------------------------------------------------------------+----------------------------+
| ``TenantName``        | SF Tenant to use (created if not found)                                       | "docker"                   |
+-----------------------+-------------------------------------------------------------------------------+----------------------------+
| ``InitiatorIFace``    | Specify interface when restricting iSCSI traffic to non-default interface     | "default"                  |
+-----------------------+-------------------------------------------------------------------------------+----------------------------+
| ``Types``             | QoS specifications                                                            | See below                  |
+-----------------------+-------------------------------------------------------------------------------+----------------------------+
| ``LegacyNamePrefix``  | Prefix for upgraded Trident installs                                          | "netappdvp-"               |
+-----------------------+-------------------------------------------------------------------------------+----------------------------+

The ``solidfire-san`` driver does not support Docker Swarm.

**LegacyNamePrefix** If you used a version of Trident prior to 1.3.2 and perform an
upgrade with existing volumes, you'll need to set this value in order to access
your old volumes that were mapped via the ``volume-name`` method.

Example Element Software Config File
------------------------------------

.. code-block:: json

  {
      "version": 1,
      "storageDriverName": "solidfire-san",
      "Endpoint": "https://admin:admin@192.168.160.3/json-rpc/8.0",
      "SVIP": "10.0.0.7:3260",
      "TenantName": "docker",
      "InitiatorIFace": "default",
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
