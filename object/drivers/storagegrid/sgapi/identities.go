// Copyright 2026 NetApp, Inc. All Rights Reserved.

package sgapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// S3Statement is one statement in a group's S3 policy. Both Action and Resource
// accept a bare string when there is a single element, but arrays are always
// emitted here to keep the serialisation uniform.
type S3Statement struct {
	Sid         string   `json:"Sid,omitempty"`
	Effect      string   `json:"Effect"`
	Action      []string `json:"Action"`
	Resource    []string `json:"Resource,omitempty"`
	NotResource []string `json:"NotResource,omitempty"`
}

// S3Policy is the group policy that scopes access to specific buckets.
type S3Policy struct {
	ID        string        `json:"Id,omitempty"`
	Version   string        `json:"Version,omitempty"`
	Statement []S3Statement `json:"Statement"`
}

// GroupPolicies holds a group's permissions. Management is deliberately never
// populated by this client: manageAllContainers overrides group and bucket
// policies, and manageOwnS3Credentials would let an identity mint its own keys,
// so either one would defeat the scoping that S3 provides.
type GroupPolicies struct {
	Management map[string]any `json:"management,omitempty"`
	S3         *S3Policy      `json:"s3,omitempty"`
}

// Group is a tenant group. AccountID, GroupURN, and Federated are
// server-generated and required on a PUT, which is why an update is a
// read-modify-write rather than a fresh body.
type Group struct {
	ID          string          `json:"id,omitempty"`
	AccountID   json.RawMessage `json:"accountId,omitempty"`
	DisplayName string          `json:"displayName,omitempty"`
	UniqueName  string          `json:"uniqueName,omitempty"`
	GroupURN    string          `json:"groupURN,omitempty"`
	Federated   bool            `json:"federated,omitempty"`
	Policies    *GroupPolicies  `json:"policies,omitempty"`
}

// GroupCreate is the POST /org/groups request body.
type GroupCreate struct {
	UniqueName  string         `json:"uniqueName"`
	DisplayName string         `json:"displayName"`
	Policies    *GroupPolicies `json:"policies,omitempty"`
}

// User is a tenant user. MemberOf carries group UUIDs.
type User struct {
	ID         string          `json:"id,omitempty"`
	AccountID  json.RawMessage `json:"accountId,omitempty"`
	FullName   string          `json:"fullName,omitempty"`
	UniqueName string          `json:"uniqueName,omitempty"`
	UserURN    string          `json:"userURN,omitempty"`
	Federated  bool            `json:"federated,omitempty"`
	MemberOf   []string        `json:"memberOf,omitempty"`
	Disable    bool            `json:"disable,omitempty"`
}

// UserCreate is the POST /org/users request body.
type UserCreate struct {
	UniqueName string   `json:"uniqueName"`
	FullName   string   `json:"fullName"`
	MemberOf   []string `json:"memberOf,omitempty"`
}

// AccessKey is an S3 credential pair. AccessKey and SecretAccessKey are
// populated only on the response that creates the key; a later read returns an
// obfuscated DisplayName and nothing else, which is why a key can never be
// recovered after the fact.
type AccessKey struct {
	ID              string          `json:"id,omitempty"`
	AccountID       json.RawMessage `json:"accountId,omitempty"`
	DisplayName     string          `json:"displayName,omitempty"`
	UserURN         string          `json:"userURN,omitempty"`
	UserUUID        string          `json:"userUUID,omitempty"`
	Expires         *string         `json:"expires,omitempty"`
	AccessKey       string          `json:"accessKey,omitempty"`
	SecretAccessKey string          `json:"secretAccessKey,omitempty"`
}

// accessKeyCreate is the POST body for minting a key. Expires has no omitempty
// because a nil expiry has to appear on the wire as an explicit null, which is
// what tells the grid the key never expires.
type accessKeyCreate struct {
	Expires *string `json:"expires"`
}

const (
	groupsPath = "/org/groups"
	usersPath  = "/org/users"
)

// GroupUniqueName derives a group's unique name from a COSI account name.
func GroupUniqueName(accountName string) string { return "group/" + accountName }

// UserUniqueName derives a user's unique name from a COSI account name.
func UserUniqueName(accountName string) string { return "user/" + accountName }

