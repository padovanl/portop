(() => {
    "use strict";

    const byId = id => document.getElementById(id);
    const body = byId("rows");
    const filter = byId("filter");
    const dialog = byId("details");
    const themeButton = byId("theme-toggle");
    const themeKey = "portop-theme";
    let theme = "dark";
    try {
        if (localStorage.getItem(themeKey) === "light") theme = "light";
    } catch (_) {
        // The switch still works if browser storage is unavailable.
    }
    function setTheme(value) {
        theme = value;
        document.documentElement.dataset.theme = value;
        themeButton.textContent = value === "dark" ? "☀ Light" : "☾ Dark";
        themeButton.setAttribute("aria-label", `Switch to ${value === "dark" ? "light" : "dark"} theme`);
        document.querySelector('meta[name="theme-color"]').content = value === "dark" ? "#1a1b26" : "#e9eaf0";
        try { localStorage.setItem(themeKey, value); } catch (_) { /* Storage is optional. */ }
    }
    setTheme(theme);
    themeButton.addEventListener("click", () => setTheme(theme === "dark" ? "light" : "dark"));
    const tabs = [...document.querySelectorAll(".tab")];
    const fragment = new URLSearchParams(location.hash.slice(1));
    const tokenKey = "portop-token-" + location.host;
    if (fragment.has("token")) {
        sessionStorage.setItem(tokenKey, fragment.get("token"));
        history.replaceState(null, "", location.pathname + location.search);
    }
    let token = sessionStorage.getItem(tokenKey);
    let rows = [];
    let selectedState = "all";
    // UDP has no LISTEN state: a bound, unconnected UDP socket is listening.
    const isListening = row => row.state === "LISTEN" || (row.protocol === "UDP" && row.state === "UNCONN");
    let selected = null;
    let pendingForce = false;
    let refreshing = false;

    // Bidirectional controls reorder the text around them, so a process
    // named "\u202Edhss" would read as "sshd"; show them as \uFFFD instead.
    const clean = value => String(value).replace(/[\u061C\u200E\u200F\u202A-\u202E\u2066-\u2069]/g, "\uFFFD");

    function cell(tr, value, className) {
        const td = document.createElement("td");
        if (className) td.className = className;
        td.textContent = clean(value);
        tr.append(td);
        return td;
    }

    function subline(parent, value) {
        const span = document.createElement("span");
        span.className = "secondary";
        span.textContent = clean(value);
        parent.append(span);
    }

    function field(list, label, value) {
        const dt = document.createElement("dt");
        const dd = document.createElement("dd");
        dt.textContent = label;
        dd.textContent = value === "" || value == null ? "—" : clean(value);
        list.append(dt, dd);
    }

    function endpoint(address, port) {
        if (!port) return "—";
        return address.includes(":") ? `[${address}]:${port}` : `${address}:${port}`;
    }

    function render() {
        // Every word must match some field: "nginx 443" narrows to both.
        const terms = filter.value.toLowerCase().split(/\s+/).filter(Boolean);
        const matches = rows.filter(row =>
            (selectedState === "all" || (selectedState === "LISTEN" ? isListening(row) : row.state === selectedState)) &&
            terms.every(term => [row.local_port, row.local_address, row.remote_port, row.remote_address,
                row.remote_host, row.pid, row.process, row.protocol, row.state,
                row.systemd_unit, row.container].some(value =>
                String(value || "").toLowerCase().includes(term))));
        const fragment = document.createDocumentFragment();

        for (const row of matches) {
            const tr = document.createElement("tr");
            const local = cell(tr, ":" + row.local_port, "primary port");
            if (isListening(row)) local.append(scopeBadge(row.scope));
            subline(local, row.local_address + " · " + row.protocol + (row.ipv6 ? "6" : "4"));
            const remote = cell(tr, endpoint(row.remote_address, row.remote_port), row.remote_port ? "" : "muted");
            if (row.remote_host) subline(remote, row.remote_host);
            const stateCell = cell(tr, "");
            const badge = document.createElement("span");
            badge.className = "state " + row.state.toLowerCase();
            badge.textContent = row.state;
            stateCell.append(badge);
            const process = cell(tr, row.process || "Unknown", row.process ? "primary" : "muted");
            const workers = row.workers ? row.workers.length : 0;
            if (row.pid) subline(process, "PID " + row.pid + (workers ? ` · +${workers} worker${workers > 1 ? "s" : ""}` : "") + (row.user ? " · " + row.user : ""));
            else if (row.user) subline(process, row.user);
            cell(tr, row.pid ? row.cpu_percent.toFixed(1) + "%" : "—", row.pid ? "" : "muted");
            const owner = cell(tr, row.systemd_unit || row.container || "—", row.systemd_unit || row.container ? "" : "muted");
            if (row.systemd_unit && row.container) subline(owner, row.container);
            const action = cell(tr, "");
            const button = document.createElement("button");
            button.type = "button";
            button.className = "detail-button";
            button.textContent = "Details";
            button.setAttribute("aria-label", `Details for ${row.protocol} port ${row.local_port}`);
            button.addEventListener("click", () => openDetail(row));
            action.append(button);
            fragment.append(tr);
        }

        body.replaceChildren(fragment);
        byId("visible-count").textContent = matches.length;
        byId("empty").hidden = matches.length !== 0;
    }

    const scopeLabels = { all: "all interfaces", address: "one address", loopback: "loopback" };

    // Who can reach the port: red for every interface, amber for one
    // address, green for loopback only.
    function scopeBadge(scope) {
        const badge = document.createElement("span");
        badge.className = "scope " + (scopeLabels[scope] ? scope : "loopback");
        badge.textContent = scopeLabels[scope] || scope;
        badge.title = "Reachable on " + (scopeLabels[scope] || scope);
        return badge;
    }

    function showSocket(row) {
        const fields = byId("socket-fields");
        fields.replaceChildren();
        byId("detail-title").textContent = `${row.protocol} :${row.local_port}`;
        field(fields, "Local", endpoint(row.local_address, row.local_port));
        field(fields, "Remote", endpoint(row.remote_address, row.remote_port));
        field(fields, "Remote host", row.remote_host);
        field(fields, "Protocol", row.protocol + (row.ipv6 ? " · IPv6" : " · IPv4"));
        field(fields, "State", row.state);
        field(fields, "Exposure", scopeLabels[row.scope] || row.scope);
        field(fields, "Process", row.process);
        field(fields, "User", row.user);
        field(fields, "Workers", row.workers && row.workers.length ? row.workers.join(", ") : "");
        field(fields, "PID", row.pid || "—");
        field(fields, "CPU", row.pid ? row.cpu_percent.toFixed(1) + "%" : "—");
        field(fields, "Systemd unit", row.systemd_unit);
        field(fields, "Container", row.container);
    }

    function showProcess(info) {
        const fields = byId("process-fields");
        fields.replaceChildren();
        field(fields, "Name", info.name);
        field(fields, "User", info.user);
        field(fields, "Command", info.cmdline);
        field(fields, "Executable", info.exe);
        field(fields, "Directory", info.cwd);
        field(fields, "Memory", info.rss_bytes ? (info.rss_bytes / 1048576).toFixed(1) + " MiB" : "—");
        field(fields, "Threads", info.num_threads || "—");
        field(fields, "Open files", info.open_files < 0 ? "—" : info.open_files);
        field(fields, "Started", info.start_time && !info.start_time.startsWith("0001-") ? new Date(info.start_time).toLocaleString() : "—");
    }

    async function openDetail(row) {
        selected = { row, info: null };
        showSocket(row);
        byId("process-fields").replaceChildren();
        byId("process-status").textContent = row.pid ? "Loading process details…" : "Process owner is unavailable.";
        byId("action-status").textContent = "";
        byId("confirm-action").hidden = true;
        byId("terminate").disabled = true;
        byId("force-kill").disabled = true;
        dialog.showModal();
        if (!row.pid) return;
        if (!token) {
            byId("process-status").textContent = "Open the URL printed by portop to inspect and manage processes.";
            return;
        }
        try {
            const response = await fetch(`/api/process/${row.pid}`, { headers: { "X-Portop-Token": token }, cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()).trim());
            const info = await response.json();
            if (!selected || selected.row !== row || !dialog.open) return;
            selected.info = info;
            byId("process-status").textContent = "";
            showProcess(info);
            const actionable = info.start_time && !info.start_time.startsWith("0001-");
            byId("terminate").disabled = !actionable;
            byId("force-kill").disabled = !actionable;
        } catch (error) {
            if (selected && selected.row === row && dialog.open) byId("process-status").textContent = error.message;
        }
    }

    function askToSignal(force) {
        if (!selected?.info) return;
        pendingForce = force;
        byId("confirm-text").textContent = `${force ? "Force kill (SIGKILL)" : "Terminate (SIGTERM)"} ${clean(selected.info.name || selected.row.process || "process")} (PID ${selected.row.pid})? All of its connections may close.`;
        byId("confirm-button").textContent = force ? "Force kill" : "Terminate";
        byId("confirm-action").hidden = false;
    }

    async function signalSelected() {
        if (!selected?.info || !token) return;
        const { row, info } = selected;
        byId("confirm-button").disabled = true;
        try {
            const response = await fetch(`/api/process/${row.pid}/signal`, {
                method: "POST",
                headers: { "Content-Type": "application/json", "X-Portop-Token": token },
                body: JSON.stringify({ start_time: info.start_time, force: pendingForce })
            });
            if (!response.ok) throw new Error((await response.text()).trim());
            byId("confirm-action").hidden = true;
            byId("terminate").disabled = true;
            byId("force-kill").disabled = true;
            byId("action-status").classList.remove("error");
            byId("action-status").textContent = pendingForce ? "SIGKILL sent" : "SIGTERM sent";
            refresh();
        } catch (error) {
            byId("action-status").classList.add("error");
            byId("action-status").textContent = error.message;
        } finally {
            byId("confirm-button").disabled = false;
        }
    }

    // With --web-auth the signed-in session replaces the token in the URL.
    async function loadSession() {
        try {
            const response = await fetch("/api/session", { cache: "no-store" });
            if (!response.ok) return;
            const session = await response.json();
            token = session.token;
            byId("session-user").textContent = session.user;
            byId("session").hidden = false;
            if (session.host) byId("mode-label").textContent = session.host.toUpperCase() + " · LIVE";
        } catch (_) {
            // Without a session endpoint the dashboard runs without sign-in.
        }
    }

    async function refresh() {
        if (refreshing || document.hidden) return;
        refreshing = true;
        try {
            const response = await fetch("/api/ports", { cache: "no-store" });
            if (response.status === 401) {
                location.assign("/login");
                return;
            }
            if (!response.ok) throw new Error("port scan failed");
            rows = await response.json();
            const listening = rows.filter(isListening).length;
            byId("total-count").textContent = rows.length;
            byId("listen-count").textContent = listening;
            byId("process-count").textContent = new Set(rows.map(row => row.pid).filter(Boolean)).size;
            byId("all-count").textContent = rows.length;
            byId("listening-count").textContent = listening;
            byId("connected-count").textContent = rows.filter(row => row.state === "ESTABLISHED").length;
            byId("status").classList.remove("error");
            byId("status").textContent = "● Updated " + new Date().toLocaleTimeString();
            render();
        } catch (error) {
            byId("status").classList.add("error");
            byId("status").textContent = "● " + error.message;
        } finally {
            refreshing = false;
        }
    }

    filter.addEventListener("input", render);
    for (const tab of tabs) tab.addEventListener("click", () => {
        selectedState = tab.dataset.state;
        for (const item of tabs) {
            item.classList.toggle("active", item === tab);
            item.setAttribute("aria-pressed", item === tab);
        }
        render();
    });
    byId("close-details").addEventListener("click", () => dialog.close());
    dialog.addEventListener("close", () => { if (!dialog.open) selected = null; });
    byId("terminate").addEventListener("click", () => askToSignal(false));
    byId("force-kill").addEventListener("click", () => askToSignal(true));
    byId("cancel-action").addEventListener("click", () => { byId("confirm-action").hidden = true; });
    byId("confirm-button").addEventListener("click", signalSelected);
    document.addEventListener("visibilitychange", refresh);
    document.addEventListener("keydown", event => {
        if (event.key === "/" && !dialog.open && document.activeElement !== filter) {
            event.preventDefault();
            filter.focus();
        }
    });
    loadSession().then(refresh);
    setInterval(refresh, 2000);
})();
