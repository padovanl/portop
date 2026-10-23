#!/usr/bin/env python3
"""Writes desktop/frontend/logos.js: the logos the desktop app shows for
well-known servers, frameworks and languages when a process has no icon
of its own (nginx and PostgreSQL ship none; a script has none).

The logos are bundled rather than fetched while the app runs, so the
window works offline and never tells a third party what runs on the
machine. They come from two pinned packages:

  Devicon (MIT): the colored "original" SVGs.
  Simple Icons (CC0-1.0): one-color glyphs, drawn in the brand's color,
  for what Devicon lacks.

The names and logos belong to their owners; the app shows them only to
say what a process is.

    python3 scripts/gen-logos.py
"""
import json
import os
import re
import sys
import urllib.request

DEVICON = "2.17.0"
SIMPLE_ICONS = "16.35.0"
MAX_BYTES = 24 * 1024  # a bigger SVG is not worth shipping for a 30px icon
# Logos that read as dark on a dark background although some of their
# colors are light (Bash's white tile has a near-black outline).
FORCE_DARK = {"bash"}

# key: ("devicon", name) or ("si", slug). The key is what the maps
# below point to.
LOGOS = {
    # Servers, databases and tools
    "nginx": ("si", "nginx"), "apache": ("devicon", "apache"), "caddy": ("si", "caddy"),
    "traefik": ("devicon", "traefikproxy"), "envoy": ("devicon", "envoy"),
    "postgresql": ("devicon", "postgresql"), "mysql": ("devicon", "mysql"), "mariadb": ("devicon", "mariadb"),
    "mongodb": ("devicon", "mongodb"), "redis": ("devicon", "redis"), "memcached": ("devicon", "memcached"),
    "clickhouse": ("devicon", "clickhouse"), "cockroachdb": ("si", "cockroachlabs"), "influxdb": ("devicon", "influxdb"),
    "etcd": ("si", "etcd"), "neo4j": ("devicon", "neo4j"), "cassandra": ("devicon", "cassandra"),
    "rabbitmq": ("devicon", "rabbitmq"), "nats": ("devicon", "nats"), "minio": ("si", "minio"),
    "meilisearch": ("si", "meilisearch"), "qdrant": ("si", "qdrant"), "surrealdb": ("devicon", "surrealdb"),
    "docker": ("devicon", "docker"), "containerd": ("si", "containerd"), "podman": ("devicon", "podman"),
    "kubernetes": ("devicon", "kubernetes"), "grafana": ("devicon", "grafana"),
    "prometheus": ("devicon", "prometheus"), "jenkins": ("devicon", "jenkins"), "vault": ("devicon", "vault"),
    "consul": ("devicon", "consul"), "nomad": ("devicon", "nomad"), "ollama": ("si", "ollama"),
    "mosquitto": ("si", "eclipsemosquitto"), "cloudflare": ("devicon", "cloudflare"),
    "tailscale": ("si", "tailscale"), "jupyter": ("devicon", "jupyter"),
    # Frameworks
    "vite": ("devicon", "vitejs"), "nextjs": ("devicon", "nextjs"), "nuxt": ("devicon", "nuxtjs"),
    "astro": ("devicon", "astro"), "remix": ("devicon", "remix"), "svelte": ("devicon", "svelte"),
    "angular": ("devicon", "angular"), "gatsby": ("devicon", "gatsby"), "docusaurus": ("si", "docusaurus"),
    "vitepress": ("si", "vitepress"), "expo": ("devicon", "expo"), "electron": ("devicon", "electron"),
    "nestjs": ("devicon", "nestjs"), "react": ("devicon", "react"), "webpack": ("devicon", "webpack"),
    "express": ("devicon", "express"), "fastify": ("devicon", "fastify"), "hono": ("si", "hono"),
    "storybook": ("devicon", "storybook"), "cloudflareworkers": ("devicon", "cloudflareworkers"),
    "django": ("devicon", "django"), "fastapi": ("devicon", "fastapi"), "flask": ("devicon", "flask"),
    "gunicorn": ("si", "gunicorn"), "streamlit": ("devicon", "streamlit"), "rails": ("devicon", "rails"),
    "jekyll": ("devicon", "jekyll"), "laravel": ("devicon", "laravel"), "symfony": ("devicon", "symfony"),
    "hugo": ("devicon", "hugo"), "tauri": ("devicon", "tauri"), "spring": ("devicon", "spring"),
    "ktor": ("devicon", "ktor"), "phoenix": ("devicon", "phoenix"), "leptos": ("si", "leptos"), "actix": ("si", "actix"),
    # Languages and runtimes
    "python": ("devicon", "python"), "php": ("devicon", "php"), "ruby": ("devicon", "ruby"), "bash": ("devicon", "bash"),
    "nodejs": ("devicon", "nodejs"), "deno": ("devicon", "denojs"), "bun": ("devicon", "bun"), "go": ("devicon", "go"),
    "rust": ("devicon", "rust"), "perl": ("devicon", "perl"), "java": ("devicon", "java"), "dotnet": ("devicon", "dotnetcore"),
    "elixir": ("devicon", "elixir"), "dart": ("devicon", "dart"), "kotlin": ("devicon", "kotlin"), "lua": ("devicon", "lua"),
    "powershell": ("devicon", "powershell"),
}

