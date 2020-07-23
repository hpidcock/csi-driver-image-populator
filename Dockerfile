FROM golang:1.14 AS build

ENV GOPATH /root/go

WORKDIR /root/
RUN go get -u github.com/opencontainers/image-tools/cmd/oci-image-tool
RUN go get -u github.com/containerd/containerd/cmd/ctr

WORKDIR /root/driver/
ADD . .
RUN go install github.com/kubernetes-csi/csi-driver-image-populator/cmd/imagepopulatorplugin

FROM ubuntu:18.04
LABEL maintainers="Kubernetes Authors"
LABEL description="Image Driver"

RUN \
  apt-get update -qq && \
  apt-get install --no-install-recommends --yes curl gnupg ca-certificates && \
  sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/xUbuntu_18.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list" && \
  curl -L -o - "https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/xUbuntu_18.04/Release.key" | apt-key add - && \
  apt-get update -qq && \
  apt-get install --no-install-recommends --yes skopeo && \
  apt-get purge -y --auto-remove curl gnupg && \
  rm -rf /var/lib/apt/lists/*

COPY --from=build /root/go/bin/oci-image-tool /usr/local/bin/oci-image-tool
COPY --from=build /root/go/bin/ctr /usr/local/bin/ctr
COPY --from=build /root/go/bin/imagepopulatorplugin /usr/local/bin/imagepopulatorplugin
ENTRYPOINT ["/usr/local/bin/imagepopulatorplugin"]
