// Copyright 2026 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	versionutils "github.com/netapp/trident/utils/version"
)

func tridentRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// cli/k8s_client -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// stripHelmTemplateLines removes Helm template directives and substitutes placeholders so documents
// can be parsed as YAML for unit tests.
func stripHelmTemplateLines(content string) string {
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "{{") {
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmed, "namespace:"):
				b.WriteString("  namespace: trident\n")
			case strings.HasPrefix(trimmed, "image:"):
				b.WriteString(leadingWhitespace(line) + "image: placeholder:test\n")
			case strings.HasPrefix(trimmed, "app.kubernetes.io/managed-by:"),
				strings.HasPrefix(trimmed, "app.kubernetes.io/instance:"),
				strings.HasPrefix(trimmed, "app.kubernetes.io/version:"),
				strings.HasPrefix(trimmed, "helm.sh/chart:"):
				continue
			default:
				continue
			}
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return pruneEmptyYAMLMappingKeys(b.String())
}

// pruneEmptyYAMLMappingKeys removes mapping keys left behind when Helm template branches
// are stripped (e.g. "annotations:" with no nested keys).
func pruneEmptyYAMLMappingKeys(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "---" {
			out = append(out, line)
			continue
		}

		if !isYAMLMappingKeyWithoutValue(trimmed) {
			out = append(out, line)
			continue
		}

		keyIndent := len(line) - len(strings.TrimLeft(line, " \t"))
		hasChild := false
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if strings.TrimSpace(next) == "" {
				continue
			}
			nextIndent := len(next) - len(strings.TrimLeft(next, " \t"))
			nextTrimmed := strings.TrimSpace(next)
			if nextIndent > keyIndent {
				hasChild = true
			} else if nextIndent == keyIndent && strings.HasPrefix(nextTrimmed, "-") {
				// Kubernetes manifests often place list items at the same indent as the parent key.
				hasChild = true
			}
			break
		}
		if hasChild {
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}

func isYAMLMappingKeyWithoutValue(trimmed string) bool {
	if !strings.HasSuffix(trimmed, ":") {
		return false
	}
	// Inline mappings such as "labels:" are keys; "readOnlyRootFilesystem: true" is not.
	return !strings.Contains(trimmed, ": ")
}

func leadingWhitespace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func splitYAMLDocuments(content string) [][]byte {
	parts := strings.Split(content, "\n---")
	docs := make([][]byte, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		docs = append(docs, []byte(part))
	}
	return docs
}

func podSpecFromManifestDoc(doc []byte) (*v1.PodSpec, error) {
	var meta metav1.TypeMeta
	if err := yaml.Unmarshal(doc, &meta); err != nil {
		return nil, err
	}

	switch meta.Kind {
	case "Deployment":
		var deployment appsv1.Deployment
		if err := yaml.Unmarshal(doc, &deployment); err != nil {
			return nil, err
		}
		return &deployment.Spec.Template.Spec, nil
	case "DaemonSet":
		var daemonSet appsv1.DaemonSet
		if err := yaml.Unmarshal(doc, &daemonSet); err != nil {
			return nil, err
		}
		return &daemonSet.Spec.Template.Spec, nil
	case "Pod":
		var pod v1.Pod
		if err := yaml.Unmarshal(doc, &pod); err != nil {
			return nil, err
		}
		return &pod.Spec, nil
	case "Job":
		var job batchv1.Job
		if err := yaml.Unmarshal(doc, &job); err != nil {
			return nil, err
		}
		return &job.Spec.Template.Spec, nil
	default:
		return nil, nil
	}
}

func assertManifestFileReadOnlyRootFilesystem(t *testing.T, path string, stripHelm bool) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err, "read manifest %s", path)

	yamlContent := string(content)
	if stripHelm {
		yamlContent = stripHelmTemplateLines(yamlContent)
	}

	var checked bool
	for _, doc := range splitYAMLDocuments(yamlContent) {
		podSpec, err := podSpecFromManifestDoc(doc)
		require.NoError(t, err, "parse manifest document in %s", path)
		if podSpec == nil {
			continue
		}
		checked = true
		require.NotEmpty(t, podSpec.Containers, "expected containers in %s", path)
		assertReadOnlyRootFilesystemOnAllContainers(t, podSpec.Containers, podSpec.InitContainers)
	}

	require.True(t, checked, "no workload document with containers found in %s", path)
}

