/* The interface for the Summary List of Sales and Purchases generator.
   Everything it talks to is this binary, on the loopback address. */
(function () {
  "use strict";

  var $ = function (id) { return document.getElementById(id); };
  var state = { list: "sales", layout: null, defaults: null, run: null };

  /* ---------------------------------------------------------------- theme */

  (function theme() {
    var root = document.documentElement;
    var btn = document.querySelector("[data-themer]");
    if (!btn) return;
    function current() {
      var t = root.getAttribute("data-theme");
      if (t) return t;
      return (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) ? "dark" : "light";
    }
    function paint() {
      var dark = current() === "dark";
      btn.textContent = dark ? "☀ Light" : "☾ Dark";
      btn.setAttribute("aria-label", dark ? "Switch to light theme" : "Switch to dark theme");
    }
    paint();
    btn.addEventListener("click", function () {
      var next = current() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      try { localStorage.setItem("omsme-theme", next); } catch (e) { /* private window */ }
      paint();
    });
  })();

  /* ------------------------------------------------------------- plumbing */

  function text(el, s) { el.textContent = s == null ? "" : String(s); }

  function show(el, on) {
    if (on) { el.removeAttribute("hidden"); } else { el.setAttribute("hidden", ""); }
  }

  function peso(centavos) {
    var neg = centavos < 0;
    var v = Math.abs(centavos);
    var whole = String(Math.floor(v / 100));
    var out = "";
    for (var i = 0; i < whole.length; i++) {
      if (i > 0 && (whole.length - i) % 3 === 0) out += ",";
      out += whole.charAt(i);
    }
    var cents = String(v % 100);
    if (cents.length < 2) cents = "0" + cents;
    return (neg ? "-" : "") + "₱" + out + "." + cents;
  }

  function fail(msg) {
    var box = $("error");
    text(box, msg);
    show(box, !!msg);
  }

  function busy(on) {
    $("run").disabled = on;
    document.body.classList.toggle("busy", on);
    text($("status"), on ? "Building the file…" : "Nothing is uploaded. Your customers' and suppliers' details are read on this computer only.");
  }

  function request(url, options) {
    return fetch(url, options || {}).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (body) {
        if (!r.ok) throw new Error(body.error || ("the tool replied " + r.status));
        return body;
      });
    });
  }

  /* -------------------------------------------------------------- settings */

  var fields = {
    tin: "i-tin", rdoCode: "i-rdo", registeredName: "i-regname", personName: "i-person",
    tradeName: "i-trade", period: "i-period", nonCreditable: "i-noncreditable"
  };

  function readSettings() {
    var s = {
      list: state.list,
      address: [$("i-addr1").value, $("i-addr2").value],
      yearEndMonth: parseInt($("i-yearend").value, 10) || 0,
      dateOrder: $("i-dateorder").value,
      allowNegative: $("i-allowneg").checked,
      noConsolidate: $("i-noconsolidate").checked,
      vatRate: (state.defaults && state.defaults.vatRate) || 12
    };
    Object.keys(fields).forEach(function (k) { s[k] = $(fields[k]).value; });
    return s;
  }

  function writeSettings(s) {
    if (!s) return;
    Object.keys(fields).forEach(function (k) {
      if (s[k] != null) $(fields[k]).value = s[k];
    });
    if (s.address) {
      $("i-addr1").value = s.address[0] || "";
      $("i-addr2").value = s.address[1] || "";
    }
    if (s.yearEndMonth != null) $("i-yearend").value = String(s.yearEndMonth);
    if (s.dateOrder) $("i-dateorder").value = s.dateOrder;
    $("i-allowneg").checked = !!s.allowNegative;
    $("i-noconsolidate").checked = !!s.noConsolidate;
    if (s.list) setList(s.list);
  }

  function setList(which) {
    state.list = which === "purchases" ? "purchases" : "sales";
    Array.prototype.forEach.call($("list-seg").querySelectorAll("button"), function (b) {
      b.setAttribute("aria-pressed", b.getAttribute("data-list") === state.list ? "true" : "false");
    });
    show($("noncreditable-field"), state.list === "purchases");
    $("dl-template").setAttribute("href", "/api/sample?list=" + state.list);
    renderLayout();
  }

  /* ---------------------------------------------------------------- layout */

  function renderLayout() {
    if (!state.layout) return;
    var d = state.layout[state.list];
    if (!d) return;

    text($("layout-note"),
      "Fields are separated by " + d.fieldSeparator + " and text fields are enclosed in " + d.textDelimiter +
      ". Each record ends with " + d.recordSeparator + ". The file is " + d.encoding +
      ". The name must be " + d.fileNamePattern + ".");

    var seg = $("layout-seg");
    seg.innerHTML = "";
    var box = $("layout");
    box.innerHTML = "";

    d.records.forEach(function (rec, idx) {
      var b = document.createElement("button");
      b.type = "button";
      b.textContent = rec.what;
      var n = document.createElement("span");
      n.className = "n";
      n.textContent = rec.fields.length;
      b.appendChild(n);
      b.setAttribute("aria-pressed", idx === 0 ? "true" : "false");
      b.addEventListener("click", function () {
        Array.prototype.forEach.call(seg.querySelectorAll("button"), function (x) {
          x.setAttribute("aria-pressed", "false");
        });
        b.setAttribute("aria-pressed", "true");
        drawRecord(box, rec);
      });
      seg.appendChild(b);
    });
    drawRecord(box, d.records[0]);

    if (d.uncertain && d.uncertain.length) {
      var warn = document.createElement("div");
      warn.className = "note";
      warn.textContent = "These parts of the layout could not be sourced from the Bureau's own material and should be checked against a file produced by its data entry module: " + d.uncertain.join("; ") + ".";
      box.appendChild(warn);
    }
  }

  function drawRecord(box, rec) {
    box.innerHTML = "";
    var wrap = document.createElement("div");
    wrap.className = "tablewrap";
    var table = document.createElement("table");
    table.className = "breakdown";
    var head = document.createElement("thead");
    var hr = document.createElement("tr");
    ["#", "Field", "Written as", "Quoted", "Max", "Required", "What it holds"].forEach(function (h) {
      var th = document.createElement("th");
      th.textContent = h;
      hr.appendChild(th);
    });
    head.appendChild(hr);
    table.appendChild(head);

    var body = document.createElement("tbody");
    rec.fields.forEach(function (f) {
      var tr = document.createElement("tr");
      [String(f.position), f.name, f.kind, f.quoted, f.max > 0 ? String(f.max) : "—",
        f.required ? "yes" : "", f.note || ""].forEach(function (v, i) {
        var td = document.createElement("td");
        td.textContent = v;
        if (i === 0 || i === 4) td.className = "num";
        if (i === 1) td.className = "mono";
        tr.appendChild(td);
      });
      body.appendChild(tr);
    });
    table.appendChild(body);
    wrap.appendChild(table);
    box.appendChild(wrap);
  }

  /* --------------------------------------------------------------- results */

  function renderRun(run) {
    state.run = run;
    var res = run && run.result;
    if (!res) return;

    text($("r-name"), res.fileName || "—");
    text($("r-label"), res.list + " · " + res.period);

    var errs = (res.findings || []).filter(function (f) { return f.severity === 0; });
    var warns = (res.findings || []).filter(function (f) { return f.severity === 1; });

    if (res.ok) {
      text($("r-sub"), res.totals.records + " record(s) written" + (warns.length ? ", " + warns.length + " thing(s) to look at" : ""));
    } else {
      text($("r-name"), "not written");
      text($("r-sub"), errs.length + " error(s) must be fixed first");
    }

    renderTotals(res);

    show($("dl-dat"), !!res.ok);
    show($("dl-sub"), !!res.ok);
    if (res.ok) {
      $("dl-dat").setAttribute("href", "/api/dat?id=" + encodeURIComponent(run.id));
      $("dl-dat").setAttribute("download", res.fileName);
      $("dl-sub").setAttribute("href", "/api/dat?id=" + encodeURIComponent(run.id) + "&copy=submission");
      $("dl-sub").setAttribute("download", res.fileName);
      text($("r-key"), "The submission copy carries the validation key " + res.validationKey +
        " on a final line. Attach that one for electronic submission; give the plain file to the validation module.");
    } else {
      text($("r-key"), "");
    }

    renderProblems(errs, warns, res);
    renderNotes(res);
    renderPreview(res);
  }

  function renderTotals(res) {
    var body = $("r-totals");
    body.innerHTML = "";
    var rows = [];
    var t = res.totals || {};
    if (res.list && res.list.indexOf("Purchases") >= 0) {
      var p = t.purchases || {};
      rows = [
        ["Exempt purchases", p.exempt], ["Zero-rated purchases", p.zeroRated],
        ["Taxable services", p.services], ["Taxable capital goods", p.capitalGoods],
        ["Taxable other goods", p.otherGoods], ["Input tax", p.inputVat],
        ["— creditable", t.creditableInputVat], ["— non-creditable", t.nonCreditableInputVat]
      ];
    } else {
      var s = t.sales || {};
      rows = [
        ["Exempt sales", s.exempt], ["Zero-rated sales", s.zeroRated],
        ["Taxable sales", s.taxable], ["Output tax", s.outputVat]
      ];
    }
    rows.forEach(function (r) {
      var tr = document.createElement("tr");
      var td = document.createElement("td");
      td.textContent = r[0];
      var num = document.createElement("td");
      num.className = "num";
      num.textContent = peso(r[1] || 0);
      tr.appendChild(td);
      tr.appendChild(num);
      body.appendChild(tr);
    });
    var tr = document.createElement("tr");
    tr.className = "total";
    var td = document.createElement("td");
    td.textContent = "Records in the file";
    var num = document.createElement("td");
    num.className = "num";
    num.textContent = String((res.totals && res.totals.records) || 0) + " of " + (res.rowsRead || 0) + " row(s)";
    tr.appendChild(td);
    tr.appendChild(num);
    body.appendChild(tr);
  }

  function renderProblems(errs, warns, res) {
    var list = $("problems");
    list.innerHTML = "";
    var all = errs.concat(warns);
    show($("problems-card"), all.length > 0);
    if (!all.length) return;

    text($("problems-head"), errs.length ? "What has to be fixed" : "Worth checking");
    text($("problems-note"), errs.length
      ? "The file was not written. Fix these rows in your CSV and build it again."
      : "The file was written. None of these stops it, but each is worth a look.");

    all.forEach(function (f) {
      var li = document.createElement("li");
      if (f.severity === 1) li.className = "warn";
      var where = document.createElement("span");
      where.className = "where";
      var bits = [];
      if (f.line > 0) bits.push("CSV line " + f.line);
      if (f.record) bits.push(f.record + " record");
      if (f.field) bits.push("field “" + f.field + "”");
      if (f.value) bits.push("value “" + f.value + "”");
      where.textContent = bits.join(" · ") || (f.severity === 1 ? "worth checking" : "the file as a whole");
      li.appendChild(where);
      li.appendChild(document.createTextNode(f.detail));
      list.appendChild(li);
    });
  }

  function renderNotes(res) {
    var list = $("notes");
    list.innerHTML = "";
    var items = [];
    (res.notes || []).forEach(function (n) { items.push(n); });
    (res.merges || []).forEach(function (m) {
      items.push("Added together into one record: " + m.name + " (" + m.tin + "), from lines " + m.lines.join(", ") + ".");
    });
    if (res.columns) {
      var pairs = Object.keys(res.columns).sort().map(function (k) {
        return k + " ← “" + res.columns[k] + "”";
      });
      if (pairs.length) items.push("Columns read: " + pairs.join("; ") + ".");
    }
    show($("notes-card"), items.length > 0);
    items.forEach(function (n) {
      var li = document.createElement("li");
      li.className = "warn";
      li.textContent = n;
      list.appendChild(li);
    });
  }

  function renderPreview(res) {
    var pre = $("preview");
    show($("preview-card"), !!(res.ok && res.dat));
    if (!res.ok || !res.dat) { pre.textContent = ""; return; }
    var lines = res.dat.split("\r\n").filter(function (l) { return l !== ""; });
    var shown = lines.slice(0, 40);
    pre.textContent = shown.join("\n") + (lines.length > shown.length
      ? "\n… and " + (lines.length - shown.length) + " more record(s)" : "");
  }

  /* ----------------------------------------------------------- saved runs */

  function renderSaved(runs) {
    var body = $("saved-body");
    body.innerHTML = "";
    show($("saved"), runs && runs.length > 0);
    (runs || []).forEach(function (r) {
      var tr = document.createElement("tr");
      [new Date(r.createdAt).toLocaleString(), r.list || "", r.period || "", r.fileName || "",
        String(r.records || 0)].forEach(function (v, i) {
        var td = document.createElement("td");
        td.textContent = v;
        if (i === 4) td.className = "num";
        if (i === 3) td.className = "mono";
        tr.appendChild(td);
      });
      var last = document.createElement("td");
      if (r.ok) {
        var a = document.createElement("a");
        a.href = "/api/dat?id=" + encodeURIComponent(r.id);
        a.setAttribute("download", r.fileName);
        a.textContent = "download";
        last.appendChild(a);
      } else {
        last.textContent = r.errors + " error(s)";
        last.className = "bad";
      }
      tr.appendChild(last);
      body.appendChild(tr);
    });
  }

  /* ------------------------------------------------------------- actions */

  function load() {
    return request("/api/state").then(function (body) {
      state.layout = body.layout;
      state.defaults = body.defaults;
      writeSettings(body.settings);
      renderSaved(body.runs);
      renderLayout();
    }).catch(function (e) { fail(e.message); });
  }

  function build() {
    fail("");
    var set = readSettings();
    var file = $("f-csv").files && $("f-csv").files[0];
    var pasted = $("i-paste").value.trim();
    var path = $("p-csv").value.trim();

    var options;
    if (file) {
      var form = new FormData();
      form.append("csv", file);
      Object.keys(set).forEach(function (k) {
        if (k === "address") {
          form.append("address1", set.address[0] || "");
          form.append("address2", set.address[1] || "");
          return;
        }
        form.append(k, typeof set[k] === "boolean" ? (set[k] ? "1" : "0") : String(set[k]));
      });
      options = { method: "POST", body: form };
    } else if (pasted || path) {
      var payload = JSON.parse(JSON.stringify(set));
      if (pasted) { payload.csvText = pasted; } else { payload.csvPath = path; }
      options = {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      };
    } else {
      fail("Choose your CSV, give its path on this computer, or paste the rows.");
      return;
    }

    busy(true);
    request("/api/generate", options).then(function (body) {
      renderRun(body.run);
      return request("/api/state");
    }).then(function (body) {
      renderSaved(body.runs);
    }).catch(function (e) {
      fail(e.message);
    }).then(function () {
      busy(false);
    });
  }

  function clearAll() {
    fail("");
    $("f-csv").value = "";
    $("i-paste").value = "";
    $("p-csv").value = "";
    state.run = null;
    text($("r-name"), "—");
    text($("r-label"), "File name");
    text($("r-sub"), "Choose a list, a month and a CSV");
    text($("r-key"), "");
    $("r-totals").innerHTML = "<tr><td colspan=\"2\" class=\"empty\">Nothing built yet.</td></tr>";
    show($("problems-card"), false);
    show($("notes-card"), false);
    show($("preview-card"), false);
    show($("dl-dat"), false);
    show($("dl-sub"), false);
  }

  /* --------------------------------------------------------------- wiring */

  Array.prototype.forEach.call($("list-seg").querySelectorAll("button"), function (b) {
    b.addEventListener("click", function () { setList(b.getAttribute("data-list")); });
  });
  $("run").addEventListener("click", build);
  $("reset").addEventListener("click", clearAll);
  load();
})();
