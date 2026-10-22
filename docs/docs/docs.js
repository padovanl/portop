(function () {
  "use strict";

  var root = document.documentElement;
  var store = {
    get: function (k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
    set: function (k, v) { try { localStorage.setItem(k, v); } catch (e) { /* optional */ } },
  };

  // Theme: the system's unless the toggle chose one (stored per browser).
  document.querySelector(".theme-toggle").addEventListener("click", function () {
    var dark = root.dataset.theme ? root.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    root.dataset.theme = dark ? "light" : "dark";
    store.set("portop-docs-theme", root.dataset.theme);
  });

  // Sidebar drawer on narrow screens.
  var toggle = document.querySelector(".navbar-toggle");
  function setNav(open) {
    document.body.classList.toggle("nav-open", open);
    toggle.setAttribute("aria-expanded", open ? "true" : "false");
  }
  toggle.addEventListener("click", function (e) { e.stopPropagation(); setNav(!document.body.classList.contains("nav-open")); });
  document.addEventListener("click", function (e) {
    if (document.body.classList.contains("nav-open") && !e.target.closest(".sidebar")) setNav(false);
  });

  // Collapsible sidebar categories; the collapsed ones are remembered.
  var collapsed = (store.get("portop-docs-collapsed") || "").split("|").filter(Boolean);
  document.querySelectorAll(".menu-category").forEach(function (cat) {
    var button = cat.querySelector(".menu-caret");
    var name = button.textContent;
    function apply(isCollapsed) {
      cat.classList.toggle("collapsed", isCollapsed);
      button.setAttribute("aria-expanded", isCollapsed ? "false" : "true");
    }
    apply(collapsed.indexOf(name) >= 0 && !cat.classList.contains("active"));
    button.addEventListener("click", function () {
      var now = !cat.classList.contains("collapsed");
      apply(now);
      collapsed = collapsed.filter(function (n) { return n !== name; });
      if (now) collapsed.push(name);
      store.set("portop-docs-collapsed", collapsed.join("|"));
    });
  });
  var active = document.querySelector(".menu-link.active");
  if (active && active.scrollIntoView) active.scrollIntoView({ block: "nearest" });

  // Heading anchors.
  document.querySelectorAll(".markdown h2[id], .markdown h3[id]").forEach(function (h) {
    var a = document.createElement("a");
    a.className = "anchor";
    a.href = "#" + h.id;
    a.setAttribute("aria-label", "Direct link to " + h.textContent);
    a.textContent = "#";
    h.appendChild(a);
  });

  // Copy buttons on code blocks.
  var copyIcon = '<svg class="copy" viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M19,21H8V7H19M19,5H8A2,2 0 0,0 6,7V21A2,2 0 0,0 8,23H19A2,2 0 0,0 21,21V7A2,2 0 0,0 19,5M16,1H4A2,2 0 0,0 2,3V17H4V3H16V1Z"/></svg>' +
    '<svg class="check" viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M21,7L9,19L3.5,13.5L4.91,12.09L9,16.17L19.59,5.59L21,7Z"/></svg>';
  document.querySelectorAll(".markdown pre").forEach(function (pre) {
    var b = document.createElement("button");
    b.type = "button";
    b.className = "copy-button";
    b.setAttribute("aria-label", "Copy code to clipboard");
    b.innerHTML = copyIcon;
    b.addEventListener("click", function () {
      var code = pre.cloneNode(true);
      code.querySelectorAll(".copy-button").forEach(function (x) { x.remove(); });
      navigator.clipboard.writeText(code.textContent.replace(/\n$/, "")).then(function () {
        b.classList.add("copied");
        setTimeout(function () { b.classList.remove("copied"); }, 1500);
      });
    });
    pre.appendChild(b);
  });

  // Tables scroll sideways when wide; on phones, tables of three or more
  // columns stack, each cell labelled with its heading.
  document.querySelectorAll(".markdown table").forEach(function (table) {
    var heads = Array.prototype.map.call(table.querySelectorAll("thead th"), function (th) { return th.textContent; });
    if (heads.length > 2) {
      table.setAttribute("data-stack", "");
      table.querySelectorAll("tbody tr").forEach(function (tr) {
        Array.prototype.forEach.call(tr.children, function (td, i) {
          if (i > 0 && heads[i]) td.setAttribute("data-label", heads[i]);
        });
      });
    }
    var wrap = document.createElement("div");
    wrap.className = "table-wrap";
    table.parentNode.insertBefore(wrap, table);
    wrap.appendChild(table);
  });

  // Table of contents: highlight the section being read.
  var tocLinks = Array.prototype.slice.call(document.querySelectorAll(".toc a"));
  if (tocLinks.length && "IntersectionObserver" in window) {
    var byId = {};
    tocLinks.forEach(function (a) { byId[a.getAttribute("href").slice(1)] = a; });
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        tocLinks.forEach(function (a) { a.classList.remove("active"); });
        byId[entry.target.id].classList.add("active");
      });
    }, { rootMargin: "-70px 0px -70% 0px" });
    Object.keys(byId).forEach(function (id) {
      var el = document.getElementById(id);
      if (el) observer.observe(el);
    });
  }
  document.querySelectorAll(".toc-mobile a").forEach(function (a) {
    a.addEventListener("click", function () { a.closest("details").open = false; });
  });

  // Search: a modal over every page's sections, from search.json.
  var isMac = /Mac|iPhone|iPad/.test(navigator.platform);
  var key = document.querySelector(".search-key");
  if (key) key.textContent = isMac ? "⌘ K" : "Ctrl K";
  var modal = document.getElementById("search-modal");
  var input = document.getElementById("search");
  var results = document.getElementById("search-results");
  var index = null, selected = -1, lastFocus = null;
  function load() {
    if (index) return Promise.resolve(index);
    return fetch("search.json").then(function (r) { return r.json(); }).then(function (data) { index = data; return data; });
  }
  function open() {
    lastFocus = document.activeElement;
    modal.hidden = false;
    document.body.style.overflow = "hidden";
    input.select();
    input.focus();
    load().then(function () { render(input.value); });
  }
  function close() {
    modal.hidden = true;
    document.body.style.overflow = "";
    if (lastFocus && lastFocus.focus) lastFocus.focus();
  }
  function render(query) {
    var terms = query.toLowerCase().split(/\s+/).filter(Boolean);
    results.replaceChildren();
    selected = -1;
    if (!terms.length) return;
    var scored = index.map(function (e) {
      var title = (e.title + " " + e.section).toLowerCase(), body = e.text.toLowerCase(), score = 0;
      for (var i = 0; i < terms.length; i++) {
        if (title.indexOf(terms[i]) >= 0) score += 3;
        else if (body.indexOf(terms[i]) >= 0) score += 1;
        else return null;
      }
      return { e: e, score: score };
    }).filter(Boolean).sort(function (a, b) { return b.score - a.score; }).slice(0, 12);
    if (!scored.length) {
      var empty = document.createElement("p");
      empty.className = "empty";
      empty.textContent = "No results for “" + query + "”";
      results.appendChild(empty);
      return;
    }
    // Group by page, in the order of each page's best hit.
    var groups = [], byPage = {};
    scored.forEach(function (s) {
      if (!byPage[s.e.page]) { byPage[s.e.page] = []; groups.push({ title: s.e.title, hits: byPage[s.e.page] }); }
      byPage[s.e.page].push(s.e);
    });
    groups.forEach(function (g) {
      var head = document.createElement("div");
      head.className = "hit-group";
      head.textContent = g.title;
      results.appendChild(head);
      g.hits.forEach(function (e) {
        var a = document.createElement("a");
        a.href = e.page + ".html" + (e.anchor ? "#" + e.anchor : "");
        var strong = document.createElement("strong");
        strong.textContent = e.section || e.title;
        var span = document.createElement("span");
        span.textContent = e.text.slice(0, 120);
        a.append(strong, span);
        a.addEventListener("click", close);
        results.appendChild(a);
      });
    });
  }
  document.querySelector(".search-button").addEventListener("click", open);
  document.querySelector(".search-cancel").addEventListener("click", close);
  modal.addEventListener("mousedown", function (e) { if (e.target === modal) close(); });
  input.addEventListener("input", function () { load().then(function () { render(input.value); }); });
  input.addEventListener("keydown", function (e) {
    var links = results.querySelectorAll("a");
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (!links.length) return;
      selected = (selected + (e.key === "ArrowDown" ? 1 : -1) + links.length) % links.length;
      links.forEach(function (a, i) { a.classList.toggle("selected", i === selected); });
      links[selected].scrollIntoView({ block: "nearest" });
    } else if (e.key === "Enter" && links.length) {
      location.href = links[Math.max(selected, 0)].href;
      close();
    }
  });
  document.addEventListener("keydown", function (e) {
    var typing = /input|textarea|select/i.test(document.activeElement.tagName);
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      if (modal.hidden) open(); else close();
    } else if (e.key === "/" && !typing && modal.hidden) {
      e.preventDefault();
      open();
    } else if (e.key === "Escape") {
      if (!modal.hidden) close();
      else setNav(false);
    }
  });
})();
