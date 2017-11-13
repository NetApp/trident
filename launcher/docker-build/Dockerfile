FROM alpine:3.6

LABEL maintainer="Ardalan.Kangarlou@netapp.com" \
      app="trident-launcher.netapp.io" \
      description="Trident Storage Orchestrator Launcher"

# for go binaries to work inside an alpine container
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

COPY launcher /usr/local/bin/launcher
ENTRYPOINT ["/usr/local/bin/launcher"]
