const refreshMs = 5000;
const COLUMN_STORAGE_KEY = "tlsbench-visible-columns";
const expandedGroups = new Set();

const METRIC_GUIDE = [
  {
    id: "tls_handshake_ms",
    title: "TLS hs cli",
    text: "Client-side TLS setup (curl). On edge/reencrypt this is the router cert; on passthrough it matches the pod cert.",
    good: "≤ 50 ms (ECDSA, same cluster)",
    warn: "50–200 ms",
    bad: "> 200 ms or rising p99 vs p50",
  },
  {
    id: "tls_handshake_server_ms",
    title: "TLS hs srv",
    text: "Pod-side handshake on a new connection. Empty on edge (pod sees HTTP). Critical on reencrypt: inner handshake only.",
    good: "≤ 50 ms (ECDSA, same cluster)",
    warn: "50–200 ms",
    bad: "> 200 ms; much higher than cli on reencrypt",
  },
  {
    id: "tcp_connect_ms",
    title: "TCP ms",
    text: "TCP connect after DNS. Mostly RTT + SYN; not TLS.",
    good: "≤ 5 ms (localhost / same AZ)",
    warn: "5–50 ms",
    bad: "> 50 ms unless cross-region",
  },
  {
    id: "dns_ms",
    title: "DNS ms",
    text: "Name lookup before connect. Often zero when using IP or cached.",
    good: "≤ 10 ms",
    warn: "10–100 ms",
    bad: "> 100 ms",
  },
  {
    id: "ttfb_ms",
    title: "TTFB ms",
    text: "Time to first response byte after connect. Handshake probes should stay low; uploads include server work.",
    good: "≤ 20 ms (handshake-only)",
    warn: "20–100 ms",
    bad: "Dominated by blob size / disk on uploads",
  },
  {
    id: "transfer_ms",
    title: "Xfer ms",
    text: "Download: curl body transfer. Upload: send + server wait (pretransfer → starttransfer), not the tiny JSON response.",
    good: "Stable at fixed size; high MiB/s on bulk",
    warn: "Compare only within same size/path",
    bad: "Using response-body ms on uploads (inflates MiB/s)",
  },
  {
    id: "throughput_mib_s",
    title: "MiB/s",
    text: "bytes ÷ bulk duration. Upload uses TTFB interval; download uses transfer interval.",
    good: "> 100 MiB/s local bulk",
    warn: "10–100 MiB/s",
    bad: "< 10 MiB/s for large local uploads",
  },
  {
    id: "route_mode",
    title: "Route mode",
    text: "Termination path label from vm-bench. Edge = TLS at router; passthrough = TLS at pod; reencrypt = both.",
    good: "Label matches the URL you tested",
    warn: "—",
    bad: "Missing label when comparing Routes",
  },
];

const COLUMNS = [
  { id: "expand", label: "", default: true, always: true, summaryOnly: false },
  { id: "timestamp", label: "Time (UTC)", default: true },
  { id: "operation", label: "Operation", default: true },
  { id: "run", label: "Run", default: true },
  { id: "bytes", label: "Size", default: true },
  { id: "route_mode", label: "Route", default: true, metric: "route_mode" },
  { id: "tls", label: "TLS", default: true },
  { id: "cipher", label: "Cipher", default: true },
  { id: "key", label: "Key", default: true },
  { id: "alpn", label: "ALPN", default: true },
  { id: "dns_ms", label: "DNS ms", default: false, metric: "dns_ms" },
  { id: "tcp_connect_ms", label: "TCP ms", default: true, metric: "tcp_connect_ms" },
  { id: "tls_handshake_ms", label: "TLS hs cli", default: true, metric: "tls_handshake_ms" },
  { id: "tls_handshake_server_ms", label: "TLS hs srv", default: true, metric: "tls_handshake_server_ms" },
  { id: "tls_reused", label: "Reused", default: true },
  { id: "ttfb_ms", label: "TTFB ms", default: false, metric: "ttfb_ms" },
  { id: "transfer_ms", label: "Xfer ms", default: true, metric: "transfer_ms" },
  { id: "write_ms", label: "Write ms", default: false },
  { id: "read_ms", label: "Read ms", default: false },
  { id: "total_ms", label: "Total ms", default: true },
  { id: "throughput_mib_s", label: "MiB/s", default: true, metric: "throughput_mib_s" },
];

