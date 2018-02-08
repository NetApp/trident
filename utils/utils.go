// Copyright 2016 NetApp, Inc. All Rights Reserved.

package utils

import (
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

// Linux is a constant value for the runtime.GOOS that represents the Linux OS
const Linux = "linux"

// Windows is a constant value for the runtime.GOOS that represents the Windows OS
const Windows = "windows"

// Darwin is a constant value for the runtime.GOOS that represents Apple MacOS
const Darwin = "darwin"

/////////////////////////////////////////////////////////////////////////////
//
// Binary units
//
/////////////////////////////////////////////////////////////////////////////

type sizeUnit2 []string

func (s sizeUnit2) Len() int {
	return len(s)
}
func (s sizeUnit2) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s sizeUnit2) Less(i, j int) bool {
	return len(s[i]) > len(s[j])
}

var lookupTable2 = make(map[string]int)
var units2 = sizeUnit2{}

func init() {
	// populate the lookup table for binary suffixes
	lookupTable2["k"] = 1
	lookupTable2["ki"] = 1
	lookupTable2["kib"] = 1
	lookupTable2["m"] = 2
	lookupTable2["mi"] = 2
	lookupTable2["mib"] = 2
	lookupTable2["g"] = 3
	lookupTable2["gi"] = 3
	lookupTable2["gib"] = 3
	lookupTable2["t"] = 4
	lookupTable2["ti"] = 4
	lookupTable2["tib"] = 4
	lookupTable2["p"] = 5
	lookupTable2["pi"] = 5
	lookupTable2["pib"] = 5
	lookupTable2["e"] = 6
	lookupTable2["ei"] = 6
	lookupTable2["eib"] = 6
	lookupTable2["z"] = 7
	lookupTable2["zi"] = 7
	lookupTable2["zib"] = 7
	lookupTable2["y"] = 8
	lookupTable2["yi"] = 8
	lookupTable2["yib"] = 8

	// The slice of units is used to ensure that they are accessed by suffix from longest to
	// shortest, i.e. match 'tib' before matching 'b'.
	for unit := range lookupTable2 {
		units2 = append(units2, unit)
	}
	sort.Sort(units2)
}

/////////////////////////////////////////////////////////////////////////////
//
// SI units
//
/////////////////////////////////////////////////////////////////////////////

type sizeUnit10 []string

func (s sizeUnit10) Len() int {
	return len(s)
}
func (s sizeUnit10) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s sizeUnit10) Less(i, j int) bool {
	return len(s[i]) > len(s[j])
}

var lookupTable10 = make(map[string]int)
var units10 = sizeUnit10{}

func init() {
	// populate the lookup table for SI suffixes
	lookupTable10["b"] = 0
	lookupTable10["bytes"] = 0
	lookupTable10["kb"] = 1
	lookupTable10["mb"] = 2
	lookupTable10["gb"] = 3
	lookupTable10["tb"] = 4
	lookupTable10["pb"] = 5
	lookupTable10["eb"] = 6
	lookupTable10["zb"] = 7
	lookupTable10["yb"] = 8

	// The slice of units is used to ensure that they are accessed by suffix from longest to
	// shortest, i.e. match 'tib' before matching 'b'.
	for unit := range lookupTable10 {
		units10 = append(units10, unit)
	}
	sort.Sort(units10)
}

// Pow is an integer version of exponentiation; existing builtin is float, we needed an int version.
func Pow(x int64, y int) int64 {
	if y == 0 {
		return 1
	}

	result := x
	for n := 1; n < y; n++ {
		result = result * x
	}
	return result
}

// ConvertSizeToBytes converts size to bytes; see also https://en.wikipedia.org/wiki/Kilobyte
func ConvertSizeToBytes(s string) (string, error) {

	// make lowercase so units detection always works
	s = strings.TrimSpace(strings.ToLower(s))

	// first look for binary units
	for _, unit := range units2 {
		if strings.HasSuffix(s, unit) {
			s = strings.TrimSuffix(s, unit)
			i, err := strconv.ParseInt(s, 10, 0)
			if err != nil {
				return "", err
			}
			i = i * Pow(1024, lookupTable2[unit])
			s = strconv.FormatInt(i, 10)
			return s, nil
		}
	}

	// fall back to SI units
	for _, unit := range units10 {
		if strings.HasSuffix(s, unit) {
			s = strings.TrimSuffix(s, unit)
			i, err := strconv.ParseInt(s, 10, 0)
			if err != nil {
				return "", err
			}
			i = i * Pow(1000, lookupTable10[unit])
			s = strconv.FormatInt(i, 10)
			return s, nil
		}
	}

	return s, nil
}