# Process names (lowercase, without .exe) of servers that ship no icon.
PROCESSES = {
    "nginx": "nginx", "openresty": "nginx", "apache2": "apache", "httpd": "apache", "caddy": "caddy",
    "traefik": "traefik", "envoy": "envoy",
    "postgres": "postgresql", "postmaster": "postgresql", "mysqld": "mysql", "mariadbd": "mariadb",
    "mongod": "mongodb", "mongos": "mongodb", "redis-server": "redis", "memcached": "memcached",
    "clickhouse-server": "clickhouse", "clickhouse": "clickhouse", "cockroach": "cockroachdb", "influxd": "influxdb",
    "etcd": "etcd", "neo4j": "neo4j", "cassandra": "cassandra", "rabbitmq-server": "rabbitmq", "nats-server": "nats",
    "minio": "minio", "meilisearch": "meilisearch", "qdrant": "qdrant", "surreal": "surrealdb",
    "dockerd": "docker", "docker-proxy": "docker", "com.docker.backend": "docker", "docker": "docker",
    "containerd": "containerd", "podman": "podman", "kubelet": "kubernetes", "kube-proxy": "kubernetes", "k3s": "kubernetes",
    "grafana": "grafana", "grafana-server": "grafana", "prometheus": "prometheus", "jenkins": "jenkins",
    "vault": "vault", "consul": "consul", "nomad": "nomad", "ollama": "ollama", "mosquitto": "mosquitto",
    "cloudflared": "cloudflare", "tailscaled": "tailscale",
}

# Framework names as the backend reports them.
FRAMEWORKS = {
    "Vite": "vite", "Next.js": "nextjs", "Nuxt": "nuxt", "Astro": "astro", "Remix": "remix", "React Router": "remix",
    "SvelteKit": "svelte", "Angular": "angular", "Gatsby": "gatsby", "Docusaurus": "docusaurus", "VitePress": "vitepress",
    "Expo": "expo", "Electron": "electron", "NestJS": "nestjs", "Create React App": "react", "webpack": "webpack",
    "Express": "express", "Fastify": "fastify", "Hono": "hono", "Storybook": "storybook",
    "Wrangler": "cloudflareworkers", "Django": "django", "FastAPI": "fastapi", "Flask": "flask", "Gunicorn": "gunicorn",
    "Streamlit": "streamlit", "Jupyter": "jupyter", "Python http.server": "python", "Rails": "rails", "Jekyll": "jekyll",
    "Laravel": "laravel", "Symfony": "symfony", "Hugo": "hugo", "Tauri": "tauri", "Spring Boot": "spring",
    "Ktor": "ktor", "ASP.NET Core": "dotnet", "Phoenix": "phoenix", "Leptos": "leptos", "Actix Web": "actix",
}

LANGUAGES = {
    "Python": "python", "PHP": "php", "Ruby": "ruby", "Bash": "bash", "Shell": "bash", "Node.js": "nodejs",
    "Deno": "deno", "Bun": "bun", "Go": "go", "Rust": "rust", "Perl": "perl", "Java": "java", ".NET": "dotnet",
    "Elixir": "elixir", "Dart": "dart", "Kotlin": "kotlin", "Lua": "lua", "PowerShell": "powershell",
}


def fetch(url):
    with urllib.request.urlopen(url, timeout=30) as r:
        return r.read().decode()


def lightness(hex6):
    # The brightest channel: navy and near-black are dark, red is not.
    return max(int(hex6[i:i + 2], 16) for i in (0, 2, 4)) / 255


def colors(svg):
    found = re.findall(r'(?:fill|stop-color)[=:]\s*"?#([0-9a-fA-F]{3,6})', svg)
    return ["".join(c * 2 for c in h) if len(h) == 3 else h for h in found]