// CreateGroup creates a tenant group and returns it with the server-assigned
// UUID.
func (c *Client) CreateGroup(ctx context.Context, spec GroupCreate) (*Group, error) {
	resp, err := c.doRetrying(ctx, request{
		method: http.MethodPost,
		path:   groupsPath,
		body:   spec,
	})
	if err != nil {
		return nil, err
	}
	var group Group
	if err := decodeData(resp, &group); err != nil {
		return nil, fmt.Errorf("creating group %q: %w", spec.UniqueName, err)
	}
	return &group, nil
}

// GetGroupByShortName reads a group by its short name, which is the part of the
// unique name after the "group/" prefix. The raw map is returned alongside the
// typed group so that an update can put back every field the grid sent,
// including any this client does not model.
func (c *Client) GetGroupByShortName(
	ctx context.Context, shortName string,
) (*Group, map[string]any, error) {
	var group Group
	raw, err := c.getIdentity(ctx, escapePath("org", "groups", "group", shortName), &group)
	if err != nil {
		return nil, nil, err
	}
	return &group, raw, nil
}

// GetUserByShortName reads a user by its short name, with the same raw-map
// contract as GetGroupByShortName.
func (c *Client) GetUserByShortName(
	ctx context.Context, shortName string,
) (*User, map[string]any, error) {
	var user User
	raw, err := c.getIdentity(ctx, escapePath("org", "users", "user", shortName), &user)
	if err != nil {
		return nil, nil, err
	}
	return &user, raw, nil
}

// getIdentity reads an identity object into both a typed value and the generic
// form an update needs.
func (c *Client) getIdentity(ctx context.Context, path string, typed any) (map[string]any, error) {
	resp, err := c.doRetrying(ctx, request{method: http.MethodGet, path: path})
	if err != nil {
		return nil, err
	}

	raw := make(map[string]any)
	if err := decodeData(resp, &raw); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := remarshal(raw, typed); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return raw, nil
}

// SetGroupS3Policy replaces a group's S3 policy, leaving every other field as
// the grid reported it. PUT /org/groups/{id} is a full replacement that requires
// the server-generated fields, so this reads the group, overwrites only
// policies.s3, and puts the whole object back.
func (c *Client) SetGroupS3Policy(
	ctx context.Context, shortName string, policy *S3Policy,
) (*Group, error) {
	group, raw, err := c.GetGroupByShortName(ctx, shortName)
	if err != nil {
		return nil, err
	}
	if group.ID == "" {
		return nil, fmt.Errorf("group %q was read but reported no id", shortName)
	}

	policies, _ := raw["policies"].(map[string]any)
	if policies == nil {
		policies = make(map[string]any)
	}
	encoded, err := toGeneric(policy)
	if err != nil {
		return nil, fmt.Errorf("encoding the S3 policy for group %q: %w", shortName, err)
	}
	policies["s3"] = encoded
	raw["policies"] = policies

	resp, err := c.doRetrying(ctx, request{
		method: http.MethodPut,
		path:   escapePath("org", "groups", group.ID),
		body:   raw,
	})
	if err != nil {
		return nil, err
	}
	var updated Group
	if decodeData(resp, &updated) == nil && updated.ID != "" {
		return &updated, nil
	}
	// Some endpoints answer a PUT with an empty body; the group is still the one
	// that was just written.
	return group, nil
}

// CreateUser creates a tenant user and returns it with the server-assigned UUID.
func (c *Client) CreateUser(ctx context.Context, spec UserCreate) (*User, error) {
	resp, err := c.doRetrying(ctx, request{
		method: http.MethodPost,
		path:   usersPath,
		body:   spec,
	})
	if err != nil {
		return nil, err
	}
	var user User
	if err := decodeData(resp, &user); err != nil {
		return nil, fmt.Errorf("creating user %q: %w", spec.UniqueName, err)
	}
	return &user, nil
}

// EnsureUserInGroup makes sure an existing user is a member of the given group,
// putting the user back only when the membership is actually missing.
func (c *Client) EnsureUserInGroup(
	ctx context.Context, shortName, groupID string,
) (*User, error) {
	user, raw, err := c.GetUserByShortName(ctx, shortName)
	if err != nil {
		return nil, err
	}
	if user.ID == "" {
		return nil, fmt.Errorf("user %q was read but reported no id", shortName)
	}
	for _, member := range user.MemberOf {
		if member == groupID {
			return user, nil
		}
	}

	raw["memberOf"] = append(asStrings(raw["memberOf"]), groupID)
	resp, err := c.doRetrying(ctx, request{
		method: http.MethodPut,
		path:   escapePath("org", "users", user.ID),
		body:   raw,
	})
	if err != nil {
		return nil, err
	}
	var updated User
	if decodeData(resp, &updated) == nil && updated.ID != "" {
		return &updated, nil
	}
	user.MemberOf = append(user.MemberOf, groupID)
	return user, nil
}

