const refreshMs = 5000;

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

function badgeTLS(isTLS) {
  if (isTLS) return '<span class="badge badge-tls">TLS</span>';
  return '<span class="badge badge-http">HTTP</span>';
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
        `<li><span>${blob.name}</span><span>${formatBytes(blob.bytes)}</span></li>`
    )
    .join("");
}

function renderResults(runs) {
  const body = document.getElementById("results-body");
  const opFilter = document.getElementById("filter-op").value;
  const tlsFilter = document.getElementById("filter-tls").value;
  const filtered = (runs || []).filter((run) => {
    if (opFilter && run.operation !== opFilter) return false;
    if (tlsFilter === "true" && !run.tls) return false;
    if (tlsFilter === "false" && run.tls) return false;
    return true;
  });
  if (filtered.length === 0) {
    body.innerHTML = '<tr><td colspan="10" class="empty">No runs match the current filters.</td></tr>';
    return;
  }
  body.innerHTML = filtered
    .map(
      (run) => `<tr>
        <td>${formatTime(run.timestamp)}</td>
        <td><span class="badge badge-op">${run.operation}</span></td>
        <td>${run.name}</td>
        <td>${formatBytes(run.bytes)}</td>
        <td>${badgeTLS(run.tls)}</td>
        <td>${run.client_addr || "—"}</td>
        <td>${formatMs(run.write_ms)}</td>
        <td>${formatMs(run.read_ms)}</td>
        <td>${formatMs(run.total_ms)}</td>
        <td>${run.throughput_mib_s ? Number(run.throughput_mib_s).toFixed(3) : "—"}</td>
      </tr>`
    )
    .join("");
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

document.getElementById("filter-op").addEventListener("change", refresh);
document.getElementById("filter-tls").addEventListener("change", refresh);

refresh();
setInterval(() => {
  if (document.getElementById("auto-refresh").checked) {
    refresh();
  }
}, refreshMs);