def minify(svg):
    svg = re.sub(r"<\?xml[^>]*>|<!--.*?-->", "", svg, flags=re.S)
    return re.sub(r">\s+<", "><", svg.strip().replace("\n", " "))


def main():
    si_data = json.loads(fetch(f"https://cdn.jsdelivr.net/npm/simple-icons@{SIMPLE_ICONS}/data/simple-icons.json"))
    si_data = si_data if isinstance(si_data, list) else si_data["icons"]
    si_hex = {}
    for icon in si_data:
        slug = icon.get("slug") or re.sub(r"[^a-z0-9]", "", icon["title"].lower().replace("+", "plus").replace(".", "dot").replace("&", "and"))
        si_hex[slug] = icon["hex"]

    svgs, dark = {}, []
    for key, (source, name) in LOGOS.items():
        if source == "devicon":
            svg = None
            for variant in ("original", "plain"):
                try:
                    candidate = minify(fetch(f"https://cdn.jsdelivr.net/npm/devicon@{DEVICON}/icons/{name}/{name}-{variant}.svg"))
                except Exception:
                    continue
                if len(candidate) <= MAX_BYTES:
                    svg = candidate
                    break
            if svg is None:
                sys.exit(f"devicon {name}: no variant under {MAX_BYTES} bytes")
            # Ids must not clash once several logos share a page.
            svg = re.sub(r'id="([^"]+)"', lambda m: f'id="{key}-{m.group(1)}"', svg)
            svg = re.sub(r"url\(#([^)]+)\)", lambda m: f"url(#{key}-{m.group(1)})", svg)
            svg = re.sub(r'href="#([^"]+)"', lambda m: f'href="#{key}-{m.group(1)}"', svg)
            cs = colors(svg)
            if not cs or all(lightness(c) < 0.36 for c in cs) or key in FORCE_DARK:
                dark.append(key)
        else:
            path = re.search(r' d="([^"]+)"', fetch(f"https://cdn.jsdelivr.net/npm/simple-icons@{SIMPLE_ICONS}/icons/{name}.svg")).group(1)
            hex6 = si_hex[name]
            svg = f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#{hex6}" d="{path}"/></svg>'
            if lightness(hex6) < 0.36:
                dark.append(key)
        svgs[key] = svg
        print(f"{key:18} {source:8} {len(svg):6} bytes{'  dark' if key in dark else ''}")

    for table in (PROCESSES, FRAMEWORKS, LANGUAGES):
        for k, v in table.items():
            assert v in LOGOS, f"{k} points to unknown logo {v}"

    out = os.path.join(os.path.dirname(__file__), "..", "desktop", "frontend", "logos.js")
    with open(out, "w") as f:
        f.write(f"""// Generated by scripts/gen-logos.py; edit that script, not this file.
// Logos of well-known servers, frameworks and languages, for a process
// with no icon of its own. From Devicon {DEVICON} (MIT) and Simple Icons
// {SIMPLE_ICONS} (CC0-1.0); the names and logos belong to their owners.
// Bundled so the window works offline and asks no one about your processes.

const SVG = {{
""")
        for key, svg in svgs.items():
            f.write(f"  {json.dumps(key)}: {json.dumps(svg)},\n")
        f.write("};\n\n")
        f.write(f"// Logos drawn in black or near-black, shown inverted (keeping their hue) in the dark theme.\nconst DARK = new Set({json.dumps(sorted(dark))});\n\n")
        f.write(f"const PROCESSES = {json.dumps(PROCESSES)};\n")
        f.write(f"const FRAMEWORKS = {json.dumps(FRAMEWORKS)};\n")
        f.write(f"const LANGUAGES = {json.dumps(LANGUAGES)};\n")
        f.write("""
const urls = new Map();

// logoFor picks a logo for a row: its server by process name, then its
// framework, then its language. It returns null when none fits.
export function logoFor(row) {
  const proc = (row.process || "").toLowerCase().replace(/\\.exe$/, "");
  const key = PROCESSES[proc] || (proc.startsWith("postgres") ? "postgresql" : "") ||
    FRAMEWORKS[row.framework] || LANGUAGES[row.language];
  if (!key) return null;
  if (!urls.has(key)) urls.set(key, "data:image/svg+xml;charset=utf-8," + encodeURIComponent(SVG[key]));
  return { key, url: urls.get(key), dark: DARK.has(key) };
}
""")
    print(f"wrote {os.path.relpath(out)}: {os.path.getsize(out)} bytes")


if __name__ == "__main__":
    main()
