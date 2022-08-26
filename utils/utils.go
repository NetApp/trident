// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"go.uber.org/multierr"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	. "github.com/netapp/trident/logger"
)

const (
	// Linux is a constant value for the runtime.GOOS that represents the Linux OS
	Linux = "linux"

	// Windows is a constant value for the runtime.GOOS that represents the Windows OS
	Windows = "windows"

	// Darwin is a constant value for the runtime.GOOS that represents Apple MacOS
	Darwin = "darwin"

	PrepPending       NodePrepStatus = "pending"
	PrepRunning       NodePrepStatus = "running"
	PrepCompleted     NodePrepStatus = "completed"
	PrepFailed        NodePrepStatus = "failed"
	PrepOutdated      NodePrepStatus = "outdated"
	PrepPreConfigured NodePrepStatus = "preconfigured"

	Centos = "centos"
	RHEL   = "rhel"
	Ubuntu = "ubuntu"
	Debian = "debian"

	REDACTED = "<REDACTED>"
)

var (
	NFSVersionMajorRegex      = regexp.MustCompile(`^(nfsvers|vers)=(?P<major>\d)$`)
	NFSVersionMajorMinorRegex = regexp.MustCompile(`^(nfsvers|vers)=(?P<major>\d)\.(?P<minor>\d)$`)
	NFSVersionMinorRegex      = regexp.MustCompile(`^minorversion=(?P<minor>\d)$`)
)

// ///////////////////////////////////////////////////////////////////////////
//
// Binary units
//
// ///////////////////////////////////////////////////////////////////////////

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

var (
	lookupTable2 = make(map[string]int)
	units2       = sizeUnit2{}
)

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

// ///////////////////////////////////////////////////////////////////////////
//
// SI units
//
// ///////////////////////////////////////////////////////////////////////////

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

