package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const kubeID = "3f2a9c1b7e5d4a6f8b0c2d4e6f8a0b1c3d5e7f9a1b3c5d7e9f0a2b4c6d8e0f1a"

func TestParseKubeLogName(t *testing.T) {
	id, name, ok := parseKubeLogName("web-7d9f8b6c5-x2k4p_shop_nginx-" + kubeID + ".log")
	if !ok || id != kubeID || name != "shop/web-7d9f8b6c5-x2k4p/nginx" {
		t.Fatalf("got %q %q %v", id, name, ok)
	}
	for _, bad := range []string{"notes.txt", "a_b_c-123.log", "a_b-" + kubeID + ".log", "a_b_c_d-" + kubeID + ".log"} {
		if _, _, ok := parseKubeLogName(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestKubernetesNameFromKubeletLogs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coredns-5d78c9869d-abcde_kube-system_coredns-"+kubeID+".log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := KubeLogDir
	KubeLogDir = dir
	kube.names = nil
	t.Cleanup(func() { KubeLogDir = old; kube.names = nil })

	if got := KubernetesName(kubeID); got != "kube-system/coredns-5d78c9869d-abcde/coredns" {
		t.Fatalf("KubernetesName = %q", got)
	}
	if got := KubernetesName(strings.Repeat("0", 64)); got != "" {
		t.Fatalf("unknown container named %q", got)
	}
}

// A process in a Kubernetes pod carries its container ID in its cgroup,
// whichever cgroup driver the kubelet uses.
func TestContainerIDFromKubernetesCgroups(t *testing.T) {
	for _, cgroup := range []string{
		"0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1d2c_44.slice/cri-containerd-" + kubeID + ".scope\n",
		"0::/kubepods/besteffort/pod1d2c4f00/" + kubeID + "\n",
		"0::/kubepods.slice/kubepods-pod1d2c.slice/crio-" + kubeID + ".scope\n",
	} {
		if got := containerIDFromCgroupContent(cgroup); got != kubeID {
			t.Errorf("containerIDFromCgroupContent(%q) = %q", cgroup, got)
		}
	}
}
