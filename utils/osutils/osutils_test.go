// Copyright 2024 NetApp, Inc. All Rights Reserved.

package osutils

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	execCmd "github.com/netapp/trident/utils/exec"
)

// TestShellProcess is a method that is called as a substitute for a shell command,
// the GO_TEST flag ensures that if it is called as part of the test suite, it is
// skipped. GO_TEST_RETURN_VALUE flag allows the caller to specify a base64 encoded version of what should be returned via stdout,
// GO_TEST_RETURN_CODE flag allows the caller to specify what the return code should be, and
// GO_TEST_DELAY flag allows the caller to inject a delay before the function returns.
// GO_TEST_RETURN_PADDING_LENGTH flag allows the caller to specify how many bytes to return in total,
// if GO_TEST_RETURN_VALUE does not use all the bytes, the end is padded with NUL data
func TestShellProcess(t *testing.T) {
	if os.Getenv("GO_TEST") != "1" {
		return
	}
	// Print out the test value to stdout
	returnString, _ := b64.StdEncoding.DecodeString(os.Getenv("GO_TEST_RETURN_VALUE"))
	fmt.Fprint(os.Stdout, string(returnString))
	if os.Getenv("GO_TEST_RETURN_PADDING_LENGTH") != "" {
		padLength, _ := strconv.Atoi(os.Getenv("GO_TEST_RETURN_PADDING_LENGTH"))
		padString := make([]byte, padLength-len(returnString))
		fmt.Fprint(os.Stdout, string(padString))
	}
	code, err := strconv.Atoi(os.Getenv("GO_TEST_RETURN_CODE"))
	if err != nil {
		code = -1
	}
	// Pause for some amount of time
	delay, err := time.ParseDuration(os.Getenv("GO_TEST_DELAY"))
	if err == nil {
		time.Sleep(delay)
	}
	os.Exit(code)
}

func TestIsLikelyDir(t *testing.T) {
	fs := afero.NewMemMapFs()
	osUtils := NewDetailed(execCmd.NewCommand(), fs)

	testFilePath := "./deleteFileTest"
	testDirPath := "./deleteDirTest"
	fs.Create(testFilePath)
	fs.Mkdir(testDirPath, 0o755)

	// Test that a directory is correctly identified as a directory
	isDir, err := osUtils.IsLikelyDir("./")
	assert.True(t, isDir)
	assert.NoError(t, err)

	// Test that a file is correctly identified as not a directory
	isDir, err = osUtils.IsLikelyDir(testFilePath)
	assert.False(t, isDir)
	assert.NoError(t, err)

	// Test that a non-existent path is correctly identified as not a directory
	isDir, err = osUtils.IsLikelyDir("./nonexistent")
	assert.False(t, isDir)
	assert.Error(t, err)
}

func TestPathExists(t *testing.T) {
	fs := afero.NewMemMapFs()
	osUtils := NewDetailed(execCmd.NewCommand(), fs)

	testFilePath := "./deleteFileTest"
	testDirPath := "./deleteDirTest"
	fs.Create(testFilePath)
	fs.Mkdir(testDirPath, 0o755)

	// Test that a directory is correctly identified as existing
	exists, err := osUtils.PathExists(testDirPath)
	assert.True(t, exists)
	assert.NoError(t, err)

	// Test that a file is correctly identified as existing
	exists, err = osUtils.PathExists(testFilePath)
	assert.True(t, exists)
	assert.NoError(t, err)

	// Test that a non-existent path is correctly identified as not existing
	exists, err = osUtils.PathExists("./nonexistent")
	assert.False(t, exists)
	assert.NoError(t, err)
}

