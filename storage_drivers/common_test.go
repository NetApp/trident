// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/semaphore"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func TestMain(m *testing.M) {
	// Disable any standard log output.
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestValidateCommonSettings(t *testing.T) {
	type output struct {
		config        *CommonStorageDriverConfig
		errorExpected bool
	}

	type test struct {
		configJSON string
		output     output
	}

	// Need to escape quotes because the raw storage prefix will be converted to a string.
	prefix := "\"trident_\""
	storagePrefixRaw := json.RawMessage(prefix)
	storagePrefix := string(storagePrefixRaw[1 : len(storagePrefixRaw)-1])

	tests := map[string]test{
		"fails to unmarshal the config when JSON field is wrong type": {
			configJSON: `{
				"version": "1"
			}`,
			output: output{
				config:        nil,
				errorExpected: true,
			},
		},
		"fails when storage driver name isn't specified": {
			configJSON: `{
				"version": 1,
				"storageDriverName": ""
			}`,
			output: output{
				config:        nil,
				errorExpected: true,
			},
		},
		"fails when the driver version is not equal to the config version": {
			configJSON: `{
				"version": 2,
				"storageDriverName": "ontap-nas"
			}`,
			output: output{
				config:        nil,
				errorExpected: true,
			},
		},
		"fails when raw storage prefix is of invalid type": {
			configJSON: `{
				"version": 1,
				"storageDriverName": "ontap-nas",
				"storagePrefix": true
			}`,
			output: output{
				config:        nil,
				errorExpected: true,
			},
		},
		"succeeds but storage prefix is not specified": {
			configJSON: `{
				"version": 1,
				"storageDriverName": "ontap-nas"
			}`,
			output: output{
				config: &CommonStorageDriverConfig{
					Version:           1,
					StorageDriverName: "ontap-nas",
					StoragePrefixRaw:  nil,
					StoragePrefix:     nil,
					Flags:             make(map[string]string),
				},
				errorExpected: false,
			},
		},
		"fails when limitVolumeSize is invalid": {
			configJSON: `{
				"version": 1,
				"storageDriverName": "ontap-nas",
				"storagePrefix": "trident_",
				"limitVolumeSize": "Gi"
			}`,
			output: output{
				config:        nil,
				errorExpected: true,
			},
		},
		"fails when invalid credentials are specified": {
			configJSON: `{
				"version": 1,
				"storageDriverName": "ontap-nas",
				"storagePrefix": "trident_",
				"credentials": {
					"name": "", "type": "KMIP"
				}
			}`,
			output: output{
				config:        nil,
				errorExpected: true,
			},
		},
		"succeeds when entire config is valid": {
			configJSON: `{
				"version": 1,
				"storageDriverName": "ontap-nas",
				"storagePrefix": "trident_",
				"disableDelete": true,
				"debug": true,
				"credentials": {
					"name": "secret1",
					"type": "secret"
				}
			}`,
			output: output{
				config: &CommonStorageDriverConfig{
					Version:           1,
					StorageDriverName: "ontap-nas",
					Debug:             true,
					DisableDelete:     true,
					StoragePrefixRaw:  storagePrefixRaw,
					StoragePrefix:     &storagePrefix,
					Credentials: map[string]string{
						"name": "secret1",
						"type": "secret",
					},
					Flags: make(map[string]string),
				},
				errorExpected: false,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.TODO()
			actual, err := ValidateCommonSettings(ctx, test.configJSON)
			if test.output.errorExpected {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
				assert.EqualValues(t, test.output.config, actual)
			}
		})
	}
}

func TestParseRawStoragePrefix(t *testing.T) {
	validStoragePrefix := "trident_"
	emptyStoragePrefix := ""

	type test struct {
		storagePrefixRaw json.RawMessage
		expectedPrefix   *string
		errorExpected    bool
	}

	tests := map[string]test{
		"returns nil pointer with valid but empty storagePrefixRaw bytes": {
			storagePrefixRaw: json.RawMessage("{}"),
			expectedPrefix:   nil,
			errorExpected:    false,
		},
		"returns pointer to empty string with no storagePrefixRaw string": {
			storagePrefixRaw: json.RawMessage(""),
			expectedPrefix:   nil, // Use the address of an uninitialized string will default to "".
			errorExpected:    false,
		},
		"returns pointer to empty string with empty but valid storagePrefixRaw string": {
			storagePrefixRaw: json.RawMessage("\"\""),
			expectedPrefix:   &emptyStoragePrefix, // Use the address of an uninitialized string will default to "".
			errorExpected:    false,
		},
		"returns storage prefix specified in storagePrefixRaw string": {
			storagePrefixRaw: json.RawMessage("\"trident_\""),
			expectedPrefix:   &validStoragePrefix,
			errorExpected:    false,
		},
		"returns nil pointer with error": {
			storagePrefixRaw: json.RawMessage("."),
			expectedPrefix:   nil,
			errorExpected:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.TODO()
			storagePrefix, err := parseRawStoragePrefix(ctx, test.storagePrefixRaw)

			if test.errorExpected {
				assert.Nil(t, storagePrefix)
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if test.expectedPrefix != nil && storagePrefix != nil {
					assert.Equal(t, *test.expectedPrefix, *storagePrefix)
				} else {
					assert.Equal(t, test.expectedPrefix, storagePrefix)
				}
			}
		})
	}
}