var (
	lookupTable10 = make(map[string]int)
	units10       = sizeUnit10{}
)

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
func GetVolumeSizeBytes(ctx context.Context, opts map[string]string, defaultVolumeSize string) (uint64, error) {
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

	Logc(ctx).WithFields(log.Fields{
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
func VolumeSizeWithinTolerance(requestedSize, currentSize, delta int64) (bool, error) {
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
func GetV(opts map[string]string, keys, defaultValue string) string {
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
	bytes := make([]byte, strSize)
	_, err := rand.Read(bytes)
	if err != nil {
		Logc(context.Background()).WithError(err).Error("Unable to generate random bytes")
	}
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

func LogHTTPRequest(request *http.Request, requestBody []byte, redactBody bool) {
	header := ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
	footer := "--------------------------------------------------------------------------------"

	ctx := request.Context()

	requestURL, err := url.Parse(request.URL.String())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Unable to parse URL '%s'", request.URL.String())
	}
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
	} else if redactBody {
		body = REDACTED
	} else {
		body = string(requestBody)
	}

	Logc(ctx).Debugf("\n%s\n%s %s\nHeaders: %v\nBody: %s\n%s",
		header, request.Method, requestURL, headers, body, footer)
}

func LogHTTPResponse(ctx context.Context, response *http.Response, responseBody []byte, redactBody bool) {
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
	} else if redactBody {
		body = REDACTED
	} else {
		body = string(responseBody)
	}
	Logc(ctx).Debugf("\n%s\nStatus: %s\nHeaders: %v\nBody: %s\n%s",
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

// SliceContains checks to see if a slice (genericSlice) contains an item (genericElement)
func SliceContains(genericSlice, genericElement interface{}) bool {
	slice, elem := reflect.ValueOf(genericSlice), reflect.ValueOf(genericElement)

	if slice.Kind() != reflect.Slice && slice.Kind() != reflect.Array {
		return false
	}

	if slice.Len() > 1 {
		if slice.Index(0).Type() != elem.Type() {
			return false
		}
	}

	for i := 0; i < slice.Len(); i++ {
		if elem.Interface() == slice.Index(i).Interface() {
			return true
		}
	}

	return false
}

// SliceContainsElements checks to see if a slice (genericSlice) contains a list of all/some items (genericElements)
func SliceContainsElements(genericSlice, genericElements interface{}) (bool, bool) {
	slice, elem := reflect.ValueOf(genericSlice), reflect.ValueOf(genericElements)

	if slice.Kind() != reflect.Slice && slice.Kind() != reflect.Array {
		return false, false
	}

	if elem.Kind() != reflect.Slice && elem.Kind() != reflect.Array {
		return false, false
	}

	containAll := true
	containSome := false
	for i := 0; i < elem.Len(); i++ {
		if SliceContains(genericSlice, elem.Index(i).Interface()) {
			containSome = true
		} else {
			containAll = false
		}
	}

	if !containSome {
		containAll = false
	}

	return containAll, containSome
}

// SliceContainsString checks to see if a []string contains a string
func SliceContainsString(slice []string, s string) bool {
	return SliceContainsStringConditionally(slice, s, func(val1, val2 string) bool { return val1 == val2 })
}

// SliceContainsStringCaseInsensitive is SliceContainsString but case insensitive
func SliceContainsStringCaseInsensitive(slice []string, s string) bool {
	matchFunc := func(main, val string) bool {
		return strings.EqualFold(main, val)
	}

	return SliceContainsStringConditionally(slice, s, matchFunc)
}

// SliceContainsStringConditionally checks to see if a []string contains a string based on certain criteria
func SliceContainsStringConditionally(slice []string, s string, fn func(string, string) bool) bool {
	for _, item := range slice {
		if fn(item, s) {
			return true
		}
	}
	return false
}

// RemoveStringFromSlice removes a string from a []string
func RemoveStringFromSlice(slice []string, s string) (result []string) {
	return RemoveStringFromSliceConditionally(slice, s, func(val1, val2 string) bool { return val1 == val2 })
}

// RemoveStringFromSliceConditionally removes a string from a []string if it meets certain criteria
func RemoveStringFromSliceConditionally(slice []string, s string, fn func(string, string) bool) (result []string) {
	for _, item := range slice {
		if fn(s, item) {
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

// ValidateCIDRs checks if a list of CIDR blocks are valid and returns a multi error containing all errors
// which may occur during the parsing process.
func ValidateCIDRs(ctx context.Context, cidrs []string) error {
	var err error
	// needed to capture all issues within the CIDR set
	errors := make([]error, 0)
	for _, cidr := range cidrs {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			errors = append(errors, err)
			Logc(ctx).WithError(err).Error("Found an invalid CIDR.")
		}
	}

	if len(errors) != 0 {
		err = multierr.Combine(errors...)
		return err
	}

	return err
}

// FilterIPs takes a list of IPs and CIDRs and returns the sorted list of IPs that are contained by one or more of the
// CIDRs
func FilterIPs(ctx context.Context, ips, cidrs []string) ([]string, error) {
	networks := make([]*net.IPNet, len(cidrs))
	filteredIPs := make([]string, 0)

	for i, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			err = fmt.Errorf("error parsing CIDR; %v", err)
			Logc(ctx).WithField("CIDR", cidr).Error(err)
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
				Logc(ctx).WithFields(fields).Debug("IP found in network.")
				filteredIPs = append(filteredIPs, ip)
				break
			} else {
				Logc(ctx).WithFields(fields).Debug("IP not found in network.")
			}
		}
	}

	sort.Strings(filteredIPs)

	return filteredIPs, nil
}

func GetYAMLTagWithSpaceCount(text, tagName string) (string, int) {
	// This matches pattern in a multiline string of type "    {something}\n"
	tagsWithIndentationRegex := regexp.MustCompile(`(?m)^[\t ]*{` + tagName + `}$\n`)
	tag := tagsWithIndentationRegex.FindStringSubmatch(text)

	// Since we have two of `()` in the pattern, we want to use the tag identified by the second `()`.
	if len(tag) > 0 {
		tagWithSpaces := tagsWithIndentationRegex.FindString(text)
		indentation := CountSpacesBeforeText(tagWithSpaces)

		return tagWithSpaces, indentation
	}

	return "", 0
}

func CountSpacesBeforeText(text string) int {
	return len(text) - len(strings.TrimLeft(text, " \t"))
}

// GetFindMultipathValue returns the value of find_multipaths
// Returned values:
// no (or off): Create a multipath device for every path that is not explicitly disabled
// yes (or on): Create a device if one of some conditions are met
// other possible values: smart, greedy, strict
func GetFindMultipathValue(text string) string {
	// This matches pattern in a multiline string of type "    find_multipaths: yes"
	tagsWithIndentationRegex := regexp.MustCompile(`(?m)^[\t ]*find_multipaths[\t ]*["|']?(?P<tagName>[\w-_]+)["|']?[\t ]*$`)
	tag := tagsWithIndentationRegex.FindStringSubmatch(text)

	// Since we have two of `()` in the pattern, we want to use the tag identified by the second `()`.
	if len(tag) > 1 {
		if tag[1] == "off" {
			return "no"
		} else if tag[1] == "on" {
			return "yes"
		}

		return tag[1]
	}

	return ""
}

// AreMountOptionsInList returns true if any of the options are in mountOptions
func AreMountOptionsInList(mountOptions string, optionList []string) bool {
	if mountOptions == "" || len(optionList) == 0 {
		return false
	}

	mountOptionsSlice := strings.Split(strings.TrimPrefix(mountOptions, "-o"), ",")

	for _, mountOptionItem := range mountOptionsSlice {
		if SliceContainsString(optionList, mountOptionItem) {
			return true
		}
	}
	return false
}

// SanitizeMountOptions removes the options provided from the string list and spaces around options
func SanitizeMountOptions(mountOptions string, removeMountOptions []string) string {
	if mountOptions == "" || len(removeMountOptions) == 0 {
		return mountOptions
	}

	sanitized := make([]string, 0)

	for _, mountOption := range strings.Split(mountOptions, ",") {
		trimmedMountOption := strings.TrimSpace(mountOption)
		if !SliceContainsString(removeMountOptions, trimmedMountOption) {
			sanitized = append(sanitized, trimmedMountOption)
		}
	}

	return strings.Join(sanitized, ",")
}

// GetNFSVersionFromMountOptions accepts a set of mount options, a default NFS version, and a list of
// supported NFS versions, and it returns the NFS version specified by the mount options, or the default
// if none is found, plus an error (if any).  If a set of supported versions is supplied, and the returned
// version isn't in it, this method returns an error; otherwise it returns nil.
func GetNFSVersionFromMountOptions(mountOptions, defaultVersion string, supportedVersions []string) (string, error) {
	major := ""
	minor := ""

	// Strip off -o prefix if present
	mountOptions = strings.TrimPrefix(mountOptions, "-o ")

	// Check each mount option using the three mutually-exclusive regular expressions.  Last option wins.
	for _, mountOption := range strings.Split(mountOptions, ",") {

		mountOption = strings.TrimSpace(mountOption)

		if matchGroups := GetRegexSubmatches(NFSVersionMajorMinorRegex, mountOption); matchGroups != nil {
			major = matchGroups["major"]
			minor = matchGroups["minor"]
			continue
		}

		if matchGroups := GetRegexSubmatches(NFSVersionMajorRegex, mountOption); matchGroups != nil {
			major = matchGroups["major"]
			continue
		}

		if matchGroups := GetRegexSubmatches(NFSVersionMinorRegex, mountOption); matchGroups != nil {
			minor = matchGroups["minor"]
			continue
		}
	}

	version := ""

	// Assemble version string from major/minor values.
	if major == "" {
		// Minor doesn't matter if major isn't set.
		version = ""
	} else if minor == "" {
		// Major works by itself if minor isn't set.
		version = major
	} else {
		version = major + "." + minor
	}

	// Handle default & supported versions.
	if version == "" {
		return defaultVersion, nil
	} else if supportedVersions == nil || SliceContainsString(supportedVersions, version) {
		return version, nil
	} else {
		return version, fmt.Errorf("unsupported NFS version: %s", version)
	}
}

// GetNFSVersionMountOptions accepts a set of mount options, identifies
// and returns all of the NFS version mount options.
func GetNFSVersionMountOptions(mountOptions string) []string {
	// Strip off -o prefix if present
	NFSMountOptions := make([]string, 0)

	// Strip off -o prefix if present
	mountOptions = strings.TrimPrefix(mountOptions, "-o ")

	// Check each mount option using the three mutually-exclusive regular expressions.  Last option wins.
	for _, mountOption := range strings.Split(mountOptions, ",") {

		mountOption = strings.TrimSpace(mountOption)

		if matchGroups := GetRegexSubmatches(NFSVersionMajorMinorRegex, mountOption); matchGroups != nil {
			NFSMountOptions = append(NFSMountOptions, mountOption)
			continue
		}

		if matchGroups := GetRegexSubmatches(NFSVersionMajorRegex, mountOption); matchGroups != nil {
			NFSMountOptions = append(NFSMountOptions, mountOption)
			continue
		}

		if matchGroups := GetRegexSubmatches(NFSVersionMinorRegex, mountOption); matchGroups != nil {
			NFSMountOptions = append(NFSMountOptions, mountOption)
			continue
		}
	}

	return NFSMountOptions
}

// SetNFSVersionMountOptions removes any existing NFS version mount option and sets passed NFS version mount options.
func SetNFSVersionMountOptions(mountOptions, newNFSVersionMountOption string) string {
	// Strip off -o prefix if present or any "," at the end
	mountOptions = strings.TrimPrefix(mountOptions, "-o ")
	mountOptions = strings.TrimSuffix(mountOptions, ",")

	NFSMountOptions := GetNFSVersionMountOptions(mountOptions)

	// Remove any of the NFS mount options already set
	if len(NFSMountOptions) != 0 {
		mountOptions = SanitizeMountOptions(mountOptions, NFSMountOptions)
	}

	// Set the new NFS version mount option
	return AppendToStringList(mountOptions, newNFSVersionMountOption, ",")
}

// GetRegexSubmatches accepts a regular expression with one or more groups and returns a map
// of the group matches found in the supplied string.
func GetRegexSubmatches(r *regexp.Regexp, s string) map[string]string {
	match := r.FindStringSubmatch(s)
	if match == nil {
		return nil
	}

	paramsMap := make(map[string]string)
	for i, name := range r.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}
	return paramsMap
}

// Detect if code is running in a container or not
func RunningInContainer() bool {
	return os.Getenv("CSI_ENDPOINT") != ""
}

func Max(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

// SplitString is same as strings.Split except it returns a nil ([]string(nil)) of size 0 instead of
// string slice with an empty string (size 1) when string to be split is empty.
func SplitString(_ context.Context, s, sep string) []string {
	if s == "" {
		return nil
	}

	return strings.Split(s, sep)
}

// ReplaceAtIndex returns a string with the rune at the specified index replaced
func ReplaceAtIndex(in string, r rune, index int) (string, error) {
	if index < 0 || index >= len(in) {
		return in, fmt.Errorf("index '%d' out of bounds for string '%s'", index, in)
	}
	out := []rune(in)
	out[index] = r
	return string(out), nil
}

// MinInt64 returns the lower of the two integers specified
func MinInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func ValidateOctalUnixPermissions(perms string) error {
	permsRegex := regexp.MustCompile(`^[0-7]{4}$`)

	if !permsRegex.MatchString(perms) {
		return fmt.Errorf("%s is not a valid octal unix permissions value", perms)
	}

	return nil
}

func GenerateVolumePublishName(volumeID, nodeID string) string {
	return fmt.Sprintf(volumeID + "." + nodeID)
}

// ToStringRedacted identifies attributes of a struct, stringifies them such that they can be consumed by the
// struct's stringer interface, and redacts elements specified in the redactList.
func ToStringRedacted(structPointer interface{}, redactList []string, configVal interface{}) (out string) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Panic in utils#ToStringRedacted; err: %v", r)
			out = "<panic>"
		}
	}()

	elements := reflect.ValueOf(structPointer).Elem()

	var output strings.Builder

	for i := 0; i < elements.NumField(); i++ {
		fieldName := elements.Type().Field(i).Name
		switch {
		case fieldName == "Config" && configVal != nil:
			output.WriteString(fmt.Sprintf("%v:%v ", fieldName, configVal))
		case SliceContainsString(redactList, fieldName):
			output.WriteString(fmt.Sprintf("%v:%v ", fieldName, REDACTED))
		default:
			output.WriteString(fmt.Sprintf("%v:%#v ", fieldName, elements.Field(i)))
		}
	}

	out = output.String()
	return
}

