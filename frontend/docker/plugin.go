// Copyright 2017 NetApp, Inc. All Rights Reserved.

package docker

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/core"
	"github.com/netapp/trident/storage"
)

type DockerPlugin struct {
	orchestrator core.Orchestrator
	driverName   string
	driverPort   string
	volumePath   string
	mutex        *sync.Mutex
}

func NewPlugin(driverName, driverPort string, orchestrator core.Orchestrator) (*DockerPlugin, error) {

	// Create the plugin object
	plugin := &DockerPlugin{
		orchestrator: orchestrator,
		driverName:   driverName,
		driverPort:   driverPort,
		volumePath:   filepath.Join(volume.DefaultDockerRootDirectory, driverName),
		mutex:        &sync.Mutex{},
	}

	// Register the plugin with Docker
	err := registerDockerVolumePlugin(plugin.volumePath)
	if err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"version":      dvp.DriverVersion,
		"mode":         dvp.ExtendedDriverVersion,
		"volumePath":   plugin.volumePath,
		"volumeDriver": driverName,
	}).Info("Initializing Trident plugin for Docker.")

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
		return fmt.Errorf("Volume directory '%v' exists and it's not a directory", root)
	}

	return nil
}

func (p *DockerPlugin) Activate() error {
	handler := volume.NewHandler(p)
	if p.driverPort != "" {
		go handler.ServeTCP(p.driverName, ":"+p.driverPort, "", &tls.Config{InsecureSkipVerify: true})
	} else {
		go handler.ServeUnix(p.driverName, 0) // 0 is the unix group to start as (root gid)
	}
	return nil
}

func (p *DockerPlugin) Deactivate() error {

	return nil
}

func (p *DockerPlugin) GetName() string {
	return pluginName
}

func (p *DockerPlugin) Version() string {
	return pluginVersion
}

func (p *DockerPlugin) Create(request *volume.CreateRequest) error {

	log.WithFields(log.Fields{
		"Method":  "Create",
		"Type":    "DockerPlugin",
		"name":    request.Name,
		"options": request.Options,
	}).Debug("Create")

	// Find a matching storage class, or register a new one
	scConfig, err := getStorageClass(request.Options, p.orchestrator)
	if err != nil {
		return err
	}

	// Convert volume creation options into a Trident volume config
	volConfig, err := getVolumeConfig(request.Name, scConfig.Name, request.Options)
	if err != nil {
		return err
	}

	// Invoke the orchestrator to create the new volume
	_, err = p.orchestrator.AddVolume(volConfig)
	return err
}

func (p *DockerPlugin) List() (*volume.ListResponse, error) {

	log.WithFields(log.Fields{
		"Method": "List",
		"Type":   "DockerPlugin",
	}).Debug("List")

	tridentVols := p.orchestrator.ListVolumes()
	var dockerVols []*volume.Volume

	for _, tridentVol := range tridentVols {
		dockerVol := &volume.Volume{Name: tridentVol.Config.Name}
		dockerVols = append(dockerVols, dockerVol)
	}

	return &volume.ListResponse{Volumes: dockerVols}, nil
}

func (p *DockerPlugin) Get(request *volume.GetRequest) (*volume.GetResponse, error) {

	log.WithFields(log.Fields{
		"Method": "Get",
		"Type":   "DockerPlugin",
		"name":   request.Name,
	}).Debug("Get")

	tridentVol := p.orchestrator.GetVolume(request.Name)
	if tridentVol == nil {
		return &volume.GetResponse{}, fmt.Errorf("Volume %s not found.", request.Name)
	}

	// Get the mountpoint, if this volume is mounted
	mountpoint, _ := p.getPath(tridentVol)
	status := map[string]interface{}{
		"Snapshots": make([]dvp.CommonSnapshot, 0),
	}

	vol := &volume.Volume{
		Name:       tridentVol.Config.Name,
		Mountpoint: mountpoint,
		Status:     status,
	}

	return &volume.GetResponse{Volume: vol}, nil
}

