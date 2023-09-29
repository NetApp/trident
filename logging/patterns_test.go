package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicAuthorizationRedactor(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "whitespace follows token",
			line:     "Authorization: Basic aGkgbW9tIEknbSBob21lCg== ",
			expected: "Authorization: Basic <REDACTED> ",
		},
		{
			name:     "bearer token",
			line:     "Authorization: Bearer aXQncyByYWluaW5nIGJlYXJzIG91dCB0aGVyZSB0b2RheQo=",
			expected: "Authorization: Bearer aXQncyByYWluaW5nIGJlYXJzIG91dCB0aGVyZSB0b2RheQo=",
		},
		{
			name:     "trident log line",
			line:     "time=\"2022-04-19T16:10:21Z\" level=debug msg=\"GET /api/network/ip/interfaces?fields=%2A%2A&services=data_nfs&svm.name=svm0 HTTP/1.1\\r\\nHost: 127.0.0.1\\r\\nUser-Agent: Go-http-client/1.1\\r\\nAccept: application/hal+json\\r\\nAccept: application/json\\r\\nAuthorization: Basic V2hlbiB5b3UgY29tZSB0byBhIGZvcmsgaW4gdGhlIHJvYWQsIHRha2UgaXQu\\r\\nAccept-Encoding: gzip\\r\\n\\r\\n\\n\"",
			expected: "time=\"2022-04-19T16:10:21Z\" level=debug msg=\"GET /api/network/ip/interfaces?fields=%2A%2A&services=data_nfs&svm.name=svm0 HTTP/1.1\\r\\nHost: 127.0.0.1\\r\\nUser-Agent: Go-http-client/1.1\\r\\nAccept: application/hal+json\\r\\nAccept: application/json\\r\\nAuthorization: Basic <REDACTED>\\r\\nAccept-Encoding: gzip\\r\\n\\r\\n\\n\"",
		},
		{
			name:     "quotes around header",
			line:     "\"Authorization: Basic WWVzLCBJJ20gYmVpbmcgZm9sbG93ZWQgYnkgYSBtb29uc2hhZG93\"",
			expected: "\"Authorization: Basic <REDACTED>\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := basicAuthorization.re.ReplaceAll([]byte(test.line), basicAuthorization.rep)
			assert.Equal(t, test.expected, string(actual))
		})
	}
}

