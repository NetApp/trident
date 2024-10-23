// Copyright 2024 NetApp, Inc. All Rights Reserved.

package oswrapper

//go:generate mockgen -destination=../../../mocks/mock_utils/mock_mount/mock_oswrapper/mock_oswrapper.go github.com/netapp/trident/utils/mount/oswrapper OS

import "os"

type OS interface {
	Stat(name string) (os.FileInfo, error)
	Lstat(name string) (os.FileInfo, error)
	Remove(name string) error
	ReadFile(filename string) ([]byte, error)
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	MkdirAll(path string, perm os.FileMode) error
}

type osWrapper struct{}

func (o *osWrapper) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (o *osWrapper) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (o *osWrapper) Remove(name string) error {
	return os.Remove(name)
}

func (o *osWrapper) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (o *osWrapper) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func (o *osWrapper) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func New() OS {
	return &osWrapper{}
}
