// Copyright 2022 NetApp, Inc. All Rights Reserved.

package filesystem

//go:generate mockgen -destination=../../mocks/mock_utils/mock_filesystem/mock_json_utils.go github.com/netapp/trident/utils/filesystem JSONReaderWriter

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/afero"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

type JSONReaderWriter interface {
	WriteJSONFile(ctx context.Context, fileContents interface{}, filepath, fileDescription string) error
	ReadJSONFile(ctx context.Context, fileContents interface{}, filepath, fileDescription string) error
}

type jsonReaderWriter struct {
	osFs afero.Fs
}

func NewJSONReaderWriter(osFs afero.Fs) JSONReaderWriter {
	return &jsonReaderWriter{
		osFs: osFs,
	}
}

// WriteJSONFile writes the contents of any type of struct to a file, with logging.
func (j jsonReaderWriter) WriteJSONFile(
	ctx context.Context, fileContents interface{}, filepath, fileDescription string,
) error {
	file, err := j.osFs.OpenFile(filepath, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if err = json.NewEncoder(file).Encode(fileContents); err != nil {
		Logc(ctx).WithFields(LogFields{
			"filename": filepath,
			"error":    err.Error(),
		}).Error(fmt.Sprintf("Unable to write %s file.", fileDescription))
		return err
	}

	return nil
}

// ReadJSONFile reads a file at the specified path and deserializes its contents into the provided fileContents var.
// fileContents must be a pointer to a struct, not a pointer type!
func (j *jsonReaderWriter) ReadJSONFile(
	ctx context.Context, fileContents interface{}, filepath, fileDescription string,
) error {
	file, err := j.osFs.Open(filepath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			Logc(ctx).WithFields(LogFields{
				"filepath": filepath,
				"error":    err.Error(),
			}).Warningf("Could not find JSON file: %s.", filepath)
			return errors.NotFoundError(err.Error())
		}
		return err
	}
	defer func() { _ = file.Close() }()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}
	// We do not consider an empty file valid JSON.
	if fileInfo.Size() == 0 {
		return errors.InvalidJSONError("file was empty, which is not considered valid JSON")
	}

	err = json.NewDecoder(file).Decode(fileContents)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"filename": filepath,
			"error":    err.Error(),
		}).Error(fmt.Sprintf("Could not parse %s file.", fileDescription))

		e, _ := errors.AsInvalidJSONError(err)
		return e
	}

	return nil
}
