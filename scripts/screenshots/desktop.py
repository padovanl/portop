# Screenshots of portop desktop (through its development server), each in
# light and dark. Run by scene.sh.
import json, os, subprocess, sys, urllib.request
from playwright.sync_api import sync_playwright

out = sys.argv[1]
with sync_playwright() as p:
    b = p.chromium.launch()
    def api(name, *args):
        req = urllib.request.Request("http://127.0.0.1:34115/api/call",
                                     data=json.dumps({"name": "github.com/padovanl/portop/desktop/backend.Backend." + name, "args": list(args)}).encode(),
                                     headers={"Content-Type": "application/json"})
        return json.load(urllib.request.urlopen(req))
    # The baseline is saved first; the port that opens next is the change
    # it reports, and an Activity entry. A Compose project is checked once
    # so it is in the recent list.
    api("SaveBaseline")
    extra = subprocess.Popen(["/srv/bin/python3", "-m", "http.server", "8080"], cwd="/srv",
                             env={**os.environ, "SERVE_PORT": "8080", "SERVE_REPLY": "2000"},
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    api("ComposeAudit", "/srv/dev/shop-stack")
    errors = []
    for theme in ("dark", "light"):
        pg = b.new_page(viewport={"width": 1440, "height": 900}, color_scheme=theme)
        pg.on("pageerror", lambda e: errors.append(str(e)))
        pg.goto("http://127.0.0.1:34115/")
        pg.wait_for_timeout(6000)
        pg.screenshot(path=f"{out}/desktop-overview-{theme}.png")
        for view, wait in (("ports", 1500), ("map", 4000), ("projects", 1500), ("activity", 1500), ("settings", 1200)):
            if view == "projects":
                pg.click('.nav-item[data-view="projects"]'); pg.wait_for_timeout(1200)
                pg.click("#compose-recent .chip"); pg.wait_for_timeout(2500)
                pg.screenshot(path=f"{out}/desktop-projects-{theme}.png")
                continue
            pg.click(f'.nav-item[data-view="{view}"]')
            pg.wait_for_timeout(wait)
            name = {"map": "connections"}.get(view, view)
            pg.screenshot(path=f"{out}/desktop-{name}-{theme}.png")
        pg.close()
    extra.terminate()
    b.close()
    if errors:
        sys.exit("page errors: " + "; ".join(errors))
