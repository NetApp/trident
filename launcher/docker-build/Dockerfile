FROM debian:jessie

LABEL maintainer="Ardalan.Kangarlou@netapp.com" \
      app="trident-launcher.netapp.io" \
      description="Trident Storage Orchestrator Launcher"

COPY launcher /usr/local/bin/launcher
ENTRYPOINT ["/usr/local/bin/launcher"]