func RedactSecretsFromString(stringToSanitize string, replacements map[string]string, useRegex bool) string {
	compileError := "regex matching the secret could not compile, so the entire string has been redacted"

	for key, value := range replacements {
		if useRegex {
			pattern, err := regexp.Compile(key)
			if err != nil {
				return compileError
			}
			stringToSanitize = pattern.ReplaceAllString(stringToSanitize, value)
		} else {
			stringToSanitize = strings.ReplaceAll(stringToSanitize, key, value)
		}
	}

	return stringToSanitize
}

// GetVerifiedBlockFsType retrieves and verifies the filesystem type for a BlockOnFile protocol
func GetVerifiedBlockFsType(filesystemType string) (string, error) {
	fsTypeSplit := strings.Split(filesystemType, "/")

	if len(fsTypeSplit) != 2 {
		return "", fmt.Errorf("unable to get filesystem type from '%v'", filesystemType)
	}

	if fsTypeSplit[0] != "nfs" {
		return "", fmt.Errorf("unrecognized fileSystemType option: %s", filesystemType)
	}

	return VerifyFilesystemSupport(fsTypeSplit[1])
}

// VerifyFilesystemSupport checks for a supported file system type
func VerifyFilesystemSupport(fs string) (string, error) {
	fstype := strings.ToLower(fs)
	switch fstype {
	case fsXfs, fsExt3, fsExt4, fsRaw:
		return fstype, nil
	default:
		return "", fmt.Errorf("unsupported fileSystemType option: %s", fstype)
	}
}