func TestStaticManifests_ReadOnlyRootFilesystem(t *testing.T) {
	root := tridentRepoRoot(t)

	t.Run("deploy operator", func(t *testing.T) {
		assertManifestFileReadOnlyRootFilesystem(t,
			filepath.Join(root, "deploy", "operator.yaml"), false)
	})

	t.Run("deploy bundle operator deployment", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(root, "deploy", "bundle.yaml"))
		require.NoError(t, err)

		var checked bool
		for _, doc := range splitYAMLDocuments(string(content)) {
			var deployment appsv1.Deployment
			if err := yaml.Unmarshal(doc, &deployment); err != nil {
				continue
			}
			if deployment.Name != "trident-operator" {
				continue
			}
			checked = true
			podSpec := deployment.Spec.Template.Spec
			assertReadOnlyRootFilesystemOnAllContainers(t, podSpec.Containers, podSpec.InitContainers)
		}
		require.True(t, checked, "trident-operator deployment not found in deploy/bundle.yaml")
	})

	t.Run("helm operator deployment", func(t *testing.T) {
		// Helm operator template uses "name:" after "command:" (not "- name:") and heavy
		// conditionals; verify via container block text rather than stripped YAML parse.
		path := filepath.Join(root, "helm", "trident-operator", "templates", "deployment.yaml")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assertContainerReadOnlyRootFilesystemInManifest(t, string(content), "trident-operator")
	})

	t.Run("helm post install upgrade hook pod", func(t *testing.T) {
		path := filepath.Join(root, "helm", "trident-operator", "templates", "postinstallupgradehook.yaml")
		content, err := os.ReadFile(path)
		require.NoError(t, err)

		stripped := stripHelmTemplateLines(string(content))
		var checked bool
		for _, doc := range splitYAMLDocuments(stripped) {
			var pod v1.Pod
			if err := yaml.Unmarshal(doc, &pod); err != nil {
				continue
			}
			if pod.Name != "trident-post-install-upgrade-hook" {
				continue
			}
			checked = true
			require.Len(t, pod.Spec.InitContainers, 1)
			require.Len(t, pod.Spec.Containers, 1)
			assertReadOnlyRootFilesystemOnAllContainers(t, pod.Spec.Containers, pod.Spec.InitContainers)
		}
		require.True(t, checked, "post-install hook pod not found in %s", path)

		raw := string(content)
		assertContainerReadOnlyRootFilesystemInManifest(t, raw, "init-container-1")
		assertContainerReadOnlyRootFilesystemInManifest(t, raw, "trident-post-hook")
	})

	t.Run("helm pre delete hook job", func(t *testing.T) {
		path := filepath.Join(root, "helm", "trident-operator", "templates", "predeletecrdshook.yaml")
		assertManifestFileReadOnlyRootFilesystem(t, path, true)
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assertContainerReadOnlyRootFilesystemInManifest(t, string(content), "pre-delete-container")
	})

	t.Run("helm post delete hook job", func(t *testing.T) {
		path := filepath.Join(root, "helm", "trident-operator", "templates", "postdeletecrdshook.yaml")
		assertManifestFileReadOnlyRootFilesystem(t, path, true)
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assertContainerReadOnlyRootFilesystemInManifest(t, string(content), "post-delete-container")
	})
}

// containerManifestBlock returns the YAML text for a container identified by its name field.
// Supports "- name: foo" (typical Kubernetes YAML) and "name: foo" under containers (Helm operator).
func containerManifestBlock(manifest, containerName string) (block string, found bool) {
	listAnchor := "- name: " + containerName
	if start := strings.Index(manifest, listAnchor); start != -1 {
		return sliceContainerManifestBlock(manifest, start, len(listAnchor))
	}

	// Bare "name:" appears in Deployment metadata and labels; only match under containers.
	containersIdx := strings.Index(manifest, "containers:")
	if containersIdx == -1 {
		return "", false
	}
	nameAnchor := "name: " + containerName
	relStart := strings.Index(manifest[containersIdx:], nameAnchor)
	if relStart == -1 {
		return "", false
	}
	start := containersIdx + relStart
	return sliceContainerManifestBlock(manifest, start, len(nameAnchor))
}

