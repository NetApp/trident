// Copyright 2020 NetApp, Inc. All Rights Reserved.

package docker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/docker/go-plugins-helpers/volume"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	frontendcommon "github.com/netapp/trident/frontend/common"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

const (
	startupTimeout = 50 * time.Second
)

// Plugin implements the frontendcommon Plugin interface
type Plugin struct {
	orchestrator       core.Orchestrator
	driverName         string
	driverPort         string
	volumePath         string
	version            *Version
	mutex              *sync.Mutex
	isDockerPluginMode bool
	hostVolumePath     string
}

func NewPlugin(driverName, driverPort string, orchestrator core.Orchestrator) (*Plugin, error) {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).Debug(">>>> docker.NewPlugin")
	defer Logc(ctx).Debug("<<<< docker.NewPlugin")

	isDockerPluginMode := false
	if os.Getenv(config.DockerPluginModeEnvVariable) != "" {
		isDockerPluginMode = true
	}

	// Create the plugin object
	plugin := &Plugin{
		orchestrator:       orchestrator,
		driverName:         driverName,
		driverPort:         driverPort,
		volumePath:         filepath.Join(volume.DefaultDockerRootDirectory, driverName),
		mutex:              &sync.Mutex{},
		isDockerPluginMode: isDockerPluginMode,
		hostVolumePath:     "",
	}

	if plugin.isDockerPluginMode {
		mountInfo, err := utils.GetMountInfo(ctx)
		if err != nil {
			return nil, err
		}

	mountInfoSearch:
		for _, procMount := range mountInfo {
			Logc(ctx).Debugf("root: %v, mountPoint: %v\n", procMount.Root, procMount.MountPoint)
			if procMount.MountPoint == volume.DefaultDockerRootDirectory+"/netapp" {
				plugin.hostVolumePath = filepath.Join(procMount.Root)
				Logc(ctx).WithFields(log.Fields{
					"plugin.volumePath":     plugin.volumePath,
					"plugin.hostVolumePath": plugin.hostVolumePath,
				}).Debugf("Running in Docker plugin mode.")
				break mountInfoSearch
			}
		}

		if plugin.hostVolumePath == "" {
			return nil, fmt.Errorf("could not find proc mount entry for %v", volume.DefaultDockerRootDirectory)
		}

	} else {

		// Register the plugin with Docker, needed in binary mode (non-plugin mode)
		err := registerDockerVolumePlugin(ctx, plugin.volumePath)
		if err != nil {
			return nil, err
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"volumePath":   plugin.volumePath,
		"volumeDriver": driverName,
	}).Info("Initializing Docker frontend.")

	return plugin, nil
}

func registerDockerVolumePlugin(ctx context.Context, root string) error {
	Logc(ctx).Debugf(">>>> docker.registerDockerVolumePlugin(%s)", root)
	defer Logc(ctx).Debugf("<<<< docker.registerDockerVolumePlugin(%s)", root)

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

func (p *Plugin) initDockerVersion() {
	time.Sleep(5 * time.Second)

	// Get Docker version
	out, err := exec.Command("docker", "version", "--format", "'{{json .}}'").CombinedOutput()
	if err != nil {
		log.Errorf("could not get Docker version: %v", err)
	}
	versionJSON := string(out)
	versionJSON = strings.TrimSpace(versionJSON)
	versionJSON = strings.TrimPrefix(versionJSON, "'")
	versionJSON = strings.TrimSuffix(versionJSON, "'")

	var version Version
	err = json.Unmarshal([]byte(versionJSON), &version)
	if err != nil {
		log.Errorf("could not parse Docker version: %v", err)
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

	p.version = &version

	// Configure telemetry
	config.OrchestratorTelemetry.Platform = string(config.PlatformDocker)
	config.OrchestratorTelemetry.PlatformVersion = p.Version()
}

func (p *Plugin) Activate() error {
	handler := volume.NewHandler(p)

	// Start serving requests on a different thread
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

	// Read the Docker version on a different thread so we don't deadlock if Docker is also initializing
	go p.initDockerVersion()

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
	}

	return p.version.Server.Version
}

func (p *Plugin) Create(request *volume.CreateRequest) error {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).WithFields(log.Fields{
		"method":  "Create",
		"name":    request.Name,
		"options": request.Options,
	}).Debug("Docker frontend method is invoked.")

	// Find a matching storage class, or register a new one
	scConfig, err := frontendcommon.GetStorageClass(ctx, request.Options, p.orchestrator)
	if err != nil {
		return p.dockerError(ctx, err)
	}

	sizeBytes, err := utils.GetVolumeSizeBytes(ctx, request.Options, "0")
	if err != nil {
		return fmt.Errorf("error creating volume: %v", err)
	}
	delete(request.Options, "size")

	// Convert volume creation options into a Trident volume config
	volConfig, err := frontendcommon.GetVolumeConfig(
		request.Name, scConfig.Name, int64(sizeBytes), request.Options, config.ProtocolAny, config.ModeAny, config.Filesystem)
	if err != nil {
		return p.dockerError(ctx, err)
	}

	// Invoke the orchestrator to create or clone the new volume
	if volConfig.CloneSourceVolume != "" {
		_, err = p.orchestrator.CloneVolume(ctx, volConfig)
	} else {
		_, err = p.orchestrator.AddVolume(ctx, volConfig)
	}
	return p.dockerError(ctx, err)
}

