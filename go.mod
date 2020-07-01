module github.com/kubernetes-csi/csi-driver-image-populator

go 1.14

require (
	github.com/container-storage-interface/spec v1.0.0
	github.com/docker/distribution v2.7.1+incompatible
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.2.0
	github.com/google/uuid v1.0.0
	github.com/kubernetes-csi/drivers v1.0.0
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709
	golang.org/x/net v0.0.0-20181113165502-88d92db4c548
	golang.org/x/sys v0.0.0-20181107165924-66b7b1311ac8
	golang.org/x/text v0.3.0
	google.golang.org/genproto v0.0.0-20181109154231-b5d43981345b
	google.golang.org/grpc v1.16.0
	k8s.io/apimachinery v0.0.0-20181110190943-2a7c93004028
	k8s.io/kubernetes v1.12.2
	k8s.io/utils v0.0.0-20181102055113-1bd4f387aa67
)