// DeleteGroup removes a group. A missing group is reported as success, since
// revocation has to be idempotent.
func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	return c.deleteTolerant(ctx, escapePath("org", "groups", id))
}

// DeleteUser removes a user, with the same not-found-is-success rule as
// DeleteGroup. Deleting a user is expected to remove its access keys too.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.deleteTolerant(ctx, escapePath("org", "users", id))
}

func (c *Client) deleteTolerant(ctx context.Context, path string) error {
	_, err := c.doRetrying(ctx, request{method: http.MethodDelete, path: path})
	if err != nil && !IsNotFound(err) {
		return err
	}
	return nil
}

// ListAccessKeys enumerates a user's S3 credentials. The secrets are not
// included: only an obfuscated display name comes back.
func (c *Client) ListAccessKeys(ctx context.Context, userID string) ([]AccessKey, error) {
	resp, err := c.doRetrying(ctx, request{
		method: http.MethodGet,
		path:   escapePath("org", "users", userID, "s3-access-keys"),
	})
	if err != nil {
		return nil, err
	}
	var keys []AccessKey
	if err := decodeData(resp, &keys); err != nil {
		return nil, fmt.Errorf("listing access keys of user %q: %w", userID, err)
	}
	return keys, nil
}

// CreateAccessKey mints a new S3 credential pair on a user. This response is the
// only time the secret is ever visible, so a caller that loses it has to delete
// the key and mint another.
//
// A nil expires means the key never expires, which is the only safe choice while
// nothing in COSI rotates a credential before it lapses.
func (c *Client) CreateAccessKey(
	ctx context.Context, userID string, expires *string,
) (*AccessKey, error) {
	resp, err := c.doRetrying(ctx, request{
		method: http.MethodPost,
		path:   escapePath("org", "users", userID, "s3-access-keys"),
		body:   accessKeyCreate{Expires: expires},
	})
	if err != nil {
		return nil, err
	}
	var key AccessKey
	if err := decodeData(resp, &key); err != nil {
		return nil, fmt.Errorf("creating an access key for user %q: %w", userID, err)
	}
	if key.AccessKey == "" || key.SecretAccessKey == "" {
		return nil, fmt.Errorf(
			"creating an access key for user %q: the response carried no credentials, so the key "+
				"cannot be recovered and must be deleted", userID)
	}
	return &key, nil
}

// DeleteAccessKey removes one credential. The path parameter accepts either the
// access key itself or the credential pair identifier.
func (c *Client) DeleteAccessKey(ctx context.Context, userID, accessKey string) error {
	return c.deleteTolerant(ctx, escapePath("org", "users", userID, "s3-access-keys", accessKey))
}

// DeleteAllAccessKeys removes every credential on a user and reports how many it
// deleted. Grant calls this before minting, which is what guarantees exactly one
// valid credential exists when a grant returns, and leaves no orphan key behind
// if a previous attempt died after minting but before returning the secret.
func (c *Client) DeleteAllAccessKeys(ctx context.Context, userID string) (int, error) {
	keys, err := c.ListAccessKeys(ctx, userID)
	if err != nil {
		return 0, err
	}
	for _, key := range keys {
		// The list reports the credential pair identifier in id; the access key
		// itself is not returned, so id is the only usable handle.
		handle := key.ID
		if handle == "" {
			handle = key.AccessKey
		}
		if handle == "" {
			continue
		}
		if err := c.DeleteAccessKey(ctx, userID, handle); err != nil {
			return 0, err
		}
	}
	return len(keys), nil
}

// remarshal converts a decoded generic value into a typed one.
func remarshal(from, to any) error {
	encoded, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, to)
}

// toGeneric converts a typed value into the generic form used for a
// read-modify-write body.
func toGeneric(from any) (any, error) {
	encoded, err := json.Marshal(from)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func asStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
