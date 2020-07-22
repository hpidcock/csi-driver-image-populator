FROM golang:1.14 AS build

ENV GOPATH /root/go

WORKDIR /root/driver/
ADD . .
RUN go install github.com/kubernetes-csi/csi-driver-image-populator/cmd/imagepopulatorplugin

WORKDIR /root/
RUN go get -u github.com/opencontainers/image-tools/cmd/oci-image-tool

FROM ubuntu:18.04
LABEL maintainers="Kubernetes Authors"
LABEL description="Image Driver"

RUN \
  sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/xUbuntu_18.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list" && \
  wget -nv "https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/xUbuntu_18.04/Release.key" -O- | apt-key add - && \
  apt-get update -qq && \
  apt-get install --no-install-recommends --yes skopeo && \
  rm -rf /var/lib/apt/lists/*

COPY --from=build /root/go/bin/oci-image-tool /usr/local/bin/oci-image-tool
COPY --from=build /root/go/bin/imagepopulatorplugin /usr/local/bin/imagepopulatorplugin
ENTRYPOINT ["/usr/local/bin/imagepopulatorplugin"]
