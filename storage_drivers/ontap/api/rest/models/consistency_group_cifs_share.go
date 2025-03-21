// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// ConsistencyGroupCifsShare CIFS share is a named access point in a volume. Before users and applications can access data on the CIFS server over SMB,
// a CIFS share must be created with sufficient share permission. CIFS shares are tied to the CIFS server on the SVM.
// When a CIFS share is created, ONTAP creates a default ACL for the share with Full Control permissions for Everyone.
//
// swagger:model consistency_group_cifs_share
type ConsistencyGroupCifsShare struct {

	// links
	Links *ConsistencyGroupCifsShareInlineLinks `json:"_links,omitempty"`

	// Specifies whether all folders inside this share are visible to a user based on that individual user's access right; prevents
	// the display of folders or other shared resources that the user does not have access to.
	//
	AccessBasedEnumeration *bool `json:"access_based_enumeration,omitempty"`

	// Specifies whether or not the SMB2 clients are allowed to access the encrypted share.
	//
	AllowUnencryptedAccess *bool `json:"allow_unencrypted_access,omitempty"`

	// Specifies whether CIFS clients can request for change notifications for directories on this share.
	ChangeNotify *bool `json:"change_notify,omitempty"`

	// Specify the CIFS share descriptions.
	// Example: HR Department Share
	// Max Length: 256
	// Min Length: 1
	Comment *string `json:"comment,omitempty"`

	// consistency group cifs share inline acls
	ConsistencyGroupCifsShareInlineAcls []*ConsistencyGroupCifsShareInlineAclsInlineArrayItem `json:"acls,omitempty"`

	// Specifies whether or not the clients connecting to this share can open files in a persistent manner.
	// Files opened in this way are protected from disruptive events, such as, failover and giveback.
	//
	ContinuouslyAvailable *bool `json:"continuously_available,omitempty"`

	// Directory mode creation mask to be viewed as an octal number.
	// Example: 18
	DirUmask *int64 `json:"dir_umask,omitempty"`

	// Specifies whether SMB encryption must be used when accessing this share. Clients that do not support encryption are not
	// able to access this share.
	//
	Encryption *bool `json:"encryption,omitempty"`

	// File mode creation mask to be viewed as an octal number.
	// Example: 18
	FileUmask *int64 `json:"file_umask,omitempty"`

	// Specifies whether or not the share is a home directory share, where the share and path names are dynamic.
	// ONTAP home directory functionality automatically offer each user a dynamic share to their home directory without creating an
	// individual SMB share for each user.
	// The ONTAP CIFS home directory feature enable us to configure a share that maps to
	// different directories based on the user that connects to it. Instead of creating a separate shares for each user,
	// a single share with a home directory parameters can be created.
	// In a home directory share, ONTAP dynamically generates the share-name and share-path by substituting
	// %w, %u, and %d variables with the corresponding Windows user name, UNIX user name, and domain name, respectively.
	//
	HomeDirectory *bool `json:"home_directory,omitempty"`

	// Specifies the name of the CIFS share that you want to create. If this
	// is a home directory share then the share name includes the pattern as
	// %w (Windows user name), %u (UNIX user name) and %d (Windows domain name)
	// variables in any combination with this parameter to generate shares dynamically.
	//
	// Example: HR_SHARE
	// Max Length: 80
	// Min Length: 1
	Name *string `json:"name,omitempty"`

	// Specifies whether or not the SMB clients connecting to this share can cache the directory enumeration
	// results returned by the CIFS servers.
	//
	NamespaceCaching *bool `json:"namespace_caching,omitempty"`

	// Specifies whether or not CIFS clients can follow Unix symlinks outside the share boundaries.
	//
	NoStrictSecurity *bool `json:"no_strict_security,omitempty"`

	// Offline Files
	// The supported values are:
	//   * none - Clients are not permitted to cache files for offline access.
	//   * manual - Clients may cache files that are explicitly selected by the user for offline access.
	//   * documents - Clients may automatically cache files that are used by the user for offline access.
	//   * programs - Clients may automatically cache files that are used by the user for offline access
	//                and may use those files in an offline mode even if the share is available.
	//
	// Enum: ["none","manual","documents","programs"]
	OfflineFiles *string `json:"offline_files,omitempty"`

	// Specifies whether opportunistic locks are enabled on this share. "Oplocks" allow clients to lock files and cache content locally,
	// which can increase performance for file operations.
	//
	Oplocks *bool `json:"oplocks,omitempty"`

	// Specifies whether or not the snapshots can be viewed and traversed by clients.
	//
	ShowSnapshot *bool `json:"show_snapshot,omitempty"`

	// Controls the access of UNIX symbolic links to CIFS clients.
	// The supported values are:
	//     * local - Enables only local symbolic links which is within the same CIFS share.
	//     * widelink - Enables both local symlinks and widelinks.
	//     * disable - Disables local symlinks and widelinks.
	//
	// Enum: ["local","widelink","disable"]
	UnixSymlink *string `json:"unix_symlink,omitempty"`

	// Vscan File-Operations Profile
	// The supported values are:
	//   * no_scan - Virus scans are never triggered for accesses to this share.
	//   * standard - Virus scans can be triggered by open, close, and rename operations.
	//   * strict - Virus scans can be triggered by open, read, close, and rename operations.
	//   * writes_only - Virus scans can be triggered only when a file that has been modified is closed.
	//
	// Enum: ["no_scan","standard","strict","writes_only"]
	VscanProfile *string `json:"vscan_profile,omitempty"`
}