function loadVisibleColumns() {
  try {
    const raw = localStorage.getItem(COLUMN_STORAGE_KEY);
    if (!raw) {
      return new Set(COLUMNS.filter((c) => c.default).map((c) => c.id));
    }
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return new Set(COLUMNS.filter((c) => c.default).map((c) => c.id));
    }
    const visible = new Set(parsed);
    for (const col of COLUMNS) {
      if (col.always) visible.add(col.id);
    }
    return visible;
  } catch {
    return new Set(COLUMNS.filter((c) => c.default).map((c) => c.id));
  }
}

let visibleColumns = loadVisibleColumns();

function saveVisibleColumns() {
  localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify([...visibleColumns]));
}

function activeColumns() {
  return COLUMNS.filter((col) => visibleColumns.has(col.id));
}

function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MiB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
}

function formatMs(value) {
  if (value === undefined || value === null || value === 0) return "—";
  return Number(value).toFixed(2);
}

function formatTime(iso) {
  if (!iso) return "—";
  return iso.replace("T", " ").replace("Z", "Z");
}

function badgeTLS(run) {
  if (!run.tls) return '<span class="badge badge-http">HTTP</span>';
  const label = run.tls_version || "TLS";
  return `<span class="badge badge-tls">${label}</span>`;
}

function formatKey(algorithm, size, curve) {
  if (!algorithm) return "—";
  if (curve) return `${algorithm} ${curve}`;
  if (size) return `${algorithm} ${size}`;
  return algorithm;
}

function formatClientHandshake(run) {
  if (run.tls_reused && !run.tls_handshake_ms) return "reused";
  return formatMs(run.tls_handshake_ms);
}

function formatThroughput(run) {
  if (!run.throughput_mib_s) return "—";
  return Number(run.throughput_mib_s).toFixed(3);
}

// curl transfer_ms is response-body only on PUT uploads; bulk send is in ttfb_ms.
function bulkMs(run) {
  if (run.operation === "upload" && run.ttfb_ms > 0) return run.ttfb_ms;
  if (run.transfer_ms > 0) return run.transfer_ms;
  return 0;
}

function formatBulkMs(run) {
  const ms = bulkMs(run);
  if (!ms) return formatMs(run.transfer_ms);
  if (run.operation === "upload") {
    return `<span title="Upload send + server wait (curl pretransfer → starttransfer)">${formatMs(ms)}</span>`;
  }
  return formatMs(ms);
}

function metricClass(metricId, value, run) {
  if (value === undefined || value === null || value === 0) return "";
  const v = Number(value);
  if (Number.isNaN(v)) return "";

  switch (metricId) {
    case "tls_handshake_ms":
    case "tls_handshake_server_ms":
      if (v <= 50) return "metric-good";
      if (v <= 200) return "metric-warn";
      return "metric-bad";
    case "tcp_connect_ms":
      if (v <= 5) return "metric-good";
      if (v <= 50) return "metric-warn";
      return "metric-bad";
    case "dns_ms":
      if (v <= 10) return "metric-good";
      if (v <= 100) return "metric-warn";
      return "metric-bad";
    case "ttfb_ms":
      if (run.operation === "handshake" && v <= 20) return "metric-good";
      if (run.operation === "handshake" && v <= 100) return "metric-warn";
      if (run.operation === "handshake") return "metric-bad";
      return "";
    case "throughput_mib_s":
      if (v >= 100) return "metric-good";
      if (v >= 10) return "metric-warn";
      if (run.bytes >= 1024 * 1024) return "metric-bad";
      return "";
    default:
      return "";
  }
}