func (p *Plugin) List() (*volume.ListResponse, error) {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).WithFields(log.Fields{
		"method": "List",
	}).Debug("Docker frontend method is invoked.")

	err := p.reloadVolumes(ctx)
	if err != nil {
		return &volume.ListResponse{}, p.dockerError(ctx, err)
	}

	tridentVols, err := p.orchestrator.ListVolumes(ctx)
	if err != nil {
		return &volume.ListResponse{}, p.dockerError(ctx, err)
	}

	var dockerVols []*volume.Volume

	for _, tridentVol := range tridentVols {
		dockerVol := &volume.Volume{Name: tridentVol.Config.Name}
		dockerVols = append(dockerVols, dockerVol)
	}

	return &volume.ListResponse{Volumes: dockerVols}, nil
}

func (p *Plugin) Get(request *volume.GetRequest) (*volume.GetResponse, error) {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).WithFields(log.Fields{
		"method": "Get",
		"name":   request.Name,
	}).Debug("Docker frontend method is invoked")

	// Get is called at the start of every 'docker volume' workflow except List & Unmount,
	// so refresh the volume list here.
	err := p.reloadVolumes(ctx)
	if err != nil {
		return &volume.GetResponse{}, p.dockerError(ctx, err)
	}

	// Get the requested volume
	tridentVol, err := p.orchestrator.GetVolume(ctx, request.Name)
	if err != nil {
		return &volume.GetResponse{}, p.dockerError(ctx, err)
	}

	// Get the volume's snapshots and convert to struct Docker expects
	snapshots, err := p.orchestrator.ReadSnapshotsForVolume(ctx, request.Name)
	if err != nil {
		return &volume.GetResponse{}, p.dockerError(ctx, err)
	}
	dockerSnapshots := make([]*Snapshot, 0)
	for _, snapshot := range snapshots {
		dockerSnapshots = append(dockerSnapshots, &Snapshot{
			Name:    snapshot.Config.Name,
			Created: snapshot.Created,
		})
	}
	status := map[string]interface{}{
		"Snapshots": dockerSnapshots,
	}

	// Get the mountpoint, if this volume is mounted
	mountpoint, _ := p.getPath(ctx, tridentVol)

	vol := &volume.Volume{
		Name:       tridentVol.Config.Name,
		Mountpoint: mountpoint,
		Status:     status,
	}

	return &volume.GetResponse{Volume: vol}, nil
}

func (p *Plugin) Remove(request *volume.RemoveRequest) error {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).WithFields(log.Fields{
		"method": "Remove",
		"name":   request.Name,
	}).Debug("Docker frontend method is invoked.")

	err := p.orchestrator.DeleteVolume(ctx, request.Name)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volume": request.Name,
			"error":  err,
		}).Warn("Could not delete volume.")
	}
	return p.dockerError(ctx, err)
}

func (p *Plugin) Path(request *volume.PathRequest) (*volume.PathResponse, error) {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).WithFields(log.Fields{
		"method": "Path",
		"name":   request.Name,
	}).Debug("Docker frontend method is invoked.")

	tridentVol, err := p.orchestrator.GetVolume(ctx, request.Name)
	if err != nil {
		return &volume.PathResponse{}, p.dockerError(ctx, err)
	}

	mountpoint, err := p.getPath(ctx, tridentVol)
	if err != nil {
		return &volume.PathResponse{}, p.dockerError(ctx, err)
	}

	return &volume.PathResponse{Mountpoint: mountpoint}, nil
}

