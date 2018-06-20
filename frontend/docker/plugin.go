// Copyright 2018 NetApp, Inc. All Rights Reserved.

package docker

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/go-plugins-helpers/volume"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	frontendcommon "github.com/netapp/trident/frontend/common"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

type Plugin struct {
	orchestrator core.Orchestrator
	driverName   string
	driverPort   string
	volumePath   string
	version      *Version
	mutex        *sync.Mutex
}

func NewPlugin(driverName, driverPort string, orchestrator core.Orchestrator) (*Plugin, error) {

	// Get the Docker version
	version, err := getDockerVersion()
	if err != nil {
		log.Errorf("Failed to get the Docker version: %v", err)
	} else {
		log.WithField("version", version.Server.Version).Info("Docker server version.")
	}

	// Create the plugin object
	plugin := &Plugin{
		orchestrator: orchestrator,
		driverName:   driverName,
		driverPort:   driverPort,
		volumePath:   filepath.Join(volume.DefaultDockerRootDirectory, driverName),
		version:      version,
		mutex:        &sync.Mutex{},
	}

	// Register the plugin with Docker
	err = registerDockerVolumePlugin(plugin.volumePath)
	if err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"volumePath":   plugin.volumePath,
		"volumeDriver": driverName,
	}).Info("Initializing Docker frontend.")

	return plugin, nil
}

func registerDockerVolumePlugin(root string) error {

	// If root (volumeDir) doesn't exist, make it.
	dir, err := os.Lstat(root)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(root, 0755); err != nil {
			return err
		}
	}
	// If root (volumeDir) isn't a directory, error
	if dir != nil && !dir.IsDir() {
		return fmt.Errorf("volume directory '%v' exists and it's not a directory", root)
	}

	return nil
}

func getDockerVersion() (*Version, error) {

	// Get Docker version
	out, err := exec.Command("docker", "version", "--format", "'{{json .}}'").CombinedOutput()
	if err != nil {
		return nil, err
	}
	versionJSON := string(out)
	versionJSON = strings.TrimSpace(versionJSON)
	versionJSON = strings.TrimPrefix(versionJSON, "'")
	versionJSON = strings.TrimSuffix(versionJSON, "'")

	var version Version
	err = json.Unmarshal([]byte(versionJSON), &version)
	if err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"serverVersion":    version.Server.Version,
		"serverAPIVersion": version.Server.APIVersion,
		"serverArch":       version.Server.Arch,
		"serverOS":         version.Server.Os,
		"clientVersion":    version.Server.Version,
		"clientAPIVersion": version.Server.APIVersion,
		"clientArch":       version.Server.Arch,
		"clientOS":         version.Server.Os,
	}).Debug("Docker version info.")

	return &version, nil
}

func (p *Plugin) Activate() error {
	handler := volume.NewHandler(p)
	go func() {
		var err error
		if p.driverPort != "" {
			log.WithFields(log.Fields{
				"driverName": p.driverName,
				"driverPort": p.driverPort,
				"volumePath": p.volumePath,
			}).Info("Activating Docker frontend.")
			err = handler.ServeTCP(p.driverName, ":"+p.driverPort, "",
				&tls.Config{InsecureSkipVerify: true})
		} else {
			log.WithFields(log.Fields{
				"driverName": p.driverName,
				"volumePath": p.volumePath,
			}).Info("Activating Docker frontend.")
			err = handler.ServeUnix(p.driverName, 0) // start as root unix group
		}
		if err != nil {
			log.Fatalf("Failed to activate Docker frontend: %v", err)
		}
	}()
	return nil
}

func (p *Plugin) Deactivate() error {
	log.Info("Deactivating Docker frontend.")
	return nil
}

func (p *Plugin) GetName() string {
	return string(config.ContextDocker)
}

func (p *Plugin) Version() string {
	if p.version == nil {
		return "unknown"
	} else {
		return p.version.Server.Version
	}
}

