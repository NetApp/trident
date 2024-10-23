// Copyright 2024 NetApp, Inc. All Rights Reserved.

package filepathwrapper

//go:generate mockgen -destination=../../../mocks/mock_utils/mock_mount/mock_filepathwrapper/mock_filepathwrapper.go github.com/netapp/trident/utils/mount/filepathwrapper FilePath

import "path/filepath"

type FilePath interface {
	EvalSymlinks(string) (string, error)
	Dir(string) string
}

type filePathWrapper struct{}

func (f *filePathWrapper) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func (f *filePathWrapper) Dir(path string) string {
	return filepath.Dir(path)
}

func New() FilePath {
	return &filePathWrapper{}
}