func TestEnsureFileExists(t *testing.T) {
	fs := afero.NewMemMapFs()
	osUtils := NewDetailed(execCmd.NewCommand(), fs)

	// File exists
	testFileName := "testfile"
	fs.Create(testFileName)
	err := osUtils.EnsureFileExists(context.TODO(), testFileName)
	assert.NoError(t, err)

	// Is directory
	dirName := "testdir"
	fs.Mkdir(dirName, 0o755)
	err = osUtils.EnsureFileExists(context.TODO(), dirName)
	assert.Error(t, err)

	// File does not exist
	err = osUtils.EnsureFileExists(context.TODO(), "./nonexistent")
	assert.NoError(t, err)
	// Ensure file was created
	_, err = fs.Stat("./nonexistent")
	assert.NoError(t, err)
}

func TestDeleteResourceAtPath(t *testing.T) {
	fs := afero.NewMemMapFs()
	osUtils := NewDetailed(execCmd.NewCommand(), fs)

	testFilePath := "./deleteFileTest"
	testDirPath := "./deleteDirTest"
	fs.Create(testFilePath)
	fs.Mkdir(testDirPath, 0o755)

	// File exists
	err := osUtils.DeleteResourceAtPath(context.TODO(), testFilePath)
	assert.NoError(t, err)
	_, err = os.Stat(testFilePath)
	assert.Error(t, err, "file should not exist")

	// Is directory
	err = osUtils.DeleteResourceAtPath(context.TODO(), testDirPath)
	assert.NoError(t, err)
	_, err = fs.Stat(testDirPath)
	assert.Error(t, err, "directory should not exist")

	// File does not exist
	err = osUtils.DeleteResourceAtPath(context.TODO(), testFilePath)
	assert.NoError(t, err)

	// Dir does not exist
	err = osUtils.DeleteResourceAtPath(context.TODO(), testDirPath)
	assert.NoError(t, err)
}

func TestWaitForResourceDeletionAtPath(t *testing.T) {
	fs := afero.NewMemMapFs()
	osUtils := NewDetailed(execCmd.NewCommand(), fs)

	testFilePath := "./deleteFileTest"
	testDirPath := "./deleteDirTest"
	fs.Create(testFilePath)
	fs.Mkdir(testDirPath, 0o755)

	// File exists
	err := osUtils.WaitForResourceDeletionAtPath(context.TODO(), testFilePath, 1*time.Second)
	assert.NoError(t, err)
	_, err = os.Stat(testFilePath)
	assert.Error(t, err, "file should not exist")

	// Is directory
	err = osUtils.WaitForResourceDeletionAtPath(context.TODO(), testDirPath, 1*time.Second)
	assert.NoError(t, err)
	_, err = fs.Stat(testDirPath)
	assert.Error(t, err, "directory should not exist")

	// File does not exist
	err = osUtils.WaitForResourceDeletionAtPath(context.TODO(), testFilePath, 1*time.Second)
	assert.NoError(t, err)

	// Dir does not exist
	err = osUtils.WaitForResourceDeletionAtPath(context.TODO(), testDirPath, 1*time.Second)
	assert.NoError(t, err)
}

func TestEnsureDirExists(t *testing.T) {
	fs := afero.NewMemMapFs()
	osUtils := NewDetailed(execCmd.NewCommand(), fs)

	testFilePath := "./deleteFileTest"
	testDirPath := "./deleteDirTest"
	fs.Create(testFilePath)
	fs.Mkdir(testDirPath, 0o755)

	// Directory exists
	err := osUtils.EnsureDirExists(context.TODO(), testDirPath)
	assert.NoError(t, err)

	// Is file
	err = osUtils.EnsureDirExists(context.TODO(), testFilePath)
	assert.Error(t, err)

	// Directory does not exist
	notExistsDir := "doesNotExist"
	err = osUtils.EnsureDirExists(context.TODO(), notExistsDir)
	assert.NoError(t, err)
	_, err = fs.Stat(notExistsDir)
	assert.NoError(t, err, "directory should exist")
}

func TestRunningInContainer(t *testing.T) {
	// Get env var
	value := os.Getenv("CSI_ENDPOINT")
	// Test that the function returns false when not running in a container
	inContainer := runningInContainer()
	if value == "" {
		assert.False(t, inContainer)
	} else {
		assert.True(t, inContainer)
	}
}
