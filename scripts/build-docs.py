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
        ("projects", "Projects and frameworks"),
        ("containers", "Containers and workers"),
        ("firewall", "Firewall"),
        ("traffic", "Traffic"),
    ]),
    ("Apps and dashboards", [
        ("desktop", "Desktop app"),
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


def group_of(page):
    for group, pages in NAV:
        if any(p == page for p, _ in pages):
            return group
    return ""


def sidebar(current):
    out = ['      <ul class="menu-list">']
    for group, pages in NAV:
        active_group = any(p == current for p, _ in pages)
        cls = "menu-category" + (" active" if active_group else "")
        out.append(f'        <li class="{cls}">')
        out.append(f'          <button class="menu-caret" type="button" aria-expanded="true">{html.escape(group)}</button>')
        out.append('          <ul class="menu-list">')
        for page, label in pages:
            attr = ' class="menu-link active" aria-current="page"' if page == current else ' class="menu-link"'
            out.append(f'            <li><a href="{page}.html"{attr}>{html.escape(label)}</a></li>')
        out.append("          </ul>")
        out.append("        </li>")
    out.append("      </ul>")
    return "\n".join(out)


def toc_items(body):
    items = []
    for level, anchor, label in HEADING.findall(body):
        cls = ' class="toc-sub"' if level == "3" else ""
        items.append(f'<li{cls}><a href="#{anchor}">{html.escape(text(label))}</a></li>')
    return items


def on_this_page(body):
    items = toc_items(body)
    if not items:
        return ""
    return ('    <nav class="toc" aria-label="On this page">\n      <ul>\n        '
            + "\n        ".join(items) + "\n      </ul>\n    </nav>")


def toc_mobile(body):
    items = toc_items(body)
    if not items:
        return ""
    return ('<details class="toc-mobile"><summary>On this page</summary><ul>'
            + "".join(items) + "</ul></details>")


def breadcrumbs(page, title):
    home = ('<a class="crumb-home" href="index.html" aria-label="Documentation home">'
            '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path d="M10 19v-5h4v5c0 .55.45 1 1 1h3c.55 0 1-.45 1-1v-7h1.7c.46 0 .68-.57.33-.87L12.67 3.6c-.38-.34-.96-.34-1.34 0l-8.36 7.53c-.34.3-.13.87.33.87H5v7c0 .55.45 1 1 1h3c.55 0 1-.45 1-1z" fill="currentColor"/></svg></a>')
    if page == "index":
        return f'<nav class="breadcrumbs" aria-label="Breadcrumbs">{home}</nav>'
    return (f'<nav class="breadcrumbs" aria-label="Breadcrumbs">{home}<span class="crumb">{html.escape(group_of(page))}</span>'
            f'<span class="crumb current" aria-current="page">{html.escape(title)}</span></nav>')


def pager(order, i):
    links = []
    if i > 0:
        page, label = order[i - 1]
        links.append(f'<a class="pagination-link prev" href="{page}.html"><span class="sublabel">Previous</span><span class="label">« {html.escape(label)}</span></a>')
    else:
        links.append("<span></span>")
    if i + 1 < len(order):
        page, label = order[i + 1]
        links.append(f'<a class="pagination-link next" href="{page}.html"><span class="sublabel">Next</span><span class="label">{html.escape(label)} »</span></a>')
    return '<nav class="pagination-nav" aria-label="Docs pages">' + "".join(links) + "</nav>"


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
<title>{title} | portop docs</title>
<meta name="description" content="{description}">
<link rel="icon" href="../assets/logo.png">
<link rel="stylesheet" href="docs.css">
<script>try {{ var t = localStorage.getItem("portop-docs-theme"); if (t) document.documentElement.dataset.theme = t; }} catch (e) {{}}</script>
<script defer src="docs.js"></script>
</head>
<body>
<a class="skip-link" href="#content">Skip to main content</a>
<header class="navbar">
  <button class="navbar-toggle" type="button" aria-controls="sidebar" aria-expanded="false" aria-label="Open the menu">
    <svg viewBox="0 0 30 30" width="26" height="26" aria-hidden="true"><path stroke="currentColor" stroke-linecap="round" stroke-width="2" d="M4 7h22M4 15h22M4 23h22"/></svg>
  </button>
  <a class="navbar-brand" href="index.html"><img src="../assets/logo.png" alt="" width="32" height="32"><b>portop</b></a>
  <nav class="navbar-items" aria-label="Site">
    <a class="navbar-link active" href="index.html">Docs</a>
    <a class="navbar-link" href="../">Home</a>
  </nav>
  <div class="navbar-right">
    <a class="navbar-link github" href="https://github.com/padovanl/portop">GitHub<svg viewBox="0 0 24 24" width="13" height="13" aria-hidden="true"><path fill="currentColor" d="M21 13v10h-21v-19h12v2h-10v15h17v-8h2zm3-12h-10.988l4.035 4-6.977 7.07 2.828 2.828 6.977-7.07 4.125 4.172v-11z"/></svg></a>
    <button class="theme-toggle" type="button" aria-label="Switch between dark and light mode">
      <svg class="sun" viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"><path fill="currentColor" d="M12 9c1.65 0 3 1.35 3 3s-1.35 3-3 3-3-1.35-3-3 1.35-3 3-3m0-2c-2.76 0-5 2.24-5 5s2.24 5 5 5 5-2.24 5-5-2.24-5-5-5zM2 13h2c.55 0 1-.45 1-1s-.45-1-1-1H2c-.55 0-1 .45-1 1s.45 1 1 1zm18 0h2c.55 0 1-.45 1-1s-.45-1-1-1h-2c-.55 0-1 .45-1 1s.45 1 1 1zM11 2v2c0 .55.45 1 1 1s1-.45 1-1V2c0-.55-.45-1-1-1s-1 .45-1 1zm0 18v2c0 .55.45 1 1 1s1-.45 1-1v-2c0-.55-.45-1-1-1s-1 .45-1 1zM5.99 4.58a.996.996 0 0 0-1.41 0 .996.996 0 0 0 0 1.41l1.06 1.06c.39.39 1.03.39 1.41 0s.39-1.03 0-1.41L5.99 4.58zm12.37 12.37a.996.996 0 0 0-1.41 0 .996.996 0 0 0 0 1.41l1.06 1.06c.39.39 1.03.39 1.41 0a.996.996 0 0 0 0-1.41l-1.06-1.06zm1.06-10.96a.996.996 0 0 0 0-1.41.996.996 0 0 0-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06zM7.05 18.36a.996.996 0 0 0 0-1.41.996.996 0 0 0-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06z"/></svg>
      <svg class="moon" viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"><path fill="currentColor" d="M9.37 5.51A7.35 7.35 0 0 0 9.1 7.5c0 4.08 3.32 7.4 7.4 7.4.68 0 1.35-.09 1.99-.27A7.014 7.014 0 0 1 12 19c-3.86 0-7-3.14-7-7 0-2.93 1.81-5.45 4.37-6.49zM12 3a9 9 0 1 0 9 9c0-.46-.04-.92-.1-1.36a5.389 5.389 0 0 1-4.4 2.26 5.403 5.403 0 0 1-3.14-9.8c-.44-.06-.9-.1-1.36-.1z"/></svg>
    </button>
    <button class="search-button" type="button" aria-label="Search the docs">
      <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true"><path d="M14.386 14.386l4.088 4.088-4.088-4.088A7.533 7.533 0 1 1 3.733 3.733a7.533 7.533 0 0 1 10.653 10.653z" stroke="currentColor" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
      <span>Search</span><kbd class="search-key">Ctrl K</kbd>
    </button>
  </div>
</header>
<div class="main-wrapper">
  <aside class="sidebar" id="sidebar" aria-label="Documentation">
    <nav class="menu">
{sidebar}
    </nav>
  </aside>
  <main class="doc" id="content">
    <div class="doc-row">
      <div class="doc-col">
        {breadcrumbs}
        {toc_mobile}
        <article class="markdown">
{body}
        </article>
        <div class="doc-footer"><a class="edit-link" href="https://github.com/padovanl/portop/edit/develop/docs-src/{page}.html"><svg viewBox="0 0 40 40" width="18" height="18" aria-hidden="true"><path fill="currentColor" d="m34.5 11.7l-3 3.1-6.3-6.3 3.1-3q0.5-0.5 1.2-0.5t1.1 0.5l3.9 3.9q0.5 0.4 0.5 1.1t-0.5 1.2z m-29.5 17.1l18.4-18.5 6.3 6.3-18.4 18.4h-6.3v-6.2z"/></svg>Edit this page</a></div>
        {pager}
      </div>
      <div class="toc-col">
{toc}
      </div>
    </div>
  </main>
</div>
<footer class="footer">
  <div class="footer-links">
    <div><p class="footer-title">Docs</p><ul><li><a href="installation.html">Installation</a></li><li><a href="the-table.html">The table</a></li><li><a href="command-line.html">Command line</a></li><li><a href="desktop.html">Desktop app</a></li></ul></div>
    <div><p class="footer-title">Community</p><ul><li><a href="https://github.com/padovanl/portop/issues">Issues</a></li><li><a href="https://github.com/padovanl/portop/blob/main/PRIVACY.md">Privacy</a></li><li><a href="https://github.com/padovanl/portop/blob/main/CONTRIBUTING.md">Contributing</a></li></ul></div>
    <div><p class="footer-title">More</p><ul><li><a href="../">Project page</a></li><li><a href="https://github.com/padovanl/portop/releases">Releases</a></li><li><a href="https://github.com/padovanl/portop">GitHub</a></li></ul></div>
  </div>
  <p class="footer-copyright">portop is released under the MIT License.</p>
</footer>
<div class="search-modal" id="search-modal" hidden>
  <div class="search-box" role="dialog" aria-modal="true" aria-label="Search the docs">
    <form class="search-form" role="search" onsubmit="return false">
      <svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true"><path d="M14.386 14.386l4.088 4.088-4.088-4.088A7.533 7.533 0 1 1 3.733 3.733a7.533 7.533 0 0 1 10.653 10.653z" stroke="currentColor" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
      <input id="search" type="search" placeholder="Search docs" autocomplete="off" spellcheck="false" aria-label="Search docs">
      <button class="search-cancel" type="button">Cancel</button>
    </form>
    <div class="search-results" id="search-results"></div>
    <div class="search-footer"><span><kbd>↵</kbd> to select</span><span><kbd>↓</kbd><kbd>↑</kbd> to navigate</span><span><kbd>esc</kbd> to close</span></div>
  </div>
</div>
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
            toc=on_this_page(body), toc_mobile=toc_mobile(body),
            breadcrumbs=breadcrumbs(page, title), page=page)
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
