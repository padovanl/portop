package docker

import (
	"os"
	"strings"
	"sync"
	"time"
)

// KubeLogDir is where the kubelet links every container's log as
// <pod>_<namespace>_<container>-<container id>.log, with containerd and
// CRI-O alike. Overridable in tests.
var KubeLogDir = "/var/log/containers"

const kubeRefresh = 10 * time.Second

var kube struct {
	mu    sync.Mutex
	at    time.Time
	names map[string]string // container ID -> namespace/pod/container
}

// KubernetesName returns "namespace/pod/container" for a container run by
// the kubelet on this node, or "" when id is not one. It needs no API
// access: the names come from the kubelet's log links, re-read at most
// every kubeRefresh.
func KubernetesName(id string) string {
	if id == "" {
		return ""
	}
	kube.mu.Lock()
	defer kube.mu.Unlock()
	if kube.names == nil || time.Since(kube.at) > kubeRefresh {
		kube.names = readKubeLogDir(KubeLogDir)
		kube.at = time.Now()
	}
	return kube.names[id]
}

func readKubeLogDir(dir string) map[string]string {
	names := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names
	}
	for _, e := range entries {
		if id, name, ok := parseKubeLogName(e.Name()); ok {
			names[id] = name
		}
	}
	return names
}

// parseKubeLogName splits "<pod>_<namespace>_<container>-<id>.log". Pod,
// namespace and container names are DNS labels, which never contain '_'.
func parseKubeLogName(file string) (id, name string, ok bool) {
	base, found := strings.CutSuffix(file, ".log")
	if !found {
		return "", "", false
	}
	dash := strings.LastIndexByte(base, '-')
	if dash < 0 || !containerIDPattern.MatchString(base[dash+1:]) || len(base)-dash-1 != 64 {
		return "", "", false
	}
	parts := strings.Split(base[:dash], "_")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return base[dash+1:], parts[1] + "/" + parts[0] + "/" + parts[2], true
}
