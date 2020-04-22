// Copyright 2020 NetApp, Inc. All Rights Reserved.

package utils

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
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
			if i, err := strconv.ParseInt(s, 10, 0); err != nil {
				return "", fmt.Errorf("invalid size value '%s': %v", s, err)
			} else {
				i = i * Pow(1024, lookupTable2[unit])
				s = strconv.FormatInt(i, 10)
				return s, nil
			}
		}
	}

	// fall back to SI units
	for _, unit := range units10 {
		if strings.HasSuffix(s, unit) {
			s = strings.TrimSuffix(s, unit)
			if i, err := strconv.ParseInt(s, 10, 0); err != nil {
				return "", fmt.Errorf("invalid size value '%s': %v", s, err)
			} else {
				i = i * Pow(1000, lookupTable10[unit])
				s = strconv.FormatInt(i, 10)
				return s, nil
			}
		}
	}

	// no valid units found, so ensure the value is a number
	if _, err := strconv.ParseUint(s, 10, 64); err != nil {
		return "", fmt.Errorf("invalid size value '%s': %v", s, err)
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
		return 0, err
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

// VolumeSizeWithinTolerance checks to see if requestedSize is within the delta of the currentSize.
// If within the delta true is returned. If not within the delta and requestedSize is less than the
// currentSize false is returned.
func VolumeSizeWithinTolerance(requestedSize int64, currentSize int64, delta int64) (bool, error) {

	sizeDiff := requestedSize - currentSize
	if sizeDiff < 0 {
		sizeDiff = -sizeDiff
	}

	if sizeDiff <= delta {
		return true, nil
	}
	return false, nil
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
func RandomString(strSize int) string {
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var bytes = make([]byte, strSize)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = chars[b%byte(len(chars))]
	}
	return string(bytes)
}

// StringInSlice checks whether a string is in a list of strings
func StringInSlice(s string, list []string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func LogHTTPRequest(request *http.Request, requestBody []byte) {
	header := ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
	footer := "--------------------------------------------------------------------------------"

	requestURL, _ := url.Parse(request.URL.String())
	requestURL.User = nil

	headers := make(map[string][]string)
	for k, v := range request.Header {
		headers[k] = v
	}
	delete(headers, "Authorization")
	delete(headers, "Api-Key")
	delete(headers, "Secret-Key")

	var body string
	if requestBody == nil {
		body = "<nil>"
	} else {
		body = string(requestBody)
	}

	log.Debugf("\n%s\n%s %s\nHeaders: %v\nBody: %s\n%s",
		header, request.Method, requestURL, headers, body, footer)
}

func LogHTTPResponse(response *http.Response, responseBody []byte) {
	header := "<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
	footer := "================================================================================"

	headers := make(map[string][]string)
	for k, v := range response.Header {
		headers[k] = v
	}
	delete(headers, "Authorization")
	delete(headers, "Api-Key")
	delete(headers, "Secret-Key")

	var body string
	if responseBody == nil {
		body = "<nil>"
	} else {
		body = string(responseBody)
	}
	log.Debugf("\n%s\nStatus: %s\nHeaders: %v\nBody: %s\n%s",
		header, response.Status, headers, body, footer)
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

// SliceContainsString checks to see if a []string contains a string
func SliceContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveStringFromSlice removes a string from a []string
func RemoveStringFromSlice(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// SplitImageDomain accepts a container image name and splits off the domain portion, if any.
func SplitImageDomain(name string) (domain, remainder string) {
	i := strings.IndexRune(name, '/')
	if i == -1 || (!strings.ContainsAny(name[:i], ".:") && name[:i] != "localhost") {
		domain, remainder = "", name
	} else {
		domain, remainder = name[:i], name[i+1:]
	}
	return
}

// ReplaceImageRegistry accepts a container image name and a registry name (FQDN[:port]) and
// returns the same image name with the supplied registry.
func ReplaceImageRegistry(image, registry string) string {
	_, remainder := SplitImageDomain(image)
	if registry == "" {
		return remainder
	}
	return registry + "/" + remainder
}

// FilterIPs takes a list of IPs and CIDRs and returns the sorted list of IPs that are contained by one or more of the
// CIDRs
func FilterIPs(ips, cidrs []string) ([]string, error) {
	networks := make([]*net.IPNet, len(cidrs))
	filteredIPs := make([]string, 0)

	for i, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			err = fmt.Errorf("error parsing CIDR; %v", err)
			log.WithField("CIDR", cidr).Error(err)
			return nil, err
		}
		networks[i] = ipNet
	}

	for _, ip := range ips {
		parsedIP := net.ParseIP(ip)
		for _, network := range networks {
			fields := log.Fields{
				"IP":      ip,
				"Network": network.String(),
			}
			if network.Contains(parsedIP) {
				log.WithFields(fields).Debug("IP found in network.")
				filteredIPs = append(filteredIPs, ip)
				break
			} else {
				log.WithFields(fields).Debug("IP not found in network.")
			}
		}
	}

	sort.Strings(filteredIPs)

	return filteredIPs, nil
}

// GetYAMLTagWithSpaceCount returns the line that matches the pattern, tag name, and the spaces before the tag
func GetYAMLTagWithSpaceCount(text string) (string, string, int) {

	// This matches pattern in a multiline string of type "    {something}\n"
	tagsWithIndentationRegex := regexp.MustCompile(`(?m)^[\t ]*{(?P<tagName>[\w-]+)}$\n`)
	tag := tagsWithIndentationRegex.FindStringSubmatch(text)

	// Since we have two of `()` in the pattern, we want to use the tag identified by the second `()`.
	if len(tag) > 1 {
		tagWithSpaces := tagsWithIndentationRegex.FindString(text)
		indentation := CountSpacesBeforeText(tagWithSpaces)

		return tagWithSpaces, tag[1], indentation
	}

	return "", "", 0
}

func CountSpacesBeforeText(text string) int {
	return len(text) - len(strings.TrimLeft(text, " \t"))
}