func (p *Plugin) Create(request *volume.CreateRequest) error {

	log.WithFields(log.Fields{
		"method":  "Create",
		"name":    request.Name,
		"options": request.Options,
	}).Debug("Docker frontend method is invoked.")

	// Find a matching storage class, or register a new one
	scConfig, err := frontendcommon.GetStorageClass(request.Options, p.orchestrator)
	if err != nil {
		return p.dockerError(err)
	}

	sizeBytes, err := utils.GetVolumeSizeBytes(request.Options, "0")
	if err != nil {
		return fmt.Errorf("error creating volume: %v", err)
	}
	delete(request.Options, "size")

	// Convert volume creation options into a Trident volume config
	volConfig, err := frontendcommon.GetVolumeConfig(
		request.Name, scConfig.Name, int64(sizeBytes), request.Options, config.ProtocolAny, config.ModeAny)
	if err != nil {
		return p.dockerError(err)
	}

	// Invoke the orchestrator to create or clone the new volume
	if volConfig.CloneSourceVolume != "" {
		_, err = p.orchestrator.CloneVolume(volConfig)
	} else {
		_, err = p.orchestrator.AddVolume(volConfig)
	}
	return p.dockerError(err)
}

func (p *Plugin) List() (*volume.ListResponse, error) {

	log.WithFields(log.Fields{
		"method": "List",
	}).Debug("Docker frontend method is invoked.")

	err := p.orchestrator.ReloadVolumes()
	if err != nil {
		return &volume.ListResponse{}, p.dockerError(err)
	}

	tridentVols, err := p.orchestrator.ListVolumes()
	if err != nil {
		return &volume.ListResponse{}, p.dockerError(err)
	}

	var dockerVols []*volume.Volume

	for _, tridentVol := range tridentVols {
		dockerVol := &volume.Volume{Name: tridentVol.Config.Name}
		dockerVols = append(dockerVols, dockerVol)
	}

	return &volume.ListResponse{Volumes: dockerVols}, nil
}

func (p *Plugin) Get(request *volume.GetRequest) (*volume.GetResponse, error) {

	log.WithFields(log.Fields{
		"method": "Get",
		"name":   request.Name,
	}).Debug("Docker frontend method is invoked")

	// Get is called at the start of every 'docker volume' workflow except List & Unmount,
	// so refresh the volume list here.
	err := p.orchestrator.ReloadVolumes()
	if err != nil {
		return &volume.GetResponse{}, p.dockerError(err)
	}

	// Get the requested volume
	tridentVol, err := p.orchestrator.GetVolume(request.Name)
	if err != nil {
		return &volume.GetResponse{}, p.dockerError(err)
	}

	// Get the volume's snapshots
	snapshots, err := p.orchestrator.ListVolumeSnapshots(request.Name)
	if err != nil {
		return &volume.GetResponse{}, p.dockerError(err)
	}
	status := map[string]interface{}{
		"Snapshots": snapshots,
	}

	// Get the mountpoint, if this volume is mounted
	mountpoint, _ := p.getPath(tridentVol)

	vol := &volume.Volume{
		Name:       tridentVol.Config.Name,
		Mountpoint: mountpoint,
		Status:     status,
	}

	return &volume.GetResponse{Volume: vol}, nil
}

func (p *Plugin) Remove(request *volume.RemoveRequest) error {

	log.WithFields(log.Fields{
		"method": "Remove",
		"name":   request.Name,
	}).Debug("Docker frontend method is invoked.")

	err := p.orchestrator.DeleteVolume(request.Name)
	if err != nil {
		log.WithFields(log.Fields{
			"volume": request.Name,
			"error":  err,
		}).Warn("Could not delete volume.")
	}
	return p.dockerError(err)
}