func (p *DockerPlugin) Remove(request *volume.RemoveRequest) error {

	log.WithFields(log.Fields{
		"Method": "Remove",
		"Type":   "DockerPlugin",
		"name":   request.Name,
	}).Debug("Remove")

	found, err := p.orchestrator.DeleteVolume(request.Name)
	if !found {
		log.WithField("volume", request.Name).Warn("Volume not found.")
	}
	return err
}

func (p *DockerPlugin) Path(request *volume.PathRequest) (*volume.PathResponse, error) {

	log.WithFields(log.Fields{
		"Method": "Path",
		"Type":   "DockerPlugin",
		"name":   request.Name,
	}).Debug("Path")

	tridentVol := p.orchestrator.GetVolume(request.Name)
	if tridentVol == nil {
		return &volume.PathResponse{}, fmt.Errorf("Volume %s not found.", request.Name)
	}

	mountpoint, err := p.getPath(tridentVol)
	if err != nil {
		return &volume.PathResponse{}, err
	}

	return &volume.PathResponse{Mountpoint: mountpoint}, nil
}

func (p *DockerPlugin) Mount(request *volume.MountRequest) (*volume.MountResponse, error) {

	log.WithFields(log.Fields{
		"Method": "Mount",
		"Type":   "DockerPlugin",
		"name":   request.Name,
		"id":     request.ID,
	}).Debug("Mount")

	tridentVol := p.orchestrator.GetVolume(request.Name)
	if tridentVol == nil {
		return &volume.MountResponse{}, fmt.Errorf("Volume %s not found.", request.Name)
	}

	mountpoint := p.mountpoint(tridentVol.Config.InternalName)
	options := make(map[string]string)

	err := p.orchestrator.AttachVolume(request.Name, mountpoint, options)
	if err != nil {
		log.Error(err)
		err = fmt.Errorf("Error attaching volume %v, mountpoint %v, error: %v", request.Name, mountpoint, err)
		return &volume.MountResponse{}, err
	}

	return &volume.MountResponse{Mountpoint: mountpoint}, nil
}

func (p *DockerPlugin) Unmount(request *volume.UnmountRequest) error {

	log.WithFields(log.Fields{
		"Method": "Unmount",
		"Type":   "DockerPlugin",
		"name":   request.Name,
		"id":     request.ID,
	}).Debug("Unmount")

	tridentVol := p.orchestrator.GetVolume(request.Name)
	if tridentVol == nil {
		return fmt.Errorf("Volume %s not found.", request.Name)
	}

	mountpoint := p.mountpoint(tridentVol.Config.InternalName)

	err := p.orchestrator.DetachVolume(request.Name, mountpoint)
	if err != nil {
		log.Error(err)
		return fmt.Errorf("Error detaching volume %v, mountpoint %v, error: %v", request.Name, mountpoint, err)
	}

	return nil
}

func (p *DockerPlugin) Capabilities() *volume.CapabilitiesResponse {

	log.WithFields(log.Fields{
		"Method": "Capabilities",
		"Type":   "DockerPlugin",
	}).Debug("Capabilities")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "global"}}
}

// getPath returns the mount point if the path exists.
func (p *DockerPlugin) getPath(vol *storage.VolumeExternal) (string, error) {

	mountpoint := p.mountpoint(vol.Config.InternalName)

	log.WithFields(log.Fields{
		"name":         vol.Config.Name,
		"internalName": vol.Config.InternalName,
		"mountpoint":   mountpoint,
	}).Debug("Getting path for volume.")

	fi, err := os.Lstat(mountpoint)
	if os.IsNotExist(err) {
		return "", err
	}
	if fi == nil {
		return "", fmt.Errorf("Could not stat %v", mountpoint)
	}

	return mountpoint, nil
}

func (p *DockerPlugin) mountpoint(name string) string {
	return filepath.Join(p.volumePath, name)
}
