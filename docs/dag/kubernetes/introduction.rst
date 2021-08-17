.. _introduction:

************
Introduction
************

Containers have quickly become one of the most popular methods of packaging and deploying applications.  The ecosystem surrounding the creation, deployment, and management of containerized applications has exploded, resulting in myriad solutions available to customers who simply want to deploy their applications with as little friction as possible.

Application teams love containers due to their ability to decouple the application from the underlying operating system.  The ability to create a container on their own laptop, then deploy to a teammate's laptop, their on-premises data center, hyperscalars, and anywhere else means that they can focus their efforts on the application and its code, not on how the underlying operating system and infrastructure are configured.

At the same time, operations teams are only just now seeing the dramatic rise in popularity of containers.  Containers are often approached first by developers for personal productivity purposes, which means the infrastructure teams are insulated from or unaware of their use.  However, this is changing.  Operations teams are now expected to deploy, maintain, and support infrastructures which host containerized applications.  In addition, the rise of DevOps is pushing operations teams to understand not just the application, but the deployment method and platform at a much greater depth than ever before.

Fortunately there are robust platforms for hosting containerized applications.  Arguably the most popular of those platforms is `Kubernetes <https://kubernetes.io/>`_, an open source `Cloud Native Computing Foundation (CNCF) <https://www.cncf.io/>`_ project, which orchestrates the deployment of containers, including connecting them to network and storage resources as needed.

Deploying an application using containers doesn't change its fundamental resource requirements.  Reading, writing, accessing, and storing data doesn't change just because a container technology is now a part of the stack in addition to virtual and/or physical machines.

To facilitate the consumption of storage resources by containerized applications,
`NetApp <https://www.netapp.com/>`_ created and released an open source project
known as `Trident <https://github.com/netapp/trident/v21>`_.  Trident is a storage
orchestrator which integrates with Docker and Kubernetes, as well as platforms
built on those technologies, such as `Red Hat OpenShift <https://www.openshift.com/>`_,
`Rancher <https://rancher.com/>`_, and `IBM Cloud Private <https://www.ibm.com/cloud/private>`_.
The goal of Trident is to make the provisioning, connection, and consumption of storage
as transparent and frictionless for applications as possible; while operating within the
constraints put forth by the storage administrator.

To achieve this goal, Trident automates the storage management tasks needed to consume storage for the storage administrator, the Kubernetes and Docker administrators, and the application consumers.  Trident fills a critical role for storage administrators, who may be feeling pressure from application teams to provide storage resources in ways which have not previously been expected.  Modern applications, and just as importantly modern development practices, have changed the storage consumption model, where resources are created, consumed, and destroyed quickly.  According to `DataDog <https://www.datadoghq.com/docker-adoption/#8>`_, containers have a median lifespan of just six days. This is dramatically different than storage resources for traditional applications, which commonly exist for years.  Those which are deployed using container orchestrators have an even shorter lifespan of just a half day.  Trident is the tool which storage administrators can rely on to safely, within the bounds given to it, provision the storage resources applications need, when they need them, and where they need them. 

Target Audience
===============

This document outlines the design and architecture considerations that should be evaluated when deploying containerized applications with persistence requirements within your organization. Additionally, you can find best practices for configuring Kubernetes and OpenShift with Trident.  

It is assumed that you, the reader, have a basic understanding of containers, Kubernetes, and storage prior to reading this document.  We will, however, explore and explain some of the concepts which are important to integrating Trident, and through it NetApp's storage platforms and services, with Kubernetes.  Unless noted, Kubernetes and OpenShift can be used interchangeably in this document.

As with all best practices, these are suggestions based on the experience and knowledge of the NetApp team.  Each should be considered according to your environment and targeted applications.