// GetVolumeSizeBytes determines the size, in bytes, of a volume from the "size" opt value.  If "size" has a units
// suffix, that is handled here.  If there are no units, the default is GiB.  If size is not in opts, the specified
// default value is parsed identically and used instead.
func GetVolumeSizeBytes(opts map[string]string, defaultVolumeSize string) (uint64, error) {

	usingDefaultSize := false
	usingDefaultUnits := false

	// Use the size if specified, else use the configured default size
	size := GetV(opts, "size", "")
	if size == "" {
		size = defaultVolumeSize
		usingDefaultSize = true
	}

	// Default to GiB if no units are present
	if !sizeHasUnits(size) {
		size += "G"
		usingDefaultUnits = true
	}

	// Ensure the size is valid
	sizeBytesStr, err := ConvertSizeToBytes(size)
	if err != nil {
		return 0, fmt.Errorf("Invalid size value '%s' : %v", size, err)
	}
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)

	log.WithFields(log.Fields{
		"sizeBytes":         sizeBytes,
		"size":              size,
		"usingDefaultSize":  usingDefaultSize,
		"usingDefaultUnits": usingDefaultUnits,
	}).Debug("Determined volume size.")

	return sizeBytes, nil
}

// sizeHasUnits checks whether a size string includes a units suffix.
func sizeHasUnits(size string) bool {

	// make lowercase so units detection always works
	size = strings.TrimSpace(strings.ToLower(size))

	for _, unit := range units2 {
		if strings.HasSuffix(size, unit) {
			return true
		}
	}
	for _, unit := range units10 {
		if strings.HasSuffix(size, unit) {
			return true
		}
	}
	return false
}

// GetV takes a map, key(s), and a defaultValue; will return the value of the key or defaultValue if none is set.
// If keys is a string of key values separated by "|", then each key is tried in turn.  This allows compatibility
// with deprecated values, i.e. "fstype|fileSystemType".
func GetV(opts map[string]string, keys string, defaultValue string) string {

	for _, key := range strings.Split(keys, "|") {
		// Try key first, then do a case-insensitive search
		if value, ok := opts[key]; ok {
			return value
		} else {
			for k, v := range opts {
				if strings.EqualFold(k, key) {
					return v
				}
			}
		}
	}
	return defaultValue
}

// RandomString returns a string of the specified length consisting only of alphabetic characters.
func RandomString(str_size int) string {
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var bytes = make([]byte, str_size)
	rand.Seed(time.Now().UnixNano())
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = chars[b%byte(len(chars))]
	}
	return string(bytes)
}

func LogHttpRequest(request *http.Request, requestBody []byte) {
	header := ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
	footer := "--------------------------------------------------------------------------------"
	var body string
	if requestBody == nil {
		body = "<nil>"
	} else {
		body = string(requestBody)
	}
	log.Debugf("\n%s\n%s %s\nHeaders: %v\nBody: %s\n%s",
		header, request.Method, request.URL, request.Header, body, footer)
}

func LogHttpResponse(response *http.Response, responseBody []byte) {
	header := "<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
	footer := "================================================================================"
	var body string
	if responseBody == nil {
		body = "<nil>"
	} else {
		body = string(responseBody)
	}
	log.Debugf("\n%s\nStatus: %s\nHeaders: %v\nBody: %s\n%s",
		header, response.Status, response.Header, body, footer)
}

type HTTPError struct {
	Status     string
	StatusCode int
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("HTTP error: %s", e.Status)
}

func NewHTTPError(response *http.Response) *HTTPError {
	if response.StatusCode < 300 {
		return nil
	}
	return &HTTPError{response.Status, response.StatusCode}
}
