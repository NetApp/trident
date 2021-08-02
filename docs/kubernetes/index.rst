Trident for Kubernetes
----------------------

Trident integrates natively with `Kubernetes`_ and its
`Persistent Volume framework`_ to seamlessly provision and manage volumes
from systems running any combination of NetApp's
`ONTAP`_ (AFF/FAS/Select/Cloud),
`Element Software`_ (NetApp HCI/SolidFire),
plus our
`Azure NetApp Files`_ service in Azure,
`Cloud Volumes Service for AWS`_, and
`Cloud Volumes Service for GCP`_.

Relative to other Kubernetes provisioners, Trident is novel in the following
respects:

1. It is the first out-of-tree, out-of-process storage provisioner that
works by watching events at the Kubernetes API Server, affording it levels of
visibility and flexibility that cannot otherwise be achieved.

2. It is capable of orchestrating across multiple platforms at the same time
through a unified interface. Rather than tying a request for a persistent
volume to a particular system, Trident selects one from those it manages based
on the higher-level qualities that the user is looking for in their volume.

Trident tightly integrates with Kubernetes to allow your users to request and
manage persistent volumes using native Kubernetes interfaces and constructs.
It's designed to work in such a way that your users can take advantage of the
underlying capabilities of your storage infrastructure without having to know
anything about it.

It automates the rest for you, the Kubernetes administrator, based on policies
that you define.

A great way to get a feel for what we're trying to accomplish is to see Trident
in action from the `perspective of an end user`_. This is a great demonstration
of Kubernetes volume consumption when Trident is in the mix, through the lens
of Red Hat's OpenShift platform, which is itself built on Kubernetes. All of
the concepts in the video apply to any Kubernetes deployment.

While some details about Trident and NetApp storage systems are shown in the
video to help you see what's going on behind-the-scenes, in standard
deployments Trident and the rest of the infrastructure is completely hidden
from the user.

Let's lift up the covers a bit to better understand Trident and what it is
doing. This `introductory video`_ is a good place to start. While based on
an earlier version of Trident, it explains the core concepts involved
and is a great way to get started on the Trident journey.

.. .. raw:: html
  <!--
  <font face="Menlo" size="-1">
  <pre style="line-height:0.9em;">
  ┌───────────────────────────────────────────────────────────────────────────┐
  │kubernetes (cluster)                                                       │
  │                        ╔═════════════════════════════════════════════════╗│
  │                        ║trident (deployment)                             ║│
  │                        ╚═════════════════════════════════════════════════╝│
  │┌─────────────────┐     ┌─────────────────────────┐    ┌──────────────────┐│
  ││master(s)        │     │worker (node 1)          │    │worker (node 2)   ││
  ││┌──────────────┐ │     │                         │    │                  ││
  │││kube-api      │ │     │┌───────────────────────┐│    │┌────────────────┐││
  │││              ├─┼───┐ ││trident (pod)          ││    ││app (pod)       │││
  │││              │ │   │ ││ ┌───────────────────┐ ││    ││                │││
  ││└──────────────┘ │   │ ││ │trident-main       │ ││    ││                │││
  ││                 │   │ ││ │(container)        │ ││    │├────────────────┤││
  ││                 │   │ ││ │┌─────────────────┐│ ││    ││volume (pv)     │││
  ││                 │   └─┼┼─┼┤    frontend     ││ ││    │└────────┬───────┘││
  ││                 │     ││ │└─────────────────┘│ ││    │         │        ││
  ││                 │     ││ │┌─────────────────┐│ ││    │         │        ││
  ││                 │   ┌─┼┼─┼┤    backends     ││ ││    │         │        ││
  ││                 │   │ ││ │└─────────────────┘│ ││    │         │        ││
  ││                 │   │ ││ └───────────────────┘ ││    │         │        ││
  ││                 │   │ ││ ┌── ─── ─── ─── ─── ┐ ││    │         │        ││
  ││                 │   │ ││ │etcd (container)   │ ││    │         │        ││
  ││                 │   │ ││ │                   │ ││    │     NFS/iSCSI    ││
  ││                 │   │ ││  ─── ─── ─── ─── ───  ││    │         │        ││
  ││                 │   │ │└───────────────────────┘│    │         │        ││
  ││                 │Storage                        │    │         │        ││
  ││                 │  API│                         │    │         │        ││
  │└─────────────────┘   │ └─────────────────────────┘    └─────────┼────────┘│
  │                      │                                          │         │
  └──────────────────────┼──────────────────────────────────────────┼─────────┘
  ┌──────────────────────┼──────────────────────────────────────────┼─────────┐
  │             ┌────────┴────────┐                       ┌─────────┴────────┐│
  │             │control          │                       │data (NFS/iSCSI)  ││
  │             │                 │  storage              │                  ││
  │             │                 │                       │                  ││
  │             └─────────────────┘                       └──────────────────┘│
  └───────────────────────────────────────────────────────────────────────────┘
  </pre>
  </font>
  -->


.. toctree::
    :maxdepth: 2
    :caption: Contents:

    upgrades/index
    deploying/index
    operations/tasks/index
    concepts/index
    known-issues
    troubleshooting

.. _Azure NetApp Files: https://cloud.netapp.com/azure-netapp-files
.. _Kubernetes: https://kubernetes.io
.. _Persistent Volume framework: https://kubernetes.io/docs/concepts/storage/persistent-volumes/
.. _ONTAP: https://www.netapp.com/us/products/data-management-software/ontap.aspx
.. _Element Software: https://www.netapp.com/us/products/data-management-software/element-os.aspx
.. _Cloud Volumes Service for AWS: https://cloud.netapp.com/cloud-volumes-service-for-aws?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _Cloud Volumes Service for GCP: https://cloud.netapp.com/cloud-volumes-service-for-gcp?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _perspective of an end user: https://youtu.be/WZ3nwl4aILU
.. _introductory video: https://youtu.be/NbhR81peqF8
