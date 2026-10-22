package project

import "strings"

// Categories, from the most to the least specific.
const (
	CategoryDev       = "dev"       // a project's dev server, or a known framework
	CategoryDatabase  = "database"  // databases, caches and brokers
	CategoryWeb       = "web"       // web servers and proxies
	CategoryContainer = "container" // a container, or the engine's port forwarder
	CategorySystem    = "system"    // system services
	CategoryApp       = "app"       // anything else: desktop apps, tools
)

var databases = set(
	"postgres", "postmaster", "mysqld", "mariadbd", "mongod", "mongos", "redis-server", "valkey-server",
	"keydb-server", "memcached", "clickhouse-server", "clickhouse", "cockroach", "influxd", "etcd",
	"cassandra", "neo4j", "couchdb", "rethinkdb", "surreal", "sqlservr", "oracle", "tnslsnr",
	"rabbitmq-server", "beam.smp", "nats-server", "kafka", "zookeeper", "minio", "meilisearch",
	"typesense-server", "qdrant", "dragonfly", "arangod", "dgraph", "questdb", "scylla",
)

var webServers = set(
	"nginx", "apache2", "httpd", "caddy", "traefik", "haproxy", "envoy", "lighttpd", "varnishd",
	"openresty", "h2o", "php-fpm", "unitd", "squid",
)

var containerProcs = set(
	"docker-proxy", "com.docker.backend", "com.docker.vpnkit", "vpnkit", "dockerd", "containerd",
	"rootlessport", "rootlesskit", "slirp4netns", "pasta", "gvproxy", "podman", "orbstack helper",
	"limactl", "colima", "wslrelay", "kubelet", "kube-proxy", "k3s", "kind",
)

// systemUsers own services rather than people's work.
var systemUsers = set("root", "system", "nt authority\\system", "local service", "network service",
	"nt authority\\local service", "nt authority\\network service", "_mdnsresponder", "systemd-resolve",
	"systemd-network", "systemd-timesync", "daemon", "nobody", "messagebus", "avahi", "_windowserver")

// Category classifies a listening process. container and systemUnit are
// what portop already knows about it; user is its owner.
func Category(info Info, process, user, container, systemUnit string) string {
	name := normalize(process)
	switch {
	case container != "":
		return CategoryContainer
	case databases[name] || strings.HasPrefix(name, "postgres"):
		return CategoryDatabase
	case webServers[name]:
		return CategoryWeb
	case containerProcs[name]:
		return CategoryContainer
	case info.Framework != "" || info.Root != "":
		return CategoryDev
	// A login session or a user's app runs in a .scope or under user@;
	// system services are units of their own (a .service on Linux, a
	// service name on Windows).
	case systemUnit != "" && !strings.HasSuffix(systemUnit, ".scope") && !strings.HasPrefix(systemUnit, "user@"),
		systemUsers[strings.ToLower(user)],
		strings.HasPrefix(strings.ToLower(user), "_"), // macOS service accounts
		strings.HasPrefix(strings.ToLower(user), "systemd-"):
		return CategorySystem
	}
	return CategoryApp
}

func normalize(process string) string {
	p := strings.ToLower(strings.TrimSpace(process))
	return strings.TrimSuffix(p, ".exe")
}

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}
