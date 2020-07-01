FROM golang:1.14 AS build

ENV GOPATH /root/go

WORKDIR /root/driver/
ADD . .
RUN go install github.com/kubernetes-csi/csi-driver-image-populator/cmd/imagepopulatorplugin

WORKDIR /root/
RUN go get -u github.com/opencontainers/image-tools/cmd/oci-image-tool

FROM centos:centos7
LABEL maintainers="Kubernetes Authors"
LABEL description="Image Driver"

RUN \
  yum install -y epel-release && \
  yum install -y skopeo && \
  yum clean all

COPY --from=build /root/go/bin/oci-image-tool /usr/local/bin/oci-image-tool
COPY --from=build /root/go/bin/imagepopulatorplugin /usr/local/bin/imagepopulatorplugin
ENTRYPOINT ["/usr/local/bin/imagepopulatorplugin"]

