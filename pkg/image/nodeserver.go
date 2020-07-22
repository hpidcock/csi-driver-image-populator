/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package image

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	deviceIDKey = "deviceID"
)

var (
	TimeoutError = fmt.Errorf("Timeout")
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	Timeout  time.Duration
	execPath string
	args     []string
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	// Check arguments
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	targetPath := req.GetTargetPath()
	if err := os.MkdirAll(targetPath, 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := os.MkdirAll("/var/lib/kubelet/plugins/csi-juju-image/blobs", 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := os.MkdirAll("/var/lib/kubelet/plugins/csi-juju-image/volumes", 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	volumeID := req.GetVolumeId()

	image := req.GetVolumeContext()["image"]
	imageURL := fmt.Sprintf("docker://%s", image)

	// ref, err := reference.Parse(image)
	// if err != nil {
	// 	return nil, status.Error(codes.Internal, err.Error())
	// }
	// switch typedRef := ref.(type) {
	// case reference.Digested:
	// case reference.Tagged:
	// }

	ociImagePath := fmt.Sprintf("/var/lib/kubelet/plugins/csi-juju-image/volumes/%s", volumeID)
	ociURL := fmt.Sprintf("oci:%s:img", ociImagePath)
	args := []string{"copy",
		"--src-shared-blob-dir", "/var/lib/kubelet/plugins/csi-juju-image/blobs/",
		"--dest-shared-blob-dir", "/var/lib/kubelet/plugins/csi-juju-image/blobs/",
		imageURL, ociURL}
	_, err := ns.runCmd("skopeo", args)
	defer os.RemoveAll(ociImagePath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	err = os.Symlink("/var/lib/kubelet/plugins/csi-juju-image/blobs/sha256", path.Join(ociImagePath, "blobs/sha256"))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	args = []string{"unpack",
		"--ref", "name=img",
		ociImagePath, targetPath}
	_, err = ns.runCmd("oci-image-tool", args)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	// Check arguments
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	targetPath := req.GetTargetPath()

	err := os.RemoveAll(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) runCmd(execPath string, args []string) ([]byte, error) {
	cmd := exec.Command(execPath, args...)

	timeout := false
	if ns.Timeout > 0 {
		timer := time.AfterFunc(ns.Timeout, func() {
			timeout = true
			// TODO: cmd.Stop()
		})
		defer timer.Stop()
	}

	output, execErr := cmd.CombinedOutput()
	if execErr != nil {
		if timeout {
			return nil, TimeoutError
		}
	}
	return output, execErr
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}