func TestCSISecretsRedactor(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "simple case",
			line:     "secrets:<key:\"blah\" value:\"blah\">",
			expected: "secrets:<REDACTED>",
		},
		{
			name:     "partial gRPC with one secret",
			line:     "> >secrets:<key:\\\"previous-luks-passphrase-name\\\" value:\\\"A\\\" > volume_context:<key:\\\"backendUUID\\\" value:\\\"b7a8cfe6-1bd4-4ef6-9361-a3919c661df5\\\" > volume_context:<key:\\\"internalName\\\" value:\\\"trident_pvc_796fea63_0d7d_46aa_ab31_1c8de8894cd4\\\" > volume_context:<key:\\\"name\\\" value:\\\"pvc-796fea63-0d7d-46aa-ab31-1c8de8894cd4\\\" > volume_context:<key:\\\"protocol\\\" value:\\\"block\\\" > volume_context:<key:\\\"storage.kubernetes.io/csiProvisionerIdentity\\\" value:\\\"1662779409100-8081-csi.trident.netapp.io\\\" > \" requestID=05c3fb7e-0d5a-4d6e-b8d3-e4e7eb9054f0 requestSource=CSI",
			expected: "> >secrets:<REDACTED> volume_context:<key:\\\"backendUUID\\\" value:\\\"b7a8cfe6-1bd4-4ef6-9361-a3919c661df5\\\" > volume_context:<key:\\\"internalName\\\" value:\\\"trident_pvc_796fea63_0d7d_46aa_ab31_1c8de8894cd4\\\" > volume_context:<key:\\\"name\\\" value:\\\"pvc-796fea63-0d7d-46aa-ab31-1c8de8894cd4\\\" > volume_context:<key:\\\"protocol\\\" value:\\\"block\\\" > volume_context:<key:\\\"storage.kubernetes.io/csiProvisionerIdentity\\\" value:\\\"1662779409100-8081-csi.trident.netapp.io\\\" > \" requestID=05c3fb7e-0d5a-4d6e-b8d3-e4e7eb9054f0 requestSource=CSI",
		},
		{
			name:     "full gRPC with 4 secrets",
			line:     "time=\"2022-09-12T18:54:21Z\" level=debug msg=\"GRPC request: volume_id:\\\"pvc-796fea63-0d7d-46aa-ab31-1c8de8894cd4\\\" publish_context:<key:\\\"LUKSEncryption\\\" value:\\\"true\\\" > publish_context:<key:\\\"filesystemType\\\" value:\\\"ext4\\\" > publish_context:<key:\\\"iscsiIgroup\\\" value:\\\"ubuntu-6a1b5710-75a5-4897-bdc3-3a34005bb0ad\\\" > publish_context:<key:\\\"iscsiInterface\\\" value:\\\"\\\" > publish_context:<key:\\\"iscsiLunNumber\\\" value:\\\"0\\\" > publish_context:<key:\\\"iscsiLunSerial\\\" value:\\\"\\\" > publish_context:<key:\\\"iscsiTargetIqn\\\" value:\\\"iqn.1992-08.com.netapp:sn.19f193c511d711ecb40d001c425e872c:vs.2\\\" > publish_context:<key:\\\"iscsiTargetPortalCount\\\" value:\\\"1\\\" > publish_context:<key:\\\"mountOptions\\\" value:\\\"\\\" > publish_context:<key:\\\"p1\\\" value:\\\"10.211.55.10\\\" > publish_context:<key:\\\"protocol\\\" value:\\\"block\\\" > publish_context:<key:\\\"sharedTarget\\\" value:\\\"true\\\" > publish_context:<key:\\\"useCHAP\\\" value:\\\"false\\\" > staging_target_path:\\\"/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-796fea63-0d7d-46aa-ab31-1c8de8894cd4/globalmount\\\" volume_capability:<mount:<> access_mode:<mode:SINGLE_NODE_WRITER > > secrets:<key:\\\"luks-passphrase\\\" value:\\\"secretB\\\" > secrets:<key:\\\"luks-passphrase-name\\\" value:\\\"B\\\" > secrets:<key:\\\"previous-luks-passphrase\\\" value:\\\"secretA\\\" > secrets:<key:\\\"previous-luks-passphrase-name\\\" value:\\\"A\\\" > volume_context:<key:\\\"backendUUID\\\" value:\\\"b7a8cfe6-1bd4-4ef6-9361-a3919c661df5\\\" > volume_context:<key:\\\"internalName\\\" value:\\\"trident_pvc_796fea63_0d7d_46aa_ab31_1c8de8894cd4\\\" > volume_context:<key:\\\"name\\\" value:\\\"pvc-796fea63-0d7d-46aa-ab31-1c8de8894cd4\\\" > volume_context:<key:\\\"protocol\\\" value:\\\"block\\\" > volume_context:<key:\\\"storage.kubernetes.io/csiProvisionerIdentity\\\" value:\\\"1662779409100-8081-csi.trident.netapp.io\\\" > \" requestID=05c3fb7e-0d5a-4d6e-b8d3-e4e7eb9054f0 requestSource=CSI",
			expected: "time=\"2022-09-12T18:54:21Z\" level=debug msg=\"GRPC request: volume_id:\\\"pvc-796fea63-0d7d-46aa-ab31-1c8de8894cd4\\\" publish_context:<key:\\\"LUKSEncryption\\\" value:\\\"true\\\" > publish_context:<key:\\\"filesystemType\\\" value:\\\"ext4\\\" > publish_context:<key:\\\"iscsiIgroup\\\" value:\\\"ubuntu-6a1b5710-75a5-4897-bdc3-3a34005bb0ad\\\" > publish_context:<key:\\\"iscsiInterface\\\" value:\\\"\\\" > publish_context:<key:\\\"iscsiLunNumber\\\" value:\\\"0\\\" > publish_context:<key:\\\"iscsiLunSerial\\\" value:\\\"\\\" > publish_context:<key:\\\"iscsiTargetIqn\\\" value:\\\"iqn.1992-08.com.netapp:sn.19f193c511d711ecb40d001c425e872c:vs.2\\\" > publish_context:<key:\\\"iscsiTargetPortalCount\\\" value:\\\"1\\\" > publish_context:<key:\\\"mountOptions\\\" value:\\\"\\\" > publish_context:<key:\\\"p1\\\" value:\\\"10.211.55.10\\\" > publish_context:<key:\\\"protocol\\\" value:\\\"block\\\" > publish_context:<key:\\\"sharedTarget\\\" value:\\\"true\\\" > publish_context:<key:\\\"useCHAP\\\" value:\\\"false\\\" > staging_target_path:\\\"/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-796fea63-0d7d-46aa-ab31-1c8de8894cd4/globalmount\\\" volume_capability:<mount:<> access_mode:<mode:SINGLE_NODE_WRITER > > secrets:<REDACTED> secrets:<REDACTED> secrets:<REDACTED> secrets:<REDACTED> volume_context:<key:\\\"backendUUID\\\" value:\\\"b7a8cfe6-1bd4-4ef6-9361-a3919c661df5\\\" > volume_context:<key:\\\"internalName\\\" value:\\\"trident_pvc_796fea63_0d7d_46aa_ab31_1c8de8894cd4\\\" > volume_context:<key:\\\"name\\\" value:\\\"pvc-796fea63-0d7d-46aa-ab31-1c8de8894cd4\\\" > volume_context:<key:\\\"protocol\\\" value:\\\"block\\\" > volume_context:<key:\\\"storage.kubernetes.io/csiProvisionerIdentity\\\" value:\\\"1662779409100-8081-csi.trident.netapp.io\\\" > \" requestID=05c3fb7e-0d5a-4d6e-b8d3-e4e7eb9054f0 requestSource=CSI",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte(test.line)
			actual := csiSecrets.re.ReplaceAll(input, csiSecrets.rep)
			assert.Equal(t, test.expected, string(actual))
		})
	}
}