func sliceContainerManifestBlock(manifest string, start, anchorLen int) (block string, found bool) {
	rest := manifest[start+anchorLen:]
	end := len(rest)
	for _, marker := range []string{
		"\n      - name:",
		"\n    - name:",
		"\n  - name:",
		"\n      - command:",
		"\n    - command:",
		"\n      volumes:",
		"\n  volumes:",
		"\n  initContainers:",
	} {
		if idx := strings.Index(rest, marker); idx != -1 && idx < end {
			end = idx
		}
	}
	return manifest[start : start+anchorLen+end], true
}

// assertContainerReadOnlyRootFilesystemInManifest verifies a container block in raw YAML text
// sets readOnlyRootFilesystem: true (used for Helm templates that are not fully parseable).
func assertContainerReadOnlyRootFilesystemInManifest(t *testing.T, manifest, containerName string) {
	t.Helper()
	block, found := containerManifestBlock(manifest, containerName)
	require.True(t, found, "container %q not found in manifest", containerName)
	require.Contains(t, block, "readOnlyRootFilesystem: true",
		"container %q must set readOnlyRootFilesystem: true", containerName)
}

// TestGeneratedCSIDeploymentYAML_ContainsReadOnlyRootFilesystem guards the YAML template
// string directly so a missing field is caught even if unmarshaling behavior changes.
func TestGeneratedCSIDeploymentYAML_ContainsReadOnlyRootFilesystem(t *testing.T) {
	version := versionutils.MustParseSemantic("1.26.0")
	yamlData := GetCSIDeploymentYAML(&DeploymentYAMLArguments{Version: version})

	require.GreaterOrEqual(t, strings.Count(yamlData, "readOnlyRootFilesystem: true"), 6,
		"controller deployment should set readOnlyRootFilesystem on all containers")
	require.Contains(t, yamlData, "name: trident-main")
	require.Contains(t, yamlData, "name: csi-provisioner")
}

func TestGeneratedCSIDaemonSetYAMLLinux_ContainsReadOnlyRootFilesystem(t *testing.T) {
	version := versionutils.MustParseSemantic("1.26.0")
	yamlData := GetCSIDaemonSetYAMLLinux(&DaemonsetYAMLArguments{Version: version})

	require.GreaterOrEqual(t, strings.Count(yamlData, "readOnlyRootFilesystem: true"), 2)
	require.Contains(t, yamlData, "name: node-driver-registrar")
}

func TestGeneratedCSIDaemonSetYAMLWindows_ContainsReadOnlyRootFilesystem(t *testing.T) {
	version := versionutils.MustParseSemantic("1.26.0")
	yamlData := GetCSIDaemonSetYAMLWindows(&DaemonsetYAMLArguments{Version: version})

	require.GreaterOrEqual(t, strings.Count(yamlData, "readOnlyRootFilesystem: true"), 3)
	require.Contains(t, yamlData, "name: liveness-probe")
}

func TestContainerManifestBlock_HelmOperatorNotMetadataName(t *testing.T) {
	root := tridentRepoRoot(t)
	path := filepath.Join(root, "helm", "trident-operator", "templates", "deployment.yaml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	block, found := containerManifestBlock(string(content), "trident-operator")
	require.True(t, found)
	require.Contains(t, block, "readOnlyRootFilesystem: true")
	require.Contains(t, block, "securityContext:")
	// Deployment metadata also uses "name: trident-operator" — block must not stop at "containers:".
	require.NotContains(t, block, "containers:")
	require.NotContains(t, block, "namespace:")
}

// Ensure splitYAMLDocuments handles standard document separators used in Helm charts.
func TestSplitYAMLDocuments(t *testing.T) {
	input := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: a
---
apiVersion: v1
kind: Pod
metadata:
  name: b
`)
	docs := splitYAMLDocuments(string(input))
	require.Len(t, docs, 2)
	require.True(t, bytes.Contains(docs[0], []byte("name: a")))
	require.True(t, bytes.Contains(docs[1], []byte("name: b")))
}