// AppendToStringList appends an item to a string list with a seperator
func AppendToStringList(stringList, newItem, sep string) string {
	stringListItems := SplitString(context.TODO(), stringList, sep)

	if len(stringListItems) == 0 {
		return newItem
	}

	stringListItems = append(stringListItems, newItem)
	return strings.Join(stringListItems, sep)
}

// getHostportIP returns just the IP address part of the given input IP address and strips any port information
func getHostportIP(hostport string) string {
	ipAddress := ""
	if IPv6Check(hostport) {
		// this is an IPv6 address, remove port value and add square brackets around the IP address
		if hostport[0] == '[' {
			ipAddress = strings.Split(hostport, "]")[0] + "]"
		} else {
			// assumption here is that without the square brackets its only IP address without port information
			ipAddress = "[" + hostport + "]"
		}
	} else {
		ipAddress = strings.Split(hostport, ":")[0]
	}

	return ipAddress
}

// ensureHostportFormatted ensures IPv6 hostport is in correct format
func ensureHostportFormatted(hostport string) string {
	// If this is an IPv6 address, ensure IP address is enclosed in square
	// brackets, as in "[::1]:80".
	if IPv6Check(hostport) && hostport[0] != '[' {
		// assumption here is that without the square brackets its only IP address without port information
		return "[" + hostport + "]"
	}

	return hostport
}

func IPv6Check(ip string) bool {
	return strings.Count(ip, ":") >= 2
}

// ConsistentRead repeatedly reads a file until it gets the same content twice.
// This is useful when reading files in /proc that are larger than page size
// and kernel may modify them between individual read() syscalls.
func ConsistentRead(filename string, attempts int) ([]byte, error) {
	oldContent, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	for i := 0; i < attempts; i++ {
		newContent, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(oldContent, newContent) {
			return newContent, nil
		}
		// Files are different, continue reading
		oldContent = newContent
	}
	return nil, fmt.Errorf("could not get consistent content of %s after %d attempts", filename, attempts)
}

func Title(str string) string {
	return cases.Title(language.Und, cases.NoLower).String(str)
}

func GetPrintableBoolPtrValue(bPtr *bool) string {
	if bPtr != nil {
		if *bPtr {
			return "true"
		}
		return "false"
	}
	return "none"
}
