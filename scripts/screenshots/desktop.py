# Screenshots of portop desktop (through its development server), each in
# light and dark. Run by scene.sh.
import os, subprocess, sys
from playwright.sync_api import sync_playwright

out = sys.argv[1]
with sync_playwright() as p:
    b = p.chromium.launch()
    # A port that opens while the window is up, for Activity.
    extra = subprocess.Popen(["/srv/bin/python3", "-m", "http.server", "8080"], cwd="/srv",
                             env={**os.environ, "SERVE_PORT": "8080", "SERVE_REPLY": "2000"},
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    errors = []
    for theme in ("dark", "light"):
        pg = b.new_page(viewport={"width": 1440, "height": 900}, color_scheme=theme)
        pg.on("pageerror", lambda e: errors.append(str(e)))
        pg.goto("http://127.0.0.1:34115/")
        pg.wait_for_timeout(6000)
        pg.screenshot(path=f"{out}/desktop-overview-{theme}.png")
        for view, wait in (("ports", 1500), ("map", 4000), ("settings", 1200)):
            pg.click(f'.nav-item[data-view="{view}"]')
            pg.wait_for_timeout(wait)
            name = {"map": "connections"}.get(view, view)
            pg.screenshot(path=f"{out}/desktop-{name}-{theme}.png")
        pg.close()
    extra.terminate()
    b.close()
    if errors:
        sys.exit("page errors: " + "; ".join(errors))