func (p *Plugin) Mount(request *volume.MountRequest) (*volume.MountResponse, error) {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).WithFields(log.Fields{
		"method": "Mount",
		"name":   request.Name,
		"id":     request.ID,
	}).Debug("Docker frontend method is invoked.")

	tridentVol, err := p.orchestrator.GetVolume(ctx, request.Name)
	if err != nil {
		return &volume.MountResponse{}, p.dockerError(ctx, err)
	}

	// First call PublishVolume to make the volume available to the node
	publishInfo := &utils.VolumePublishInfo{Localhost: true}
	if err = p.orchestrator.PublishVolume(ctx, request.Name, publishInfo); err != nil {
		err = fmt.Errorf("error publishing volume %s: %v", request.Name, err)
		Logc(ctx).Error(err)
		return &volume.MountResponse{}, p.dockerError(ctx, err)
	}

	// if this is binary mode, then hostMountpoint and mountpoint will be the same
	hostMountpoint := p.hostMountpoint(tridentVol.Config.InternalName)

	// Then call AttachVolume to discover/format/mount the volume on the node
	if err = p.orchestrator.AttachVolume(ctx, request.Name, hostMountpoint, publishInfo); err != nil {
		err = fmt.Errorf("error attaching volume %v, hostMountpoint %v, error: %v", request.Name, hostMountpoint, err)
		Logc(ctx).Error(err)
		return &volume.MountResponse{}, p.dockerError(ctx, err)
	}

	// if this is binary mode, then hostMountpoint and mountpoint will be the same
	mountpoint := p.mountpoint(tridentVol.Config.InternalName)
	return &volume.MountResponse{Mountpoint: mountpoint}, nil
}

func (p *Plugin) Unmount(request *volume.UnmountRequest) error {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).WithFields(log.Fields{
		"method": "Unmount",
		"name":   request.Name,
		"id":     request.ID,
	}).Debug("Docker frontend method is invoked.")

	tridentVol, err := p.orchestrator.GetVolume(ctx, request.Name)
	if err != nil {
		return p.dockerError(ctx, err)
	}

	// if this is binary mode, then hostMountpoint and mountpoint will be the same
	hostMountpoint := p.hostMountpoint(tridentVol.Config.InternalName)

	if err = p.orchestrator.DetachVolume(ctx, request.Name, hostMountpoint); err != nil {
		err = fmt.Errorf("error detaching volume %v, hostMountpoint %v, error: %v", request.Name, hostMountpoint, err)
		Logc(ctx).Error(err)
		return p.dockerError(ctx, err)
	}

	// No longer detaching and removing iSCSI session here because it was causing issues with 'docker cp'.
	// See https://github.com/moby/moby/issues/34665

	return nil
}

func (p *Plugin) Capabilities() *volume.CapabilitiesResponse {

	ctx := GenerateRequestContext(nil, "", ContextSourceDocker)

	Logc(ctx).WithFields(log.Fields{
		"method": "Capabilities",
	}).Debug("Docker frontend method is invoked.")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "global"}}
}

// getPath returns the mount point if the path exists.
func (p *Plugin) getPath(ctx context.Context, vol *storage.VolumeExternal) (string, error) {

	mountpoint := p.mountpoint(vol.Config.InternalName)

	Logc(ctx).WithFields(log.Fields{
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

// mountpoint differs from hostMountpoint when running in docker plugin container mode
func (p *Plugin) mountpoint(name string) string {
	return filepath.Join(p.volumePath, name)
}

// hostMountpoint TBD but it's where the plugin's /var/lib/docker-volumes/netapp lives on the host
func (p *Plugin) hostMountpoint(name string) string {
	if p.isDockerPluginMode {
		return filepath.Join(p.hostVolumePath, name)
	}
	return filepath.Join(p.volumePath, name)
}

func (p *Plugin) dockerError(ctx context.Context, err error) error {

	if err != nil {
		Logc(ctx).Errorf("Docker frontend method returning error: %v", err)
	}

	if utils.IsBootstrapError(err) {
		return fmt.Errorf("%s: use 'journalctl -fu docker' to learn more", err.Error())
	} else {
		return err
	}
}

// reloadVolumes instructs Trident core to refresh its cached volume info from its
// backend storage controller(s).  If Trident isn't ready, it will retry for nearly
// the Docker timeout of 60 seconds.  Otherwise, it returns immediately with any
// other error or nil if the operation succeeded.
func (p *Plugin) reloadVolumes(ctx context.Context) error {

	reloadVolumesFunc := func() error {

		err := p.orchestrator.ReloadVolumes(ctx)
		if err == nil {
			return nil
		} else if utils.IsNotReadyError(err) {
			return err
		} else {
			return backoff.Permanent(err)
		}
	}
	reloadNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Docker frontend waiting to reload volumes.")
	}
	reloadBackoff := backoff.NewExponentialBackOff()
	reloadBackoff.InitialInterval = 1 * time.Second
	reloadBackoff.RandomizationFactor = 0.0
	reloadBackoff.Multiplier = 1.0
	reloadBackoff.MaxInterval = 1 * time.Second
	reloadBackoff.MaxElapsedTime = startupTimeout

	return backoff.RetryNotify(reloadVolumesFunc, reloadBackoff, reloadNotify)
}
