(function () {
  "use strict";

  // Theme: the system's unless the toggle chose one (stored per browser).
  var root = document.documentElement;
  document.querySelector(".theme-toggle").addEventListener("click", function () {
    var dark = root.dataset.theme ? root.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    root.dataset.theme = dark ? "light" : "dark";
    try { localStorage.setItem("portop-docs-theme", root.dataset.theme); } catch (e) { /* optional */ }
  });

  // Sidebar drawer on narrow screens.
  var toggle = document.querySelector(".nav-toggle");
  function setNav(open) {
    document.body.classList.toggle("nav-open", open);
    toggle.setAttribute("aria-expanded", open ? "true" : "false");
  }
  toggle.addEventListener("click", function () { setNav(!document.body.classList.contains("nav-open")); });
  document.addEventListener("click", function (e) {
    if (document.body.classList.contains("nav-open") && !e.target.closest(".sidebar, .nav-toggle")) setNav(false);
  });
  document.addEventListener("keydown", function (e) { if (e.key === "Escape") setNav(false); });

  // Wide tables scroll sideways instead of widening the page; on phones
  // they stack, and cells of tables with three or more columns carry their
  // heading as a label.
  document.querySelectorAll("main table").forEach(function (table) {
    var heads = Array.prototype.map.call(table.querySelectorAll("thead th"), function (th) { return th.textContent; });
    if (heads.length > 2) {
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

  // "On this page": highlight the section being read.
  var tocLinks = Array.prototype.slice.call(document.querySelectorAll(".page-toc a"));
  if (tocLinks.length && "IntersectionObserver" in window) {
    var byId = {};
    tocLinks.forEach(function (a) { byId[a.getAttribute("href").slice(1)] = a; });
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        tocLinks.forEach(function (a) { a.classList.remove("active"); });
        byId[entry.target.id].classList.add("active");
      });
    }, { rootMargin: "-60px 0px -70% 0px" });
    Object.keys(byId).forEach(function (id) {
      var el = document.getElementById(id);
      if (el) observer.observe(el);
    });
  }

  // Search over every page's sections, from search.json.
  var input = document.getElementById("search");
  var results = document.getElementById("search-results");
  var index = null, selected = -1;
  function load() {
    if (index) return Promise.resolve(index);
    return fetch("search.json").then(function (r) { return r.json(); }).then(function (data) { index = data; return data; });
  }
  function render(query) {
    var terms = query.toLowerCase().split(/\s+/).filter(Boolean);
    results.replaceChildren();
    selected = -1;
    if (!terms.length) { results.hidden = true; return; }
    var scored = index.map(function (e) {
      var title = (e.title + " " + e.section).toLowerCase(), body = e.text.toLowerCase(), score = 0;
      for (var i = 0; i < terms.length; i++) {
        if (title.indexOf(terms[i]) >= 0) score += 3;
        else if (body.indexOf(terms[i]) >= 0) score += 1;
        else return null;
      }
      return { e: e, score: score };
    }).filter(Boolean).sort(function (a, b) { return b.score - a.score; }).slice(0, 8);
    if (!scored.length) {
      var li = document.createElement("li");
      li.className = "empty";
      li.textContent = "No results";
      results.appendChild(li);
    }
    scored.forEach(function (s) {
      var li = document.createElement("li"), a = document.createElement("a");
      a.href = s.e.page + ".html" + (s.e.anchor ? "#" + s.e.anchor : "");
      var strong = document.createElement("strong");
      strong.textContent = s.e.section ? s.e.title + " › " + s.e.section : s.e.title;
      var span = document.createElement("span");
      span.textContent = s.e.text.slice(0, 110) + (s.e.text.length > 110 ? "…" : "");
      a.append(strong, span);
      li.appendChild(a);
      results.appendChild(li);
    });
    results.hidden = false;
  }
  input.addEventListener("input", function () { load().then(function () { render(input.value); }); });
  input.addEventListener("focus", function () { load(); });
  input.addEventListener("keydown", function (e) {
    var links = results.querySelectorAll("a");
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (!links.length) return;
      selected = (selected + (e.key === "ArrowDown" ? 1 : -1) + links.length) % links.length;
      links.forEach(function (a, i) { a.classList.toggle("selected", i === selected); });
    } else if (e.key === "Enter" && links.length) {
      location.href = links[Math.max(selected, 0)].href;
    } else if (e.key === "Escape") {
      results.hidden = true;
      input.blur();
    }
  });
  document.addEventListener("click", function (e) { if (!e.target.closest(".search")) results.hidden = true; });
  document.addEventListener("keydown", function (e) {
    if (e.key === "/" && document.activeElement !== input && !/input|textarea/i.test(document.activeElement.tagName)) {
      e.preventDefault();
      input.focus();
    }
  });
})();