func TestGetDefaultStoragePrefix(t *testing.T) {
	tests := []struct {
		context  config.DriverContext
		expected string
	}{
		{
			context:  "",
			expected: "",
		},
		{
			context:  config.ContextCSI,
			expected: DefaultTridentStoragePrefix,
		},
		{
			context:  config.ContextDocker,
			expected: DefaultDockerStoragePrefix,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			actual := GetDefaultStoragePrefix(test.context)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGetDefaultIgroupName(t *testing.T) {
	tests := []struct {
		context  config.DriverContext
		expected string
	}{
		{
			context:  "",
			expected: DefaultTridentIgroupName,
		},
		{
			context:  config.ContextCSI,
			expected: DefaultTridentIgroupName,
		},
		{
			context:  config.ContextDocker,
			expected: DefaultDockerIgroupName,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			actual := GetDefaultIgroupName(test.context)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestSanitizeCommonStorageDriverConfig(t *testing.T) {
	config := &CommonStorageDriverConfig{
		StoragePrefixRaw: nil,
	}
	SanitizeCommonStorageDriverConfig(config)
	assert.NotEmpty(t, config.StoragePrefixRaw)
}

func TestGetCommonInternalVolumeName(t *testing.T) {
	const name = "volume"
	for _, test := range []struct {
		prefix   *string
		expected string
	}{
		{
			prefix:   &[]string{"specific"}[0],
			expected: fmt.Sprintf("specific-%s", name),
		},
		{
			prefix:   &[]string{""}[0],
			expected: name,
		},
		{
			prefix:   nil,
			expected: fmt.Sprintf("%s-%s", config.OrchestratorName, name),
		},
	} {
		c := CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "fake",
			StoragePrefix:     test.prefix,
		}
		got := GetCommonInternalVolumeName(&c, name)
		if test.expected != got {
			t.Errorf("Mismatch between volume names.  Expected %s, got %s",
				test.expected, got)
		}
	}
}

func TestCheckVolumeSizeLimits(t *testing.T) {
	ctx := context.TODO()
	requestedSize := uint64(1073741824) // 1Gi

	// Returns because LimitVolumeSize not specified.
	config := &CommonStorageDriverConfig{
		LimitVolumeSize: "",
	}
	shouldLimit, sizeLimit, err := CheckVolumeSizeLimits(ctx, requestedSize, config)
	assert.False(t, shouldLimit, "expected should limit to be false")
	assert.Zero(t, sizeLimit, "expected zero size limit")
	assert.Nil(t, err, "expected nil error")

	// Errors when LimitVolumeSize is not empty but invalid and cannot be parsed.
	config = &CommonStorageDriverConfig{
		LimitVolumeSize: "Gi",
	}
	shouldLimit, sizeLimit, err = CheckVolumeSizeLimits(ctx, requestedSize, config)
	assert.False(t, shouldLimit, "expected should limit to be false")
	assert.Zero(t, sizeLimit, "expected zero size limit")
	assert.NotNil(t, err, "expected non-nil error")

	requestedSize = uint64(2000000000)
	config = &CommonStorageDriverConfig{
		LimitVolumeSize: "1Gi",
	}
	shouldLimit, sizeLimit, err = CheckVolumeSizeLimits(ctx, requestedSize, config)
	assert.True(t, shouldLimit, "expected should limit to be true")
	assert.Equal(t, sizeLimit, uint64(1073741824), "expected size limit of 1Gi")
	assert.NotNil(t, err, "expected non-nil error")

	requestedSize = uint64(1000000000)
	config = &CommonStorageDriverConfig{
		LimitVolumeSize: "1Gi",
	}
	shouldLimit, sizeLimit, err = CheckVolumeSizeLimits(ctx, requestedSize, config)
	assert.True(t, shouldLimit, "expected should limit to be true")
	assert.Equal(t, sizeLimit, uint64(1073741824), "expected size limit of 1Gi")
	assert.Nil(t, err, "expected nil error")
}

func TestCheckMinVolumeSize(t *testing.T) {
	tests := []struct {
		requestedSizeBytes  uint64
		minimumVolSizeBytes uint64
		expected            error
	}{
		{
			requestedSizeBytes:  1000000000,
			minimumVolSizeBytes: 999999999,
			expected:            nil,
		},
		{
			requestedSizeBytes:  1000000000,
			minimumVolSizeBytes: 1000000000,
			expected:            nil,
		},
		{
			requestedSizeBytes:  1000000000,
			minimumVolSizeBytes: 1000000001,
			expected:            errors.UnsupportedCapacityRangeError(fmt.Errorf("test")),
		},
		{
			requestedSizeBytes:  1000000000,
			minimumVolSizeBytes: 1000000001,
			expected: fmt.Errorf("wrapping the UnsuppportedCapacityError; %w", errors.UnsupportedCapacityRangeError(
				fmt.Errorf("test"))),
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("CheckMinimumVolSize: %d", i), func(t *testing.T) {
			actualErr := CheckMinVolumeSize(test.requestedSizeBytes, test.minimumVolSizeBytes)
			actualIsUnsupportedCapError, _ := errors.HasUnsupportedCapacityRangeError(actualErr)
			expectedIsUnsupportedCapError, _ := errors.HasUnsupportedCapacityRangeError(test.expected)
			assert.Equal(t, actualIsUnsupportedCapError, expectedIsUnsupportedCapError)
		})
	}
}

func TestCalculateVolumeSizeBytes(t *testing.T) {
	ctx := context.TODO()

	tests := []struct {
		requestedSizeBytes uint64
		snapshotReserve    int
		expectedSizeBytes  uint64
	}{
		{
			requestedSizeBytes: 1000000000,
			snapshotReserve:    0,
			expectedSizeBytes:  1000000000,
		},
		{
			requestedSizeBytes: 1000000000,
			snapshotReserve:    10,
			expectedSizeBytes:  1111111111,
		},
		{
			requestedSizeBytes: 1000000000,
			snapshotReserve:    50,
			expectedSizeBytes:  2000000000,
		},
		{
			requestedSizeBytes: 1000000000,
			snapshotReserve:    90,
			expectedSizeBytes:  10000000000,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("CalculateVolumeSizeBytes: %d", i), func(t *testing.T) {
			actual := CalculateVolumeSizeBytes(ctx, "", test.requestedSizeBytes, test.snapshotReserve)
			assert.Equal(t, test.expectedSizeBytes, actual, "incorrect volume size")
		})
	}
}

func TestClone(t *testing.T) {
	type test struct {
		source      interface{}
		destination interface{}
		shouldEqual bool
	}

	tests := map[string]test{
		"succeeds when destination is a pointer type": {
			source: &CommonStorageDriverConfigDefaults{
				Size: "100Gi",
			},
			destination: &CommonStorageDriverConfigDefaults{},
			shouldEqual: true,
		},
		"fails when destination is a non-pointer type": {
			source:      "anything",
			destination: "",
			shouldEqual: false,
		},
		"fails when source is nil": {
			source:      nil,
			destination: "",
			shouldEqual: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			Clone(context.TODO(), test.source, test.destination)
			if test.shouldEqual {
				assert.EqualValues(t, test.source, test.destination)
			} else {
				assert.NotEqual(t, test.source, test.destination)
			}
		})
	}
}

func TestCheckSupportedFilesystem(t *testing.T) {
	ctx := context.TODO()
	validFilesystem := "xfs"
	invalidFilesystem := "ext2"

	fsType, err := CheckSupportedFilesystem(ctx, validFilesystem, "")
	assert.Equal(t, validFilesystem, fsType)
	assert.Nil(t, err, "expected nil error")

	fsType, err = CheckSupportedFilesystem(ctx, invalidFilesystem, "")
	assert.Empty(t, "")
	assert.NotNil(t, err, "expected non-nil error")

	fsType, err = CheckSupportedFilesystem(ctx, "", "")
	assert.Empty(t, "")
	assert.NotNil(t, err, "expected non-nil error")
}

func TestAreSameCredentials(t *testing.T) {
	type Credentials struct {
		Credential1 map[string]string
		Credential2 map[string]string
		Same        bool
	}

	inputs := []Credentials{
		{
			map[string]string{"name": "secret1", "type": "secret"},
			map[string]string{"name": "secret1", "type": "secret"},
			true,
		},
		{
			map[string]string{"name": "secret1", "type": "secret"},
			map[string]string{"name": "secret1"},
			true,
		},
		{
			map[string]string{"name": "secret1", "type": "secret"},
			map[string]string{"name": "secret1", "type": "random"},
			false,
		},
		{
			map[string]string{"name": "secret1"},
			map[string]string{"name": "secret1", "type": "random"},
			false,
		},
		{
			map[string]string{"name": "", "type": "secret", "randomKey": "randomValue"},
			map[string]string{"name": "", "type": "secret", "randomKey": "randomValue"},
			false,
		},
	}

	for _, input := range inputs {
		areEqual := AreSameCredentials(input.Credential1, input.Credential2)
		assert.Equal(t, areEqual, input.Same)
	}
}

func TestEnsureMountOption(t *testing.T) {
	// This is an exported method for ensureJoinedStringContainsElem so tests are similar.
	tests := []struct {
		mountOptions string
		option       string
		sep          string
		expected     string
	}{
		{
			mountOptions: "",
			option:       "def",
			expected:     "def",
		},
		{
			mountOptions: "abc",
			option:       "",
			expected:     "abc",
		},
		{
			mountOptions: "abc",
			option:       "efg",
			expected:     "abc,efg",
		},
		{
			mountOptions: "abc,def",
			option:       "efg",
			expected:     "abc,def,efg",
		},
		{
			mountOptions: "def g",
			option:       "efg",
			expected:     "def g,efg",
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			actual := EnsureMountOption(test.mountOptions, test.option)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestEnsureJoinedStringContainsElem(t *testing.T) {
	tests := []struct {
		joined   string
		elem     string
		sep      string
		expected string
	}{
		{
			elem:     "abc",
			sep:      ",",
			expected: "abc",
		},
		{
			joined:   "abc,def",
			elem:     "efg",
			sep:      ",",
			expected: "abc,def,efg",
		},
		{
			joined:   "def",
			elem:     "abc",
			sep:      ".",
			expected: "def.abc",
		},
		{
			joined:   "defabc|123",
			elem:     "abc",
			sep:      "|",
			expected: "defabc|123",
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			actual := ensureJoinedStringContainsElem(test.joined, test.elem, test.sep)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestEncodeAndDecode_OntapFlexGroupStorageBackendPools(t *testing.T) {
	ctx := context.Background()
	config := &CommonStorageDriverConfig{
		StorageDriverName: "test storage driver",
		DebugTraceFlags:   map[string]bool{"method": true},
	}
	backendPools := []OntapFlexGroupStorageBackendPool{{SvmUUID: "svm0"}}

	encoded, err := EncodeStorageBackendPools[OntapFlexGroupStorageBackendPool](ctx, config, backendPools)
	assert.NoError(t, err)
	assert.True(t, len(backendPools) == len(encoded))

	// Passing the type of the backend pools is required for DecodeStorageBackendPools.
	decoded, err := DecodeStorageBackendPools[OntapFlexGroupStorageBackendPool](ctx, config, encoded)
	assert.NoError(t, err)
	assert.EqualValues(t, backendPools, decoded)
}

func TestEncodeAndDecode_OntapStorageBackendPools(t *testing.T) {
	ctx := context.Background()
	config := &CommonStorageDriverConfig{
		StorageDriverName: "test storage driver",
		DebugTraceFlags:   map[string]bool{"method": true},
	}

	backendPools := []OntapStorageBackendPool{
		{SvmUUID: "svm0", Aggregate: "aggr0"},
		{SvmUUID: "svm0", Aggregate: "aggr1"},
	}

	encoded, err := EncodeStorageBackendPools[OntapStorageBackendPool](ctx, config, backendPools)
	assert.NoError(t, err)
	assert.True(t, len(backendPools) == len(encoded))

	// Passing the type of the backend pools is required for DecodeStorageBackendPools.
	decoded, err := DecodeStorageBackendPools[OntapStorageBackendPool](ctx, config, encoded)
	assert.NoError(t, err)
	assert.EqualValues(t, backendPools, decoded)
}

func TestEncodeAndDecode_OntapEconomyStorageBackendPools(t *testing.T) {
	ctx := context.Background()
	config := &CommonStorageDriverConfig{
		StorageDriverName: "test storage driver",
		DebugTraceFlags:   map[string]bool{"method": true},
	}

	backendPools := []OntapEconomyStorageBackendPool{
		{SvmUUID: "svm0", Aggregate: "aggr0", FlexVolPrefix: "trident_qtree_pool_test_"},
		{SvmUUID: "svm0", Aggregate: "aggr1", FlexVolPrefix: "trident_qtree_pool_test_"},
	}

	encoded, err := EncodeStorageBackendPools[OntapEconomyStorageBackendPool](ctx, config, backendPools)
	assert.NoError(t, err)
	assert.True(t, len(backendPools) == len(encoded))

	// Passing the type of the backend pools is required for DecodeStorageBackendPools.
	decoded, err := DecodeStorageBackendPools[OntapEconomyStorageBackendPool](ctx, config, encoded)
	assert.NoError(t, err)
	assert.EqualValues(t, backendPools, decoded)
}

func TestEncodeStorageBackendPools_FailsWithInvalidBackendPools(t *testing.T) {
	ctx := context.Background()
	config := &CommonStorageDriverConfig{
		StorageDriverName: "test storage driver",
		DebugTraceFlags:   map[string]bool{"method": true},
	}

	// Backend pools are nil.
	encodedPools, err := EncodeStorageBackendPools[OntapStorageBackendPool](ctx, config, nil)
	assert.Error(t, err)
	assert.Nil(t, encodedPools)

	// Backend pools are empty.
	encodedPools, err = EncodeStorageBackendPools[OntapStorageBackendPool](ctx, config, []OntapStorageBackendPool{})
	assert.Error(t, err)
	assert.Nil(t, encodedPools)
}

func TestDecodeStorageBackendPools_FailsWithInvalidEncodedPools(t *testing.T) {
	ctx := context.Background()
	config := &CommonStorageDriverConfig{
		StorageDriverName: "test storage driver",
		DebugTraceFlags:   map[string]bool{"method": true},
	}

	// Backend pools are nil.
	backendPools, err := DecodeStorageBackendPools[OntapStorageBackendPool](ctx, config, nil)
	assert.Error(t, err)
	assert.Nil(t, backendPools)

	// Backend pools are empty.
	backendPools, err = DecodeStorageBackendPools[OntapStorageBackendPool](ctx, config, []string{})
	assert.Error(t, err)
	assert.Nil(t, backendPools)

	// Backend pools specified are not valid base64 encoded strings.
	backendPools, err = DecodeStorageBackendPools[OntapStorageBackendPool](ctx, config, []string{"test", ""})
	assert.Error(t, err)
	assert.Nil(t, backendPools)
}

type TestTransport func(*http.Request) (*http.Response, error)

func (t TestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t(req)
}

func TestLimitedRetryTransport(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		tr := NewLimitedRetryTransport(semaphore.NewWeighted(1), TestTransport(func(req *http.Request) (*http.Response, error) {
			return nil, nil
		}))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
		assert.NoError(t, err)
		_, err = tr.RoundTrip(req)
		assert.NotNil(t, err)
		assert.ErrorContains(t, err, "context canceled")
	})
	t.Run("times out with EOF", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		tr := NewLimitedRetryTransport(semaphore.NewWeighted(1), TestTransport(func(req *http.Request) (*http.Response, error) {
			return nil, io.EOF
		}))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
		assert.NoError(t, err)
		_, err = tr.RoundTrip(req)
		assert.NotNil(t, err)
		assert.ErrorContains(t, err, "deadline exceeded")
	})
	t.Run("permanent failure", func(t *testing.T) {
		tr := NewLimitedRetryTransport(semaphore.NewWeighted(1), TestTransport(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("permanent failure")
		}))
		req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
		assert.NoError(t, err)
		_, err = tr.RoundTrip(req)
		assert.NotNil(t, err)
		assert.ErrorContains(t, err, "permanent failure")
	})
	t.Run("succeeds after failure", func(t *testing.T) {
		attempts := 0
		tr := NewLimitedRetryTransport(semaphore.NewWeighted(1), TestTransport(func(req *http.Request) (*http.Response, error) {
			if attempts < 3 {
				attempts++
				return nil, io.EOF
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}))
		req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
		assert.NoError(t, err)
		resp, err := tr.RoundTrip(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "ok", string(body))
	})
}

// TestSemaphore tests that New and Free won't cause deadlocks, and at the end everything is cleaned up
func TestSemaphore(t *testing.T) {
	count := 100
	sems := 3
	wg := sync.WaitGroup{}
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := strconv.FormatInt(int64(i%sems), 10)
			s := NewSemaphore(name, 2)
			defer FreeSemaphore(name)
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = s.Acquire(context.Background(), 1)
					defer s.Release(1)
					time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
				}()
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 0, len(semaphores), "expected all semaphores to be freed")
}