function cellHTML(col, run, context = {}) {
  const { summary = false, samples = [], expanded = false, groupId = "" } = context;

  switch (col.id) {
    case "expand":
      if (!summary || !groupId) return "";
      return `<button type="button" class="expand-btn" data-group="${groupId}" aria-expanded="${expanded}" title="Show individual samples">${expanded ? "▼" : "▶"}</button>`;
    case "timestamp":
      return formatTime(run.timestamp);
    case "operation":
      return `<span class="badge badge-op">${run.operation}</span>`;
    case "run":
      if (summary && run.experiment_id) {
        return `<span class="run-label">${escapeHtml(run.experiment_id)}</span> <span class="muted">(${samples.length} samples)</span>`;
      }
      if (run.experiment_id) {
        return `${escapeHtml(run.experiment_id)}#${escapeHtml(run.sample_index || "?")}`;
      }
      return escapeHtml(run.name || "—");
    case "bytes":
      return formatBytes(run.bytes);
    case "route_mode":
      return run.route_mode || "—";
    case "tls":
      return badgeTLS(run);
    case "cipher":
      return `<span class="cipher">${run.cipher || "—"}</span>`;
    case "key":
      return formatKey(run.tls_key_algorithm, run.tls_key_size, run.tls_key_curve);
    case "alpn":
      return run.alpn || "—";
    case "dns_ms":
      return `<span class="${metricClass("dns_ms", run.dns_ms, run)}">${formatMs(run.dns_ms)}</span>`;
    case "tcp_connect_ms":
      return `<span class="${metricClass("tcp_connect_ms", run.tcp_connect_ms, run)}">${formatMs(run.tcp_connect_ms)}</span>`;
    case "tls_handshake_ms":
      if (summary) {
        const hs = summarize(samples.map((s) => s.tls_handshake_ms));
        const cls = metricClass("tls_handshake_ms", hs.p50, run);
        return `<span class="${cls}">p50 ${formatMs(hs.p50)} / p99 ${formatMs(hs.p99)}</span>`;
      }
      return `<span class="${metricClass("tls_handshake_ms", run.tls_handshake_ms, run)}">${formatClientHandshake(run)}</span>`;
    case "tls_handshake_server_ms":
      if (summary) {
        const hs = summarize(samples.map((s) => s.tls_handshake_server_ms));
        const cls = metricClass("tls_handshake_server_ms", hs.p50, run);
        return `<span class="${cls}">p50 ${formatMs(hs.p50)} / p99 ${formatMs(hs.p99)}</span>`;
      }
      return `<span class="${metricClass("tls_handshake_server_ms", run.tls_handshake_server_ms, run)}">${formatMs(run.tls_handshake_server_ms)}</span>`;
    case "tls_reused":
      return run.tls_reused ? "yes" : "—";
    case "ttfb_ms":
      return `<span class="${metricClass("ttfb_ms", run.ttfb_ms, run)}">${formatMs(run.ttfb_ms)}</span>`;
    case "transfer_ms":
      if (summary) {
        const xfer = summarize(samples.map((s) => bulkMs(s)));
        return `p50 ${formatMs(xfer.p50)} / p99 ${formatMs(xfer.p99)}`;
      }
      return formatBulkMs(run);
    case "write_ms":
      return formatMs(run.write_ms);
    case "read_ms":
      return formatMs(run.read_ms);
    case "total_ms":
      return formatMs(run.client_total_ms || run.total_ms);
    case "throughput_mib_s":
      return `<span class="${metricClass("throughput_mib_s", run.throughput_mib_s, run)}">${formatThroughput(run)}</span>`;
    default:
      return "—";
  }
}

function renderTableHead() {
  const headRow = document.querySelector("#results-head tr");
  headRow.innerHTML = activeColumns()
    .map((col) => {
      const guide = METRIC_GUIDE.find((g) => g.id === col.metric);
      const title = guide ? `${guide.text}\nGood: ${guide.good}\nCaution: ${guide.warn}\nConcern: ${guide.bad}` : col.label;
      return `<th data-col="${col.id}" title="${escapeAttr(title)}">${col.label}</th>`;
    })
    .join("");
}

function renderColumnPicker() {
  const container = document.getElementById("column-picker-options");
  container.innerHTML = COLUMNS.filter((col) => !col.always)
    .map(
      (col) => `
      <label class="column-option">
        <input type="checkbox" data-col="${col.id}" ${visibleColumns.has(col.id) ? "checked" : ""}>
        ${col.label || col.id}
      </label>`
    )
    .join("");
}