func (p *Plugin) Path(request *volume.PathRequest) (*volume.PathResponse, error) {

	log.WithFields(log.Fields{
		"method": "Path",
		"name":   request.Name,
	}).Debug("Docker frontend method is invoked.")

	tridentVol, err := p.orchestrator.GetVolume(request.Name)
	if err != nil {
		return &volume.PathResponse{}, p.dockerError(err)
	}

	mountpoint, err := p.getPath(tridentVol)
	if err != nil {
		return &volume.PathResponse{}, p.dockerError(err)
	}

	return &volume.PathResponse{Mountpoint: mountpoint}, nil
}

func (p *Plugin) Mount(request *volume.MountRequest) (*volume.MountResponse, error) {

	log.WithFields(log.Fields{
		"method": "Mount",
		"name":   request.Name,
		"id":     request.ID,
	}).Debug("Docker frontend method is invoked.")

	tridentVol, err := p.orchestrator.GetVolume(request.Name)
	if err != nil {
		return &volume.MountResponse{}, p.dockerError(err)
	}

	mountpoint := p.mountpoint(tridentVol.Config.InternalName)

	// First call PublishVolume to make the volume available to the node
	publishInfo := &utils.VolumePublishInfo{Localhost: true}
	if err = p.orchestrator.PublishVolume(request.Name, publishInfo); err != nil {
		err = fmt.Errorf("error publishing volume %s: %v", request.Name, err)
		log.Error(err)
		return &volume.MountResponse{}, p.dockerError(err)
	}

	// Then call AttachVolume to discover/format/mount the volume on the node
	if err = p.orchestrator.AttachVolume(request.Name, mountpoint, publishInfo); err != nil {
		err = fmt.Errorf("error attaching volume %v, mountpoint %v, error: %v", request.Name, mountpoint, err)
		log.Error(err)
		return &volume.MountResponse{}, p.dockerError(err)
	}

	return &volume.MountResponse{Mountpoint: mountpoint}, nil
}

func (p *Plugin) Unmount(request *volume.UnmountRequest) error {

	log.WithFields(log.Fields{
		"method": "Unmount",
		"name":   request.Name,
		"id":     request.ID,
	}).Debug("Docker frontend method is invoked.")

	tridentVol, err := p.orchestrator.GetVolume(request.Name)
	if err != nil {
		return p.dockerError(err)
	}

	mountpoint := p.mountpoint(tridentVol.Config.InternalName)

	if err = p.orchestrator.DetachVolume(request.Name, mountpoint); err != nil {
		err = fmt.Errorf("error detaching volume %v, mountpoint %v, error: %v", request.Name, mountpoint, err)
		log.Error(err)
		return p.dockerError(err)
	}

	// No longer detaching and removing iSCSI session here because it was causing issues with 'docker cp'.
	// See https://github.com/moby/moby/issues/34665

	return nil
}

func (p *Plugin) Capabilities() *volume.CapabilitiesResponse {

	log.WithFields(log.Fields{
		"method": "Capabilities",
	}).Debug("Docker frontend method is invoked.")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "global"}}
}

// getPath returns the mount point if the path exists.
func (p *Plugin) getPath(vol *storage.VolumeExternal) (string, error) {

	mountpoint := p.mountpoint(vol.Config.InternalName)

	log.WithFields(log.Fields{
		"name":         vol.Config.Name,
		"internalName": vol.Config.InternalName,
		"mountpoint":   mountpoint,
	}).Debug("Getting path for volume.")

	fileInfo, err := os.Lstat(mountpoint)
	if os.IsNotExist(err) {
		return "", err
	}
	if fileInfo == nil {
		return "", fmt.Errorf("could not stat %v", mountpoint)
	}

	return mountpoint, nil
}

func (p *Plugin) mountpoint(name string) string {
	return filepath.Join(p.volumePath, name)
}

func (p *Plugin) dockerError(err error) error {
	if berr, ok := err.(*core.BootstrapError); ok {
		return fmt.Errorf("%s: use 'journalctl -fu docker' to learn more", berr.Error())
	} else {
		return err
	}
}
