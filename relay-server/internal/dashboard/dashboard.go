package dashboard

import "net/http"

// Handler serves the Relay dashboard as a single self-contained HTML page.
type Handler struct{}

// NewHandler creates a dashboard handler.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeHTTP writes the dashboard HTML page.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Relay Dashboard</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: #0f1117;
    color: #e1e4e8;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    min-height: 100vh;
  }
  header {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 20px 32px;
    border-bottom: 1px solid #21262d;
  }
  header h1 {
    font-size: 22px;
    font-weight: 700;
    letter-spacing: -0.5px;
  }
  .live-dot {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: #3fb950;
    font-weight: 500;
    margin-left: 8px;
  }
  .live-dot::before {
    content: '';
    width: 8px;
    height: 8px;
    background: #3fb950;
    border-radius: 50%;
    animation: pulse 2s infinite;
  }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
  .container {
    max-width: 1100px;
    margin: 0 auto;
    padding: 24px 32px;
  }
  .stats-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
    margin-bottom: 24px;
  }
  .stat-card {
    background: #161b22;
    border: 1px solid #21262d;
    border-radius: 10px;
    padding: 24px;
    text-align: center;
  }
  .stat-card .label {
    font-size: 13px;
    color: #8b949e;
    text-transform: uppercase;
    letter-spacing: 1px;
    margin-bottom: 8px;
  }
  .stat-card .value {
    font-size: 48px;
    font-weight: 700;
    color: #f0f6fc;
  }
  .panel {
    background: #161b22;
    border: 1px solid #21262d;
    border-radius: 10px;
    margin-bottom: 24px;
    overflow: hidden;
  }
  .panel-header {
    padding: 16px 20px;
    border-bottom: 1px solid #21262d;
    font-size: 15px;
    font-weight: 600;
  }
  .panel-body { padding: 0; }
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th {
    text-align: left;
    padding: 10px 20px;
    font-size: 12px;
    color: #8b949e;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    border-bottom: 1px solid #21262d;
  }
  td {
    padding: 10px 20px;
    font-size: 14px;
    border-bottom: 1px solid #21262d;
  }
  tr:last-child td { border-bottom: none; }
  .badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 12px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .badge-public { background: #1f6feb22; color: #58a6ff; }
  .badge-private { background: #da363322; color: #f85149; }
  .badge-presence { background: #3fb95022; color: #3fb950; }
  .empty-state {
    padding: 32px 20px;
    text-align: center;
    color: #484f58;
    font-size: 14px;
  }
  .event-log {
    font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
    font-size: 13px;
    max-height: 420px;
    overflow-y: auto;
  }
  .event-log table { table-layout: fixed; }
  .event-log .col-time { width: 200px; }
  .event-log .col-channel { width: 40%; }
  .event-log .col-event { width: auto; }
  .event-log td { color: #c9d1d9; }
  .event-log .ts { color: #484f58; }
</style>
</head>
<body>
<header>
  <h1>Relay</h1>
  <span class="live-dot">Live</span>
</header>
<div class="container">
  <div class="stats-row">
    <div class="stat-card">
      <div class="label">Total Connections</div>
      <div class="value" id="conn-count">0</div>
    </div>
    <div class="stat-card">
      <div class="label">Active Channels</div>
      <div class="value" id="chan-count">0</div>
    </div>
  </div>

  <div class="panel">
    <div class="panel-header">Channels</div>
    <div class="panel-body" id="channel-list">
      <div class="empty-state">No active channels</div>
    </div>
  </div>

  <div class="panel">
    <div class="panel-header">Event Log</div>
    <div class="panel-body event-log" id="event-log">
      <div class="empty-state">No events yet</div>
    </div>
  </div>
</div>
<script>
(function() {
  const appId = 'relay-app';

  function fetchJSON(url) {
    return fetch(url).then(r => r.ok ? r.json() : null).catch(() => null);
  }

  async function pollStats() {
    const data = await fetchJSON('/stats');
    if (data) {
      document.getElementById('conn-count').textContent = data.connections;
      document.getElementById('chan-count').textContent = data.channels;
    }
  }

  async function pollChannels() {
    const data = await fetchJSON('/apps/' + appId + '/channels');
    const el = document.getElementById('channel-list');
    if (!data || !data.channels || data.channels.length === 0) {
      el.innerHTML = '<div class="empty-state">No active channels</div>';
      return;
    }
    let html = '<table><tr><th>Channel</th><th>Type</th><th>Subscribers</th></tr>';
    data.channels.forEach(function(ch) {
      const badge = 'badge-' + ch.type;
      html += '<tr><td>' + esc(ch.name) + '</td><td><span class="badge ' + badge + '">' + esc(ch.type) + '</span></td><td>' + ch.subscriber_count + '</td></tr>';
    });
    html += '</table>';
    el.innerHTML = html;
  }

  async function pollEvents() {
    const secret = '';
    const headers = {};
    // Event log endpoint requires Bearer auth — we embed the app secret via a meta tag if needed
    // For now, try without auth first (dashboard is internal)
    const resp = await fetch('/apps/' + appId + '/events/log', {
      headers: { 'Authorization': 'Bearer ' + getSecret() }
    });
    if (!resp.ok) return;
    const data = await resp.json();
    const el = document.getElementById('event-log');
    if (!data || !data.events || data.events.length === 0) {
      el.innerHTML = '<div class="empty-state">No events yet</div>';
      return;
    }
    let html = '<table><tr><th class="col-time">Timestamp</th><th class="col-channel">Channel</th><th class="col-event">Event</th></tr>';
    data.events.forEach(function(ev) {
      const t = ev.timestamp ? new Date(ev.timestamp).toLocaleTimeString() : '';
      html += '<tr><td class="ts">' + esc(t) + '</td><td>' + esc(ev.channel) + '</td><td>' + esc(ev.event) + '</td></tr>';
    });
    html += '</table>';
    el.innerHTML = html;
  }

  function getSecret() {
    // Read from query param ?secret= for dashboard access
    const params = new URLSearchParams(window.location.search);
    return params.get('secret') || '';
  }

  function esc(s) {
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  // Initial fetch
  pollStats();
  pollChannels();
  pollEvents();

  // Polling intervals
  setInterval(pollStats, 2000);
  setInterval(pollChannels, 3000);
  setInterval(pollEvents, 2000);
})();
</script>
</body>
</html>`