function renderMetricsGuide() {
  const container = document.getElementById("metrics-guide");
  container.innerHTML = METRIC_GUIDE.map(
    (item) => `
    <article class="guide-card">
      <h3>${item.title}</h3>
      <p>${item.text}</p>
      <dl>
        <dt class="metric-good">Good</dt><dd>${item.good}</dd>
        <dt class="metric-warn">Caution</dt><dd>${item.warn}</dd>
        <dt class="metric-bad">Concern</dt><dd>${item.bad}</dd>
      </dl>
    </article>`
  ).join("");
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function escapeAttr(value) {
  return escapeHtml(value);
}

function percentile(sorted, p) {
  if (sorted.length === 0) return 0;
  if (sorted.length === 1) return sorted[0];
  const rank = (p / 100) * (sorted.length - 1);
  const lower = Math.floor(rank);
  const upper = Math.min(lower + 1, sorted.length - 1);
  const weight = rank - lower;
  return sorted[lower] * (1 - weight) + sorted[upper] * weight;
}

function summarize(values) {
  const filtered = values.filter((v) => v > 0).sort((a, b) => a - b);
  if (filtered.length === 0) {
    return { count: 0, p50: 0, p99: 0 };
  }
  return {
    count: filtered.length,
    p50: percentile(filtered, 50),
    p99: percentile(filtered, 99),
  };
}

function experimentKey(run) {
  if (!run.experiment_id) return "";
  return `${run.experiment_id}|${run.operation}|${run.route_mode || ""}`;
}

function groupRuns(runs) {
  const groups = new Map();
  const singles = [];
  for (const run of runs) {
    const key = experimentKey(run);
    if (!key) {
      singles.push(run);
      continue;
    }
    if (!groups.has(key)) {
      groups.set(key, []);
    }
    groups.get(key).push(run);
  }
  for (const samples of groups.values()) {
    samples.sort((a, b) => (b.sample_index || 0) - (a.sample_index || 0));
  }
  return { groups, singles };
}

async function fetchJSON(path) {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(`${path} ${response.status}`);
  }
  return response.json();
}

function renderConnection(info) {
  const summary = document.getElementById("conn-summary");
  const details = document.getElementById("conn-details");
  const path = info.tls
    ? "Pod saw TLS (passthrough, reencrypt, or Service HTTPS)."
    : "Pod saw HTTP (edge Route or Service HTTP).";
  summary.textContent = path;
  const rows = [
    ["Hostname", info.hostname],
    ["Host header", info.host],
    ["TLS version", info.tls_version || "none"],
    ["Cipher", info.cipher || "—"],
    ["ALPN", info.alpn || "—"],
    ["Key", formatKey(info.server_cert_key_algorithm, info.server_cert_key_size, info.server_cert_curve)],
    ["SNI", info.sni || "—"],
    ["X-Forwarded-Proto", info.x_forwarded_proto || "—"],
    ["X-Forwarded-For", info.x_forwarded_for || "—"],
    ["Client", info.client_addr],
    ["Server CN", info.server_cert_cn || "—"],
    ["Cert SANs", (info.server_cert_sans || []).join(", ") || "—"],
  ];
  details.innerHTML = rows
    .map(([key, value]) => `<dt>${key}</dt><dd>${value || "—"}</dd>`)
    .join("");
}

function renderBlobs(blobs) {
  const list = document.getElementById("blob-list");
  if (!blobs || blobs.length === 0) {
    list.innerHTML = "<li><span>No blobs on the PVC yet.</span></li>";
    return;
  }
  list.innerHTML = blobs
    .map(
      (blob) =>
        `<li><span>${escapeHtml(blob.name)}</span><span>${formatBytes(blob.bytes)}</span></li>`
    )
    .join("");
}

function renderRow(run, options = {}) {
  const {
    summary = false,
    samples = [],
    groupId = "",
    expanded = false,
    hidden = false,
    sample = false,
  } = options;
  const cols = activeColumns();
  const classParts = [];
  if (summary) classParts.push("summary-row");
  if (sample) classParts.push("sample-row");
  if (hidden) classParts.push("collapsed");

  const attrParts = [];
  if (classParts.length) {
    attrParts.push(`class="${classParts.join(" ")}"`);
  }
  if (summary && groupId) {
    attrParts.push(`data-group-id="${escapeAttr(groupId)}"`);
  }
  if (sample && groupId) {
    attrParts.push(`data-parent-group="${escapeAttr(groupId)}"`);
  }

  const cells = cols
    .map((col) => `<td data-col="${col.id}">${cellHTML(col, run, { summary, samples, expanded, groupId })}</td>`)
    .join("");

  return `<tr ${attrParts.join(" ")}>${cells}</tr>`;
}

