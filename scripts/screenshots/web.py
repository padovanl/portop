# Screenshots of the web dashboard with nginx's details open, in light and
# dark. Run by scene.sh.
import subprocess, sys, time
from playwright.sync_api import sync_playwright

out = sys.argv[1]
web = subprocess.Popen([f"{out}/portop", "--web", "--web-port", "8088"], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
url = web.stdout.readline().split()[-1]  # the URL with the session token
time.sleep(2)
with sync_playwright() as p:
    b = p.chromium.launch()
    for theme in ("dark", "light"):
        pg = b.new_page(viewport={"width": 1440, "height": 900})
        pg.goto("http://127.0.0.1:8088/")
        pg.evaluate(f"localStorage.setItem('portop-theme', '{theme}')")
        pg.goto(url)
        pg.reload()
        pg.wait_for_timeout(4000)
        pg.locator("tr", has_text="nginx").first.get_by_role("button", name="Details").click()
        pg.wait_for_timeout(1500)
        pg.screenshot(path=f"{out}/web-dashboard-{theme}.png")
        pg.close()
    b.close()
web.terminate()