func TestCHAPAuthorizationRedactor(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "simple case",
			line:     "\\\"password\\\":\\\"password1234*&^%$#-+=_!@toberedacted\\\"",
			expected: "\\\"password\\\":<REDACTED>",
		},
		{
			name:     "password truncated at first '\"' ",
			line:     "\\\"password\\\":\\\"double quote\\\"\"",
			expected: "\\\"password\\\":<REDACTED>\"",
		},
		{
			name:     "simple case with wrong key",
			line:     "\"password:\":\"password1234*&^%$#toberedacted\"",
			expected: "\"password:\":\"password1234*&^%$#toberedacted\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte(test.line)
			actual := chapAuthorization.re.ReplaceAll(input, chapAuthorization.rep)
			assert.Equal(t, test.expected, string(actual))
		})
	}
}

func TestCHAPAuthorizationUserRedactor(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "simple case",
			line:     "\\\"user\\\":\\\"password1234*&^%$#-+=_!@toberedacted\\\"",
			expected: "\\\"user\\\":<REDACTED>",
		},
		{
			name:     "password truncated at first '\"' ",
			line:     "\\\"user\\\":\\\"double quote\\\"\"",
			expected: "\\\"user\\\":<REDACTED>\"",
		},
		{
			name:     "simple case with wrong key",
			line:     "\"user:\":\"password1234*&^%$#toberedacted\"",
			expected: "\"user:\":\"password1234*&^%$#toberedacted\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte(test.line)
			actual := chapAuthorizationUser.re.ReplaceAll(input, chapAuthorizationUser.rep)
			assert.Equal(t, test.expected, string(actual))
		})
	}
}

func TestBackendCreateCHAPSecretsRedactor(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "simple case",
			line:     "\\\"chapInitiatorSecret\\\":\\\"password1234*&^%$#-+=_!@toberedacted\\\"",
			expected: "\\\"chap[Target]InitiatorSecrets\\\":<REDACTED>",
		},
		{
			name:     "simple case for target secret ",
			line:     "\\\"chapTargetInitiatorSecret\\\":\\\"chaptarget123%$#@^&*\\\"",
			expected: "\\\"chap[Target]InitiatorSecrets\\\":<REDACTED>",
		},
		{
			name:     "simple case with wrong key",
			line:     "\"chapInitiatorSecret:\":\"password1234*&^%$#toberedacted\"",
			expected: "\"chapInitiatorSecret:\":\"password1234*&^%$#toberedacted\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte(test.line)
			actual := backendCreateCHAPSecrets.re.ReplaceAll(input, backendCreateCHAPSecrets.rep)
			assert.Equal(t, test.expected, string(actual))
		})
	}
}

func TestBackendCreateCHAPUsernameRedactor(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "simple case",
			line:     "\\\"chapUsername\\\":\\\"username1234*&^%$#-+=_!@toberedacted\\\"",
			expected: "\\\"chap[Target]Username\\\":<REDACTED>",
		},
		{
			name:     "simple case for target secret ",
			line:     "\\\"chapTargetUsername\\\":\\\"chaptargetusername123%$#@^&*\\\"",
			expected: "\\\"chap[Target]Username\\\":<REDACTED>",
		},
		{
			name:     "simple case with wrong key",
			line:     "\"chapUsername:\":\"password1234*&^%$#toberedacted\"",
			expected: "\"chapUsername:\":\"password1234*&^%$#toberedacted\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte(test.line)
			actual := backendCreateCHAPUsername.re.ReplaceAll(input, backendCreateCHAPUsername.rep)
			assert.Equal(t, test.expected, string(actual))
		})
	}
}

func TestBackendAuthorizationRedactor(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "simple case",
			line:     "\\\"username\\\":\\\"admin1234*&^%$#-+=_!@toberedacted\\\"",
			expected: "\\\"username\\\":<REDACTED>",
		},
		{
			name:     "password truncated at first '\"' ",
			line:     "\\\"username\\\":\\\"double quote\\\"\"",
			expected: "\\\"username\\\":<REDACTED>\"",
		},
		{
			name:     "simple case with wrong key",
			line:     "\"username:\":\"admin1234*&^%$#toberedacted\"",
			expected: "\"username:\":\"admin1234*&^%$#toberedacted\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte(test.line)
			actual := backendAuthorization.re.ReplaceAll(input, backendAuthorization.rep)
			assert.Equal(t, test.expected, string(actual))
		})
	}
}
