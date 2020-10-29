##################
Monitoring Trident
##################

Trident provides a set of Prometheus metrics endpoints that you can use to
monitor Trident's performance and understand how it functions. The metrics
exposed by Trident provide a convenient way of:

1. Keeping tabs on Trident's health and configuration. You can examine how
   successful Trident operations are and if it can communicate with the backends
   as expected.
2. Examining backend usage information and understanding how many volumes are
   provisioned on a backend and the amount of space consumed and so on.
3. Maintaining a mapping of the amount of volumes provisioned on available
   backends.
4. Tracking Trident's performance metrics. You can take a look at how long it
   takes to communicate to backends and perform operations.

Requirements
------------

1. A Kubernetes cluster with Trident installed. By default, Trident's metrics
   are exposed on the target port ``8001`` at the ``/metrics`` endpoint.
   These metrics are enabled by default when Trident is installed.
2. A Prometheus instance. This can be a containerized Prometheus deployment such
   as the `Prometheus Operator <https://github.com/coreos/prometheus-operator>`_
   or you could choose to run Prometheus as a
   `native application <https://prometheus.io/download/>`_.

Once these requirements are satisfied, you can define a Prometheus target to gather
the metrics exposed by Trident and obtain information on the backends it manages,
the volumes it creates and so on. As stated above, Trident reports its metrics by
default; to **disable them** from being reported, you will have to generate
custom YAMLs (using the ``--generate-custom-yaml`` flag) and edit them to
remove the ``--metrics`` flag from being invoked for the ``trident-main``
container.

This `blog <https://netapp.io/2020/02/20/prometheus-and-trident/>`_ is a great
place to start. It explains how Prometheus and Grafana can
be used with Trident to retrieve metrics. The blog explains how you
can run Prometheus as an operator in your Kubernetes cluster and the creation of a
ServiceMonitor to obtain Trident's metrics.

.. note::

  Metrics that are part of the Trident ``core`` subsystem are deprecated and
  marked for removal in a later release.

Scraping Trident metrics
------------------------

After installing Trident with the ``--metrics`` flag (done by default), Trident
will return its Prometheus metrics at the ``metrics`` port that is defined in
the ``trident-csi`` service. To consume them, you will need to create a
Prometheus ServiceMonitor that watches the ``trident-csi`` service and listens on
the ``metrics`` port. A sample ServiceMonitor looks like this.

.. code-block:: yaml

    apiVersion: monitoring.coreos.com/v1
    kind: ServiceMonitor
    metadata:
      name: trident-sm
      namespace: monitoring
      labels:
        release: prom-operator
    spec:
      jobLabel: trident
      selector:
        matchLabels:
          app: controller.csi.trident.netapp.io
      namespaceSelector:
        matchNames:
        - trident
      endpoints:
      - port: metrics
        interval: 15s

This ServiceMonitor definition will retrieve metrics returned by the
``trident-csi`` service and specifically looks for the ``metrics`` endpoint of
the service. As a result, Prometheus is now configured to understand Trident's
metrics. This can now be extended to work on the metrics returned by creating
PromQL queries or creating custom Grafana dashboards. PromQL is good for creating
expressions that return time-series or tabular data. With Grafana it is
easier to create visually descriptive dashboards that are completely
customizable.

In addition to metrics available directly from Trident, kubelet will expose many
``kubelet_volume_*`` metrics via it's own metrics endpoint. Kubelet can provide
information about the volumes that are attached, pods and other internal
operations it handles. You can find more information on it
`here <https://kubernetes.io/docs/concepts/cluster-administration/monitoring/>`_.


Querying Trident metrics with PromQL
------------------------------------

Here are some PromQL queries that you can use:

Trident Health Information
~~~~~~~~~~~~~~~~~~~~~~~~~~

**Percentage of HTTP 2XX responses from Trident**

.. code-block:: bash

    (sum (trident_rest_ops_seconds_total_count{status_code=~"2.."} OR on() vector(0)) / sum (trident_rest_ops_seconds_total_count)) * 100

**Percentage of REST responses from Trident via status code**

.. code-block:: bash

    (sum (trident_rest_ops_seconds_total_count) by (status_code)  / scalar (sum (trident_rest_ops_seconds_total_count))) * 100

**Average duration in ms of operations performed by Trident**

.. code-block:: bash

    sum by (operation) (trident_operation_duration_milliseconds_sum{success="true"}) / sum by (operation) (trident_operation_duration_milliseconds_count{success="true"})

Trident Usage Information
~~~~~~~~~~~~~~~~~~~~~~~~~

**Average volume size**

.. code-block:: bash

    trident_volume_allocated_bytes/trident_volume_count

**Total volume space provisioned by each backend**

.. code-block:: bash

    sum (trident_volume_allocated_bytes) by (backend_uuid)

Individual volume usage
~~~~~~~~~~~~~~~~~~~~~~~

.. note::

   This is only enabled if kubelet metrics are also gathered

**Percentage of used space for each volume**

.. code-block:: bash

    kubelet_volume_stats_used_bytes / kubelet_volume_stats_capacity_bytes * 100


Trident Autosupport Telemetry
-----------------------------

By default, Trident will send Prometheus metrics and basic backend information
to NetApp on a daily cadence. This behavior can be disabled during Trident
installation by passing the --silence-autosupport flag. In addition, Trident
can also send Trident container logs along with everything mentioned above
to NetApp support on-demand via ``tridentctl send autosupport``. Users will
always need to trigger Trident to upload it's logs. Unless specified, Trident
will fetch the logs from the past 24 hours. Users can specify the log retention
timeframe with the ``--since`` flag, e.g: ``tridentctl send autosupport --since=1h``.
Submitting the logs will require users to accept NetApp's
`privacy policy <https://www.netapp.com/company/legal/privacy-policy/>`_.


This information is collected and sent via a ``trident-autosupport`` container
that is installed alongside Trident. You can obtain the container image at
`netapp/trident-autosupport <https://hub.docker.com/r/netapp/trident-autosupport>`_
Trident Autosupport does not gather or transmit Personally Identifiable Information
(PII) or Personal Information.
It comes with a `EULA <https://www.netapp.com/us/media/enduser-license-agreement-worldwide.pdf>`_
that is not applicable to the `Trident <https://hub.docker.com/r/netapp/trident>`_
container image itself. You can learn more about NetApp's commitment to data
security and trust `here <https://www.netapp.com/us/company/trust-center/index.aspx,>`_.

An example payload sent by Trident looks like this:

.. code-block:: json

    {
      "items": [
        {
          "backendUUID": "ff3852e1-18a5-4df4-b2d3-f59f829627ed",
          "protocol": "file",
          "config": {
            "version": 1,
            "storageDriverName": "ontap-nas",
            "debug": false,
            "debugTraceFlags": null,
            "disableDelete": false,
            "serialNumbers": [
              "nwkvzfanek_SN"
            ],
            "limitVolumeSize": ""
          },
          "state": "online",
          "online": true
        }
      ]
    }

The Autosupport messages are sent to NetApp's Autosupport endpoint. If you are
using a private registry to store container images the ``--image-registry`` flag
can be used. Proxy URLs can also be configured by generating the installation
YAML files. This can be done by using ``tridentctl install --generate-custom-yaml``
to create the YAML files and adding the ``--proxy-url`` argument for the
``trident-autosupport`` container in ``trident-deployment.yaml``.
