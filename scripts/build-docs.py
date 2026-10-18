#!/usr/bin/env python3
"""Build the documentation pages in docs/docs/ from docs-src/.

Each file in docs-src/ is the body of one page and starts with its <h1>.
This script wraps every page in the shared layout (header, navigation,
"On this page", previous/next links) and writes a search index. The output
is committed, since GitHub Pages serves docs/ as it is.

    python3 scripts/build-docs.py           # write docs/docs/
    python3 scripts/build-docs.py --check   # fail if docs/docs/ is stale
"""

import argparse
import html
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "docs-src"
OUT = ROOT / "docs" / "docs"

# Navigation: (group, [(page, title in the sidebar)]).
NAV = [
    ("Getting started", [
        ("index", "Introduction"),
        ("installation", "Installation"),
        ("first-run", "First run and permissions"),
    ]),
    ("Using portop", [
        ("the-table", "The table"),
        ("command-line", "Command line"),
        ("scripting", "Scripting"),
        ("configuration", "Configuration"),
    ]),
    ("Checks", [
        ("baseline", "Baseline drift"),
        ("compose", "Compose audit"),
        ("notifications", "New-port alerts"),
    ]),
    ("Context", [
        ("containers", "Containers and workers"),
        ("firewall", "Firewall"),
        ("traffic", "Traffic"),
    ]),
    ("Dashboards", [
        ("web-dashboard", "Web dashboard"),
        ("cockpit", "Cockpit add-on"),
    ]),
    ("Reference", [
        ("platforms", "Platform support"),
        ("how-it-works", "How it works"),
        ("troubleshooting", "Troubleshooting"),
    ]),
]

HEADING = re.compile(r'<h([23]) id="([^"]+)">(.*?)</h\1>', re.S)
TAG = re.compile(r"<[^>]+>")


def text(fragment):
    return html.unescape(re.sub(r"\s+", " ", TAG.sub(" ", fragment))).strip()


def page_title(body, page):
    m = re.search(r"<h1>(.*?)</h1>", body, re.S)
    if not m:
        sys.exit(f"docs-src/{page}.html has no <h1>")
    return text(m.group(1))


def sidebar(current):
    out = []
    for group, pages in NAV:
        out.append(f'      <p class="nav-group">{html.escape(group)}</p>\n      <ul>')
        for page, label in pages:
            attr = ' class="active" aria-current="page"' if page == current else ""
            out.append(f'        <li><a href="{page}.html"{attr}>{html.escape(label)}</a></li>')
        out.append("      </ul>")
    return "\n".join(out)


def on_this_page(body):
    items = []
    for level, anchor, label in HEADING.findall(body):
        cls = ' class="sub"' if level == "3" else ""
        items.append(f'        <li{cls}><a href="#{anchor}">{html.escape(text(label))}</a></li>')
    if not items:
        return ""
    return ('    <nav class="page-toc" aria-label="On this page">\n      <p>On this page</p>\n      <ul>\n'
            + "\n".join(items) + "\n      </ul>\n    </nav>")


def pager(order, i):
    links = []
    if i > 0:
        page, label = order[i - 1]
        links.append(f'<a class="prev" href="{page}.html"><span>Previous</span>{html.escape(label)}</a>')
    else:
        links.append("<span></span>")
    if i + 1 < len(order):
        page, label = order[i + 1]
        links.append(f'<a class="next" href="{page}.html"><span>Next</span>{html.escape(label)}</a>')
    return '      <nav class="pager" aria-label="Pages">' + "".join(links) + "</nav>"


def search_entries(page, title, body):
    """One entry per section, with the words a search can match."""
    entries = []
    parts = HEADING.split(body)
    # parts: [intro, level, id, label, content, level, id, label, content, ...]
    intro = re.sub(r"<h1>.*?</h1>", "", parts[0], flags=re.S)
    entries.append({"page": page, "title": title, "section": "", "anchor": "", "text": text(intro)[:300]})
    for k in range(1, len(parts), 4):
        entries.append({"page": page, "title": title, "section": text(parts[k + 2]),
                        "anchor": parts[k + 1], "text": text(parts[k + 3])[:300]})
    return entries


