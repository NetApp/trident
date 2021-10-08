################
What is Trident?
################

Trident is a fully supported `open source project`_ maintained by `NetApp`_. It
has been designed from the ground up to help you meet your containerized
applications' persistence demands using industry-standard interfaces, such as the
`Container Storage Interface (CSI)`_.

Trident deploys in Kubernetes clusters as pods and provides dynamic storage
orchestration services for your Kubernetes workloads. It enables your
containerized applications to quickly and easily consume persistent storage from
NetApp’s broad portfolio that includes `ONTAP`_ (AFF/FAS/Select/Cloud/Amazon FSx for NetApp ONTAP), `Element Software`_
(NetApp HCI/SolidFire), as
well as the `Azure NetApp Files`_ service, `Cloud Volumes Service on Google Cloud`_,
and the `Cloud Volumes Service on AWS`_.

Trident is also a foundational technology for NetApp's `Astra`_, which addresses
your data protection, disaster recovery, portability, and migration use cases
for Kubernetes workloads leveraging NetApp's industry-leading data management
technology for snapshots, backups, replication, and cloning.

Take a look at the `Astra documentation`_ to get started today.
See `NetApp's Support site`_ for details on Trident's support policy under the
`Trident's Release and Support Lifecycle`_ tab.

.. important::

  The Astra Trident documentation has a new home! Check out the content at `the new location on docs.netapp.com <https://docs.netapp.com/us-en/trident/index.html>`_. For all the latest updates, please visit the new location. We appreciate feedback on content and encourage users to take advantage of the “Request doc changes” function available on every page of the documentation. We also offer an embedded content contribution function for GitHub users using the "Edit on GitHub" option. You can edit, request a change, or even send feedback. Go to `NetApp Astra Trident documentation <https://docs.netapp.com/us-en/trident/index.html>`_ to begin.

.. _open source project: https://github.com/netapp/trident
.. _NetApp: https://www.netapp.com
.. _Kubernetes: https://kubernetes.io
.. _Docker: https://docker.com
.. _ONTAP: https://www.netapp.com/us/products/data-management-software/ontap.aspx
.. _Element Software: https://www.netapp.com/data-management/element-software?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _SANtricity: https://www.netapp.com/data-management/santricity?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _Azure NetApp Files: https://cloud.netapp.com/azure-netapp-files?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _Azure: https://azure.microsoft.com/
.. _Cloud Volumes Service on AWS: https://cloud.netapp.com/cloud-volumes-service-for-aws?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _Cloud Volumes Service on Google Cloud: https://cloud.netapp.com/cloud-volumes-service-for-gcp?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _Amazon Web Services: https://aws.amazon.com/
.. _Google Cloud: https://cloud.google.com/
.. _NetApp's Support site: https://mysupport.netapp.com/site/info/version-support
.. _Trident's Release and Support Lifecycle: https://mysupport.netapp.com/site/info/trident-support
.. _Container Storage Interface (CSI): https://kubernetes-csi.github.io/docs/introduction.html
.. _Astra: http://cloud.netapp.com/Astra?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _Astra documentation: https://docs.netapp.com/us-en/astra/
