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
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
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

var driverMutex = sync.Mutex{}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (outRes *csi.NodePublishVolumeResponse, outErr error) {
	driverMutex.Lock()
	defer driverMutex.Unlock()
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

	if err := os.MkdirAll("/var/run/imager2/blobs", 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := os.MkdirAll("/var/run/imager2/images", 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := os.MkdirAll("/var/run/imager2/volumes", 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := os.MkdirAll("/var/run/imager2/rootfs", 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	cleanups := []func(){}
	defer func() {
		if outErr != nil {
			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
		}
	}()

	volumeID := req.GetVolumeId()

	uid := strings.Split(strings.TrimPrefix(req.GetTargetPath(), "/var/lib/kubelet/pods/"), "/")[0]
	podPath := fmt.Sprintf("/var/run/imager2/pods/%s", uid)
	if err := os.MkdirAll(podPath, 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	volumeMapPath := path.Join(podPath, volumeID)

	volumeName := req.GetVolumeContext()["name"]
	targetPath := path.Join(podPath, volumeName)

	err := os.Symlink(targetPath, volumeMapPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	cleanups = append(cleanups, func() {
		os.RemoveAll(volumeMapPath)
	})

	image := req.GetVolumeContext()["image"]
	imageURL := fmt.Sprintf("docker://%s", image)

	ociImagePath := fmt.Sprintf("/var/run/imager2/images/%s", volumeID)
	ociURL := fmt.Sprintf("oci:%s:img", ociImagePath)
	args := []string{"copy",
		"--src-shared-blob-dir", "/var/run/imager2/blobs/",
		"--dest-shared-blob-dir", "/var/run/imager2/blobs/",
		imageURL, ociURL}
	_, err = ns.runCmd("skopeo", args)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	cleanups = append(cleanups, func() {
		os.RemoveAll(ociImagePath)
	})

	err = os.Symlink("/var/run/imager2/blobs/sha256", path.Join(ociImagePath, "blobs/sha256"))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	digestBytes, err := ns.runCmd("skopeo", []string{"manifest-digest", path.Join(ociImagePath, "index.json")})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	digest := strings.TrimSpace(string(digestBytes))

	createdRootfs := false
	rootfsPath := fmt.Sprintf("/var/run/imager2/rootfs/%s", digest)
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		args = []string{"unpack",
			"--ref", "name=img",
			ociImagePath, rootfsPath}
		_, err = ns.runCmd("oci-image-tool", args)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		createdRootfs = true
		cleanups = append(cleanups, func() {
			os.RemoveAll(rootfsPath)
		})
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	volumePath := path.Join("/var/run/imager2/volumes", digest)
	if err := os.MkdirAll(volumePath, 0750); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if createdRootfs {
		cleanups = append(cleanups, func() {
			os.RemoveAll(volumePath)
		})
	}

	volumeRegistrationPath := path.Join(volumePath, volumeID)
	if f, err := os.Create(volumeRegistrationPath); err == nil {
		f.Close()
		cleanups = append(cleanups, func() {
			os.RemoveAll(volumeRegistrationPath)
		})
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = os.RemoveAll(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	err = os.Symlink(rootfsPath, targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	cleanups = append(cleanups, func() {
		os.RemoveAll(targetPath)
	})

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	driverMutex.Lock()
	defer driverMutex.Unlock()
	// Check arguments
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	volumeID := req.GetVolumeId()

	uid := strings.Split(strings.TrimPrefix(req.GetTargetPath(), "/var/lib/kubelet/pods/"), "/")[0]
	podPath := fmt.Sprintf("/var/run/imager2/pods/%s", uid)
	volumeMapPath := path.Join(podPath, volumeID)
	targetPath, err := os.Readlink(volumeMapPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = os.RemoveAll(targetPath)
	if os.IsNotExist(err) {
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	ociImagePath := fmt.Sprintf("/var/run/imager2/images/%s", volumeID)
	digestBytes, err := ns.runCmd("skopeo", []string{"manifest-digest", path.Join(ociImagePath, "index.json")})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	digest := strings.TrimSpace(string(digestBytes))

	volumePath := path.Join("/var/run/imager2/volumes", digest)
	volumeRegistrationPath := path.Join(volumePath, volumeID)
	err = os.RemoveAll(volumeRegistrationPath)
	if os.IsNotExist(err) {
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	dirEnts, err := ioutil.ReadDir(volumePath)
	if os.IsNotExist(err) {
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(dirEnts) == 0 {
		err := os.RemoveAll(volumePath)
		if os.IsNotExist(err) {
		} else if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		rootfsPath := fmt.Sprintf("/var/run/imager2/rootfs/%s", digest)
		err = os.RemoveAll(rootfsPath)
		if os.IsNotExist(err) {
		} else if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	err = os.RemoveAll(ociImagePath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = os.RemoveAll(volumeMapPath)
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