TEMPLATE = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title} · portop docs</title>
<meta name="description" content="{description}">
<link rel="icon" href="../assets/logo.png">
<link rel="stylesheet" href="docs.css">
<script>try {{ var t = localStorage.getItem("portop-docs-theme"); if (t) document.documentElement.dataset.theme = t; }} catch (e) {{}}</script>
<script defer src="docs.js"></script>
</head>
<body>
<header class="topbar">
  <button class="nav-toggle" type="button" aria-controls="sidebar" aria-expanded="false" aria-label="Menu">
    <svg width="20" height="20" viewBox="0 0 20 20" aria-hidden="true"><path d="M3 5h14M3 10h14M3 15h14" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
  </button>
  <a class="brand" href="index.html"><img src="../assets/logo.png" alt="" width="24" height="24">portop <span>docs</span></a>
  <div class="search">
    <input id="search" type="search" placeholder="Search the docs" autocomplete="off" aria-label="Search the docs">
    <ul id="search-results" class="search-results" hidden></ul>
  </div>
  <nav class="topnav" aria-label="Site">
    <a href="../">Home</a>
    <a href="https://github.com/padovanl/portop">GitHub</a>
  </nav>
  <button class="theme-toggle" type="button" aria-label="Toggle dark mode">
    <svg width="18" height="18" viewBox="0 0 20 20" aria-hidden="true"><path d="M10 3a7 7 0 1 0 7 7 5.5 5.5 0 0 1-7-7z" fill="currentColor"/></svg>
  </button>
</header>
<div class="layout">
  <aside class="sidebar" id="sidebar" aria-label="Documentation">
{sidebar}
  </aside>
  <div class="content">
    <main>
{body}
{pager}
    </main>
{toc}
  </div>
</div>
<footer class="footer">portop · MIT License · <a href="https://github.com/padovanl/portop/edit/develop/docs-src/{page}.html">Edit this page</a></footer>
</body>
</html>
"""


def build():
    order = [p for _, pages in NAV for p in pages]
    known = {p for p, _ in order}
    sources = {p.stem for p in SRC.glob("*.html")}
    if sources != known:
        sys.exit(f"docs-src/ and NAV disagree: only in docs-src {sorted(sources - known)}, only in NAV {sorted(known - sources)}")

    files, index = {}, []
    for i, (page, _) in enumerate(order):
        body = (SRC / f"{page}.html").read_text().strip()
        title = page_title(body, page)
        lede = re.search(r'<p class="lede">(.*?)</p>', body, re.S)
        description = text(lede.group(1)) if lede else title
        for anchor in re.findall(r'href="(?:([a-z-]+)\.html)?#([^"]+)"', body):
            target = anchor[0] or page
            target_body = (SRC / f"{target}.html").read_text() if target in known else ""
            if f'id="{anchor[1]}"' not in target_body:
                sys.exit(f"docs-src/{page}.html links to {target}.html#{anchor[1]}, which does not exist")
        files[f"{page}.html"] = TEMPLATE.format(
            title=html.escape(title), description=html.escape(description, quote=True),
            sidebar=sidebar(page), body=body, pager=pager(order, i),
            toc=on_this_page(body), page=page)
        index.extend(search_entries(page, title, body))
    files["search.json"] = json.dumps(index, ensure_ascii=False, indent=1) + "\n"
    return files


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--check", action="store_true", help="fail if the built pages differ from docs/docs/")
    args = parser.parse_args()
    files = build()
    stale = [name for name, content in files.items()
             if not (OUT / name).exists() or (OUT / name).read_text() != content]
    if args.check:
        if stale:
            sys.exit("docs/docs/ is out of date; run python3 scripts/build-docs.py: " + ", ".join(stale))
        return
    OUT.mkdir(parents=True, exist_ok=True)
    for name, content in files.items():
        (OUT / name).write_text(content)
    print(f"wrote {len(files)} files to {OUT.relative_to(ROOT)} ({len(stale)} changed)")


if __name__ == "__main__":
    main()