function bindExpandHandlers() {
  document.querySelectorAll(".expand-btn").forEach((btn) => {
    btn.onclick = (event) => {
      event.stopPropagation();
      toggleGroup(btn.dataset.group);
    };
  });
  document.querySelectorAll("tr.summary-row[data-group-id]").forEach((row) => {
    row.onclick = () => toggleGroup(row.dataset.groupId);
  });
}

function toggleGroup(groupId) {
  if (expandedGroups.has(groupId)) {
    expandedGroups.delete(groupId);
  } else {
    expandedGroups.add(groupId);
  }
  renderResults(lastRuns);
}

let lastRuns = [];

function renderResults(runs) {
  lastRuns = runs || [];
  const body = document.getElementById("results-body");
  const opFilter = document.getElementById("filter-op").value;
  const tlsFilter = document.getElementById("filter-tls").value;
  const routeFilter = document.getElementById("filter-route").value;
  const groupExperiments = document.getElementById("group-experiments").checked;
  const colCount = activeColumns().length;

  const filtered = (runs || []).filter((run) => {
    if (opFilter && run.operation !== opFilter) return false;
    if (tlsFilter === "true" && !run.tls) return false;
    if (tlsFilter === "false" && run.tls) return false;
    if (routeFilter && run.route_mode !== routeFilter) return false;
    return true;
  });

  renderTableHead();

  if (filtered.length === 0) {
    body.innerHTML = `<tr><td colspan="${colCount}" class="empty">No runs match the current filters.</td></tr>`;
    return;
  }

  if (!groupExperiments) {
    body.innerHTML = filtered.map((run) => renderRow(run)).join("");
    return;
  }

  const { groups, singles } = groupRuns(filtered);
  const rows = [];
  for (const [key, samples] of groups.entries()) {
    if (samples.length < 2) {
      rows.push(renderRow(samples[0]));
      continue;
    }
    const latest = samples[0];
    const expanded = expandedGroups.has(key);
    rows.push(
      renderRow(latest, {
        summary: true,
        samples,
        groupId: key,
        expanded,
      })
    );
    for (const sample of samples) {
      rows.push(
        renderRow(sample, {
          sample: true,
          groupId: key,
          hidden: !expanded,
        })
      );
    }
  }
  for (const run of singles) {
    rows.push(renderRow(run));
  }
  body.innerHTML = rows.join("");
  bindExpandHandlers();
}

async function refresh() {
  try {
    const [info, blobs, results] = await Promise.all([
      fetchJSON("/api/info"),
      fetchJSON("/api/blobs"),
      fetchJSON("/api/results"),
    ]);
    renderConnection(info);
    renderBlobs(blobs.blobs || blobs);
    renderResults(results.runs || results);
  } catch (err) {
    document.getElementById("conn-summary").textContent = `Failed to load: ${err.message}`;
  }
}

function initColumnPicker() {
  renderColumnPicker();
  renderMetricsGuide();
  renderTableHead();

  const picker = document.getElementById("column-picker");
  const btn = document.getElementById("column-picker-btn");

  btn.addEventListener("click", () => {
    const open = picker.classList.toggle("hidden");
    btn.setAttribute("aria-expanded", String(!open));
  });

  document.getElementById("column-reset").addEventListener("click", () => {
    visibleColumns = new Set(COLUMNS.filter((c) => c.default || c.always).map((c) => c.id));
    saveVisibleColumns();
    renderColumnPicker();
    renderResults(lastRuns);
  });

  picker.addEventListener("change", (event) => {
    const input = event.target.closest("input[data-col]");
    if (!input) return;
    const colId = input.dataset.col;
    if (input.checked) {
      visibleColumns.add(colId);
    } else {
      visibleColumns.delete(colId);
    }
    saveVisibleColumns();
    renderResults(lastRuns);
  });

  document.addEventListener("click", (event) => {
    if (picker.classList.contains("hidden")) return;
    if (picker.contains(event.target) || btn.contains(event.target)) return;
    picker.classList.add("hidden");
    btn.setAttribute("aria-expanded", "false");
  });
}

document.getElementById("filter-op").addEventListener("change", refresh);
document.getElementById("filter-tls").addEventListener("change", refresh);
document.getElementById("filter-route").addEventListener("change", refresh);
document.getElementById("group-experiments").addEventListener("change", refresh);

initColumnPicker();
refresh();
setInterval(() => {
  if (document.getElementById("auto-refresh").checked) {
    refresh();
  }
}, refreshMs);