// Validate validates this consistency group cifs share
func (m *ConsistencyGroupCifsShare) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateComment(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateConsistencyGroupCifsShareInlineAcls(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateName(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateOfflineFiles(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateUnixSymlink(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateVscanProfile(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ConsistencyGroupCifsShare) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(m.Links) { // not required
		return nil
	}

	if m.Links != nil {
		if err := m.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links")
			}
			return err
		}
	}

	return nil
}

func (m *ConsistencyGroupCifsShare) validateComment(formats strfmt.Registry) error {
	if swag.IsZero(m.Comment) { // not required
		return nil
	}

	if err := validate.MinLength("comment", "body", *m.Comment, 1); err != nil {
		return err
	}

	if err := validate.MaxLength("comment", "body", *m.Comment, 256); err != nil {
		return err
	}

	return nil
}

func (m *ConsistencyGroupCifsShare) validateConsistencyGroupCifsShareInlineAcls(formats strfmt.Registry) error {
	if swag.IsZero(m.ConsistencyGroupCifsShareInlineAcls) { // not required
		return nil
	}

	for i := 0; i < len(m.ConsistencyGroupCifsShareInlineAcls); i++ {
		if swag.IsZero(m.ConsistencyGroupCifsShareInlineAcls[i]) { // not required
			continue
		}

		if m.ConsistencyGroupCifsShareInlineAcls[i] != nil {
			if err := m.ConsistencyGroupCifsShareInlineAcls[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("acls" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (m *ConsistencyGroupCifsShare) validateName(formats strfmt.Registry) error {
	if swag.IsZero(m.Name) { // not required
		return nil
	}

	if err := validate.MinLength("name", "body", *m.Name, 1); err != nil {
		return err
	}

	if err := validate.MaxLength("name", "body", *m.Name, 80); err != nil {
		return err
	}

	return nil
}

var consistencyGroupCifsShareTypeOfflineFilesPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["none","manual","documents","programs"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		consistencyGroupCifsShareTypeOfflineFilesPropEnum = append(consistencyGroupCifsShareTypeOfflineFilesPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// offline_files
	// OfflineFiles
	// none
	// END DEBUGGING
	// ConsistencyGroupCifsShareOfflineFilesNone captures enum value "none"
	ConsistencyGroupCifsShareOfflineFilesNone string = "none"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// offline_files
	// OfflineFiles
	// manual
	// END DEBUGGING
	// ConsistencyGroupCifsShareOfflineFilesManual captures enum value "manual"
	ConsistencyGroupCifsShareOfflineFilesManual string = "manual"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// offline_files
	// OfflineFiles
	// documents
	// END DEBUGGING
	// ConsistencyGroupCifsShareOfflineFilesDocuments captures enum value "documents"
	ConsistencyGroupCifsShareOfflineFilesDocuments string = "documents"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// offline_files
	// OfflineFiles
	// programs
	// END DEBUGGING
	// ConsistencyGroupCifsShareOfflineFilesPrograms captures enum value "programs"
	ConsistencyGroupCifsShareOfflineFilesPrograms string = "programs"
)

// prop value enum
func (m *ConsistencyGroupCifsShare) validateOfflineFilesEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, consistencyGroupCifsShareTypeOfflineFilesPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *ConsistencyGroupCifsShare) validateOfflineFiles(formats strfmt.Registry) error {
	if swag.IsZero(m.OfflineFiles) { // not required
		return nil
	}

	// value enum
	if err := m.validateOfflineFilesEnum("offline_files", "body", *m.OfflineFiles); err != nil {
		return err
	}

	return nil
}

var consistencyGroupCifsShareTypeUnixSymlinkPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["local","widelink","disable"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		consistencyGroupCifsShareTypeUnixSymlinkPropEnum = append(consistencyGroupCifsShareTypeUnixSymlinkPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// unix_symlink
	// UnixSymlink
	// local
	// END DEBUGGING
	// ConsistencyGroupCifsShareUnixSymlinkLocal captures enum value "local"
	ConsistencyGroupCifsShareUnixSymlinkLocal string = "local"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// unix_symlink
	// UnixSymlink
	// widelink
	// END DEBUGGING
	// ConsistencyGroupCifsShareUnixSymlinkWidelink captures enum value "widelink"
	ConsistencyGroupCifsShareUnixSymlinkWidelink string = "widelink"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// unix_symlink
	// UnixSymlink
	// disable
	// END DEBUGGING
	// ConsistencyGroupCifsShareUnixSymlinkDisable captures enum value "disable"
	ConsistencyGroupCifsShareUnixSymlinkDisable string = "disable"
)

// prop value enum
func (m *ConsistencyGroupCifsShare) validateUnixSymlinkEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, consistencyGroupCifsShareTypeUnixSymlinkPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *ConsistencyGroupCifsShare) validateUnixSymlink(formats strfmt.Registry) error {
	if swag.IsZero(m.UnixSymlink) { // not required
		return nil
	}

	// value enum
	if err := m.validateUnixSymlinkEnum("unix_symlink", "body", *m.UnixSymlink); err != nil {
		return err
	}

	return nil
}

var consistencyGroupCifsShareTypeVscanProfilePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["no_scan","standard","strict","writes_only"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		consistencyGroupCifsShareTypeVscanProfilePropEnum = append(consistencyGroupCifsShareTypeVscanProfilePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// vscan_profile
	// VscanProfile
	// no_scan
	// END DEBUGGING
	// ConsistencyGroupCifsShareVscanProfileNoScan captures enum value "no_scan"
	ConsistencyGroupCifsShareVscanProfileNoScan string = "no_scan"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// vscan_profile
	// VscanProfile
	// standard
	// END DEBUGGING
	// ConsistencyGroupCifsShareVscanProfileStandard captures enum value "standard"
	ConsistencyGroupCifsShareVscanProfileStandard string = "standard"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// vscan_profile
	// VscanProfile
	// strict
	// END DEBUGGING
	// ConsistencyGroupCifsShareVscanProfileStrict captures enum value "strict"
	ConsistencyGroupCifsShareVscanProfileStrict string = "strict"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share
	// ConsistencyGroupCifsShare
	// vscan_profile
	// VscanProfile
	// writes_only
	// END DEBUGGING
	// ConsistencyGroupCifsShareVscanProfileWritesOnly captures enum value "writes_only"
	ConsistencyGroupCifsShareVscanProfileWritesOnly string = "writes_only"
)

// prop value enum
func (m *ConsistencyGroupCifsShare) validateVscanProfileEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, consistencyGroupCifsShareTypeVscanProfilePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *ConsistencyGroupCifsShare) validateVscanProfile(formats strfmt.Registry) error {
	if swag.IsZero(m.VscanProfile) { // not required
		return nil
	}

	// value enum
	if err := m.validateVscanProfileEnum("vscan_profile", "body", *m.VscanProfile); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this consistency group cifs share based on the context it is used
func (m *ConsistencyGroupCifsShare) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateConsistencyGroupCifsShareInlineAcls(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ConsistencyGroupCifsShare) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if m.Links != nil {
		if err := m.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links")
			}
			return err
		}
	}

	return nil
}

func (m *ConsistencyGroupCifsShare) contextValidateConsistencyGroupCifsShareInlineAcls(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.ConsistencyGroupCifsShareInlineAcls); i++ {

		if m.ConsistencyGroupCifsShareInlineAcls[i] != nil {
			if err := m.ConsistencyGroupCifsShareInlineAcls[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("acls" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (m *ConsistencyGroupCifsShare) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *ConsistencyGroupCifsShare) UnmarshalBinary(b []byte) error {
	var res ConsistencyGroupCifsShare
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// ConsistencyGroupCifsShareInlineAclsInlineArrayItem The permissions that users and groups have on a CIFS share.
//
// swagger:model consistency_group_cifs_share_inline_acls_inline_array_item
type ConsistencyGroupCifsShareInlineAclsInlineArrayItem struct {

	// links
	Links *ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks `json:"_links,omitempty"`

	// Specifies the access rights that a user or group has on the defined CIFS Share.
	// The following values are allowed:
	// * no_access    - User does not have CIFS share access
	// * read         - User has only read access
	// * change       - User has change access
	// * full_control - User has full_control access
	//
	// Enum: ["no_access","read","change","full_control"]
	Permission *string `json:"permission,omitempty"`

	// Specifies the type of the user or group to add to the access control
	// list of a CIFS share. The following values are allowed:
	// * windows    - Windows user or group
	// * unix_user  - UNIX user
	// * unix_group - UNIX group
	//
	// Enum: ["windows","unix_user","unix_group"]
	Type *string `json:"type,omitempty"`

	// Specifies the user or group name to add to the access control list of a CIFS share.
	// Example: ENGDOMAIN\\ad_user
	UserOrGroup *string `json:"user_or_group,omitempty"`

	// Windows SID/UNIX ID depending on access-control type.
	WinSidUnixID *string `json:"win_sid_unix_id,omitempty"`
}

// Validate validates this consistency group cifs share inline acls inline array item
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validatePermission(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateType(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(m.Links) { // not required
		return nil
	}

	if m.Links != nil {
		if err := m.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links")
			}
			return err
		}
	}

	return nil
}

var consistencyGroupCifsShareInlineAclsInlineArrayItemTypePermissionPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["no_access","read","change","full_control"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		consistencyGroupCifsShareInlineAclsInlineArrayItemTypePermissionPropEnum = append(consistencyGroupCifsShareInlineAclsInlineArrayItemTypePermissionPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// consistency_group_cifs_share_inline_acls_inline_array_item
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItem
	// permission
	// Permission
	// no_access
	// END DEBUGGING
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItemPermissionNoAccess captures enum value "no_access"
	ConsistencyGroupCifsShareInlineAclsInlineArrayItemPermissionNoAccess string = "no_access"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share_inline_acls_inline_array_item
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItem
	// permission
	// Permission
	// read
	// END DEBUGGING
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItemPermissionRead captures enum value "read"
	ConsistencyGroupCifsShareInlineAclsInlineArrayItemPermissionRead string = "read"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share_inline_acls_inline_array_item
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItem
	// permission
	// Permission
	// change
	// END DEBUGGING
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItemPermissionChange captures enum value "change"
	ConsistencyGroupCifsShareInlineAclsInlineArrayItemPermissionChange string = "change"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share_inline_acls_inline_array_item
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItem
	// permission
	// Permission
	// full_control
	// END DEBUGGING
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItemPermissionFullControl captures enum value "full_control"
	ConsistencyGroupCifsShareInlineAclsInlineArrayItemPermissionFullControl string = "full_control"
)

// prop value enum
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) validatePermissionEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, consistencyGroupCifsShareInlineAclsInlineArrayItemTypePermissionPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) validatePermission(formats strfmt.Registry) error {
	if swag.IsZero(m.Permission) { // not required
		return nil
	}

	// value enum
	if err := m.validatePermissionEnum("permission", "body", *m.Permission); err != nil {
		return err
	}

	return nil
}

var consistencyGroupCifsShareInlineAclsInlineArrayItemTypeTypePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["windows","unix_user","unix_group"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		consistencyGroupCifsShareInlineAclsInlineArrayItemTypeTypePropEnum = append(consistencyGroupCifsShareInlineAclsInlineArrayItemTypeTypePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// consistency_group_cifs_share_inline_acls_inline_array_item
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItem
	// type
	// Type
	// windows
	// END DEBUGGING
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItemTypeWindows captures enum value "windows"
	ConsistencyGroupCifsShareInlineAclsInlineArrayItemTypeWindows string = "windows"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share_inline_acls_inline_array_item
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItem
	// type
	// Type
	// unix_user
	// END DEBUGGING
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItemTypeUnixUser captures enum value "unix_user"
	ConsistencyGroupCifsShareInlineAclsInlineArrayItemTypeUnixUser string = "unix_user"

	// BEGIN DEBUGGING
	// consistency_group_cifs_share_inline_acls_inline_array_item
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItem
	// type
	// Type
	// unix_group
	// END DEBUGGING
	// ConsistencyGroupCifsShareInlineAclsInlineArrayItemTypeUnixGroup captures enum value "unix_group"
	ConsistencyGroupCifsShareInlineAclsInlineArrayItemTypeUnixGroup string = "unix_group"
)

// prop value enum
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) validateTypeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, consistencyGroupCifsShareInlineAclsInlineArrayItemTypeTypePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) validateType(formats strfmt.Registry) error {
	if swag.IsZero(m.Type) { // not required
		return nil
	}

	// value enum
	if err := m.validateTypeEnum("type", "body", *m.Type); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this consistency group cifs share inline acls inline array item based on the context it is used
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if m.Links != nil {
		if err := m.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItem) UnmarshalBinary(b []byte) error {
	var res ConsistencyGroupCifsShareInlineAclsInlineArrayItem
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks consistency group cifs share inline acls inline array item inline links
//
// swagger:model consistency_group_cifs_share_inline_acls_inline_array_item_inline__links
type ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this consistency group cifs share inline acls inline array item inline links
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(m.Self) { // not required
		return nil
	}

	if m.Self != nil {
		if err := m.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this consistency group cifs share inline acls inline array item inline links based on the context it is used
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if m.Self != nil {
		if err := m.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks) UnmarshalBinary(b []byte) error {
	var res ConsistencyGroupCifsShareInlineAclsInlineArrayItemInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// ConsistencyGroupCifsShareInlineLinks consistency group cifs share inline links
//
// swagger:model consistency_group_cifs_share_inline__links
type ConsistencyGroupCifsShareInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this consistency group cifs share inline links
func (m *ConsistencyGroupCifsShareInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ConsistencyGroupCifsShareInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(m.Self) { // not required
		return nil
	}

	if m.Self != nil {
		if err := m.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this consistency group cifs share inline links based on the context it is used
func (m *ConsistencyGroupCifsShareInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ConsistencyGroupCifsShareInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if m.Self != nil {
		if err := m.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *ConsistencyGroupCifsShareInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *ConsistencyGroupCifsShareInlineLinks) UnmarshalBinary(b []byte) error {
	var res ConsistencyGroupCifsShareInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
