// Copyright 2020 NetApp, Inc. All Rights Reserved.

//go:build linux

package cmd

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/zcalusic/sysinfo"
	"golang.org/x/sys/unix"
)

var (
	chrootPath string
)

func init() {
	RootCmd.AddCommand(systemCmd)
	systemCmd.Flags().StringVarP(&chrootPath, "chroot-path", "p", "", "Path to chroot to.")
}

var systemCmd = &cobra.Command{
	Use:    "system",
	Short:  "Get information about the current system",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		initLogging()

		// chroot if needed
		if chrootPath != "" {
			err := unix.Chroot(chrootPath)
			if err != nil {
				log.WithFields(log.Fields{
					"chrootPath": chrootPath,
					"err":        err,
				}).Error("Could not change root.")
				return err
			}
		}

		// discover system OS info
		var si sysinfo.SysInfo
		si.GetSysInfo()
		data, err := json.MarshalIndent(&si.OS, "", "  ")
		if err != nil {
			log.WithError(err).Error("Could not retrieve system info.")
			return err
		}
		fmt.Println(string(data))

		return nil
	},
}
