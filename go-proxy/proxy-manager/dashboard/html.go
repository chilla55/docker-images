package dashboard

// HTML returns the dashboard HTML template
func (d *Dashboard) getHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Proxy Manager Dashboard</title>
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); min-height: 100vh; padding: 20px; color: #333; }
    .container { max-width: 1400px; margin: 0 auto; background: white; border-radius: 12px; box-shadow: 0 20px 60px rgba(0,0,0,0.3); overflow: hidden; }
    header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; display: flex; justify-content: space-between; align-items: center; }
    header h1 { font-size: 28px; font-weight: 600; }
    .controls { display: flex; gap: 15px; }
    button { background: rgba(255,255,255,0.2); border: 1px solid rgba(255,255,255,0.3); color: white; padding: 8px 16px; border-radius: 6px; cursor: pointer; transition: all 0.2s; font-size: 14px; }
    button:hover { background: rgba(255,255,255,0.3); border-color: rgba(255,255,255,0.5); }
    .auto-refresh { display: flex; align-items: center; gap: 8px; font-size: 14px; }
    .content { padding: 30px; }
    .section { margin-bottom: 40px; }
    .section-title { font-size: 18px; font-weight: 600; margin-bottom: 15px; color: #333; border-bottom: 2px solid #667eea; padding-bottom: 10px; }
    .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin-bottom: 20px; }
    .stat-card { background: linear-gradient(135deg, #f5f7fa 0%, #c3cfe2 100%); padding: 20px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
    .stat-label { font-size: 12px; color: #666; text-transform: uppercase; letter-spacing: 1px; margin-bottom: 8px; }
    .stat-value { font-size: 28px; font-weight: 700; color: #333; }
    .routes-table { width: 100%; border-collapse: collapse; margin-top: 15px; }
    .routes-table th { background: #f5f7fa; padding: 12px; text-align: left; font-weight: 600; color: #666; border-bottom: 2px solid #e0e0e0; font-size: 13px; }
    .routes-table td { padding: 12px; border-bottom: 1px solid #e0e0e0; }
    .status-badge { display: inline-block; padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }
    .status-healthy { background: #d4edda; color: #155724; }
    .status-degraded { background: #fff3cd; color: #856404; }
    .status-down { background: #f8d7da; color: #721c24; }
    .status-maintenance { background: #cce5ff; color: #004085; }
    .status-draining { background: #e2e3e5; color: #383d41; }
    .status-disabled { background: #6c757d; color: #ffffff; }
    .cert-card { background: white; border: 1px solid #e0e0e0; border-radius: 8px; padding: 15px; border-left: 4px solid #667eea; margin-bottom: 10px; }
    .error-item { background: #fff5f5; border-left: 3px solid #dc3545; padding: 12px; margin-bottom: 8px; border-radius: 4px; font-size: 13px; }
    .empty-state { text-align: center; padding: 30px; color: #999; }
    .btn-primary { background: #667eea; color: white; border: none; padding: 10px 20px; border-radius: 6px; cursor: pointer; font-size: 14px; }
    .btn-primary:hover { background: #5568d3; }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>📊 Proxy Manager Dashboard</h1>
      <div class="controls">
        <button onclick="refreshData()">🔄 Refresh</button>
          <button onclick="openDebug()">🔍 Debug</button>
        <button onclick="copyAIContext()">📋 Copy AI</button>
      </div>
    </header>
    <div class="content">
      <div class="section">
        <div class="section-title">System Statistics</div>
        <div class="stats-grid" id="statsGrid">
          <div class="stat-card">
            <div class="stat-label">Uptime</div>
            <div class="stat-value"><span id="uptime">--</span></div>
          </div>
          <div class="stat-card">
            <div class="stat-label">Active Connections</div>
            <div class="stat-value"><span id="connections">--</span></div>
          </div>
          <div class="stat-card">
            <div class="stat-label">Requests/sec</div>
            <div class="stat-value"><span id="requestsPerSec">--</span></div>
          </div>
          <div class="stat-card">
            <div class="stat-label">Error Rate</div>
            <div class="stat-value"><span id="errorRate">--%</span></div>
          </div>
        </div>
      </div>
      <div class="section">
        <div class="section-title">Routes</div>
        <table class="routes-table"><thead><tr><th>Domain</th><th>Backend</th><th>Status</th><th>24h Req</th><th>Avg Response</th><th>Errors</th></tr></thead><tbody id="routesList"><tr><td colspan="6" class="empty-state">Loading...</td></tr></tbody></table>
      </div>
      <div class="section">
        <div class="section-title">Certificates</div>
        <div id="certsList">Loading...</div>
      </div>
      <div class="section">
        <div class="section-title">Recent Errors (Grouped)</div>
        <div id="errorsList">Loading...</div>
      </div>
      <div class="section">
        <div class="section-title">Debug Info</div>
        <button onclick="loadDebugData()" style="margin-bottom: 15px;">Load Debug Data</button>
        <div id="debugData" style="background: white; border: 1px solid #e0e0e0; border-radius: 8px; padding: 15px; font-family: monospace; font-size: 12px; max-height: 500px; overflow-y: auto; white-space: pre-wrap; word-wrap: break-word; display: none;"></div>
      </div>
      <div class="section">
        <div class="section-title">Available API Endpoints</div>
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(350px, 1fr)); gap: 15px;">
          <div style="background: #f5f7fa; padding: 12px; border-radius: 8px; border-left: 3px solid #667eea;">
            <div style="font-weight: 600; margin-bottom: 8px;">Dashboard & UI</div>
            <div style="font-size: 12px; line-height: 1.6;">
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/dashboard</span></div>
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/api/dashboard</span></div>
            </div>
          </div>
          <div style="background: #f5f7fa; padding: 12px; border-radius: 8px; border-left: 3px solid #667eea;">
            <div style="font-weight: 600; margin-bottom: 8px;">System Info</div>
            <div style="font-size: 12px; line-height: 1.6;">
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/api/dashboard/stats</span></div>
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/api/dashboard/debug</span></div>
            </div>
          </div>
          <div style="background: #f5f7fa; padding: 12px; border-radius: 8px; border-left: 3px solid #667eea;">
            <div style="font-weight: 600; margin-bottom: 8px;">Routes & Status</div>
            <div style="font-size: 12px; line-height: 1.6;">
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/api/dashboard/routes</span></div>
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/api/dashboard/maintenance</span></div>
            </div>
          </div>
          <div style="background: #f5f7fa; padding: 12px; border-radius: 8px; border-left: 3px solid #667eea;">
            <div style="font-weight: 600; margin-bottom: 8px;">Monitoring</div>
            <div style="font-size: 12px; line-height: 1.6;">
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/api/dashboard/certificates</span></div>
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/api/dashboard/errors</span></div>
            </div>
          </div>
          <div style="background: #f5f7fa; padding: 12px; border-radius: 8px; border-left: 3px solid #667eea;">
            <div style="font-weight: 600; margin-bottom: 8px;">Export</div>
            <div style="font-size: 12px; line-height: 1.6;">
              <div><span style="color: #28a745;">GET</span> <span style="color: #667eea;">/api/dashboard/context</span></div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
  <script>
    async function refreshData() {
      try {
        const response = await fetch('/api/dashboard');
        const data = await response.json();
        updateUI(data);
      } catch (err) {
        console.error('Failed to fetch:', err);
      }
    }
    function updateUI(data) {
      console.log('Dashboard data:', data);
      if (!data) {
        console.error('No data received');
        return;
      }
      
      // Update system stats if available
      if (data.system_stats) {
        const s = data.system_stats;
        document.getElementById('uptime').textContent = formatDuration(s.uptime_ms);
        document.getElementById('connections').textContent = s.active_connections;
        document.getElementById('requestsPerSec').textContent = s.requests_per_sec.toFixed(1);
        document.getElementById('errorRate').textContent = (s.error_rate * 100).toFixed(2) + '%';
      }
      
      // Update routes
      const routes = data.routes || [];
      console.log('Routes:', routes);
      updateRoutes(routes);
      
      // Update certificates
      const certs = data.certificates || [];
      console.log('Certificates:', certs);
      updateCerts(certs);
      
      // Update errors
      const errors = data.recent_errors || [];
      console.log('Recent errors:', errors);
      updateErrors(errors);
    }
    function updateRoutes(routes) {
      const tbody = document.getElementById('routesList');
      console.log('updateRoutes called with', routes.length, 'routes');
      
      if (!routes || routes.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="empty-state">No routes configured</td></tr>';
        return;
      }
      
      tbody.innerHTML = routes.map(r => {
        const statusClass = 'status-' + (r.status || 'unknown');
        const statusText = (r.status || 'unknown').toUpperCase();
        return '<tr>' +
          '<td>' + r.domain + r.path + '</td>' +
          '<td><small>' + r.backend + '</small></td>' +
          '<td><span class="status-badge ' + statusClass + '">' + statusText + '</span></td>' +
          '<td>' + (r.requests_24h || 0) + '</td>' +
          '<td>' + formatResponseTime(r.avg_response_time || 0) + '</td>' +
          '<td>' + ((r.error_rate || 0) * 100).toFixed(2) + '%</td>' +
          '</tr>';
      }).join('');
    }
    function updateCerts(certs) {
      const div = document.getElementById('certsList');
      if (!certs || certs.length === 0) {
        div.innerHTML = '<div class="empty-state">No certificates monitored</div>';
        return;
      }
      
      div.innerHTML = certs.map(c => {
        const severityClass = c.severity === 2 ? 'status-down' : (c.severity === 1 ? 'status-degraded' : 'status-healthy');
        const expiryDate = new Date(c.expires_at).toLocaleDateString();
        return '<div class="cert-card" style="padding: 12px; margin: 8px 0; background: #f9f9f9; border-left: 4px solid ' + 
               (c.severity === 2 ? '#e74c3c' : c.severity === 1 ? '#f39c12' : '#2ecc71') + ';">' +
               '<strong>' + c.domain + '</strong>' +
               '<span class="status-badge ' + severityClass + '" style="float: right;">' + c.status + '</span><br/>' +
               '<small>Issuer: ' + c.issuer + '</small><br/>' +
               '<small>Expires: ' + expiryDate + ' (' + c.days_left + ' days)</small>' +
               '</div>';
      }).join('');
    }
    
    function updateErrors(errs) {
      const div = document.getElementById('errorsList');
      if (!errs || errs.length === 0) {
        div.innerHTML = '<div class="empty-state">No recent errors</div>';
        return;
      }
      
      div.innerHTML = errs.map(e => {
        const lastTime = new Date(e.last_occurred).toLocaleString();
        const statusClass = e.status_code >= 500 ? 'status-down' : 'status-degraded';
        const countBadge = e.count > 1 ? '<span style="background: #e74c3c; color: white; padding: 2px 8px; border-radius: 12px; font-size: 0.85em; font-weight: bold; margin-left: 8px;">×' + e.count + '</span>' : '';
        return '<div class="error-item" style="padding: 10px 12px; margin: 6px 0; background: #fff3cd; border-left: 4px solid ' + 
               (e.status_code >= 500 ? '#e74c3c' : '#f39c12') + '; border-radius: 4px;">' +
               '<div style="display: flex; justify-content: space-between; align-items: center;">' +
               '<div>' +
               '<span class="status-badge ' + statusClass + '">' + e.status_code + '</span> ' +
               '<strong>' + e.domain + e.path + '</strong>' + countBadge +
               '</div>' +
               '<div style="text-align: right;">' +
               '<small style="color: #666;">Last: ' + lastTime + '</small>' +
               '</div>' +
               '</div>' +
               (e.error ? '<div style="margin-top: 6px;"><small style="color: #666; font-family: monospace;">' + e.error + '</small></div>' : '') +
               '</div>';
      }).join('');
    }
    async function copyAIContext() {
      try {
        const response = await fetch('/api/dashboard/context');
        const text = await response.text();
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(text);
        } else {
          const ta = document.createElement('textarea');
          ta.value = text;
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
        }
        alert('Copied to clipboard');
      } catch (err) {
        console.error('Failed:', err);
      }
    }
    async function loadDebugData() {
      try {
        const response = await fetch('/api/dashboard/debug');
        const data = await response.json();
        const debugDiv = document.getElementById('debugData');
        debugDiv.textContent = JSON.stringify(data, null, 2);
        debugDiv.style.display = 'block';
      } catch (err) {
        console.error('Failed to load debug data:', err);
        document.getElementById('debugData').textContent = 'Error loading debug data: ' + err.message;
        document.getElementById('debugData').style.display = 'block';
      }
    }
    function openDebug() {
      const debugSection = document.getElementById('debugData');
      loadDebugData();
      if (debugSection) {
        debugSection.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
    }
    function formatDuration(ms) {
      if (typeof ms === 'string') {
        const p = ms.match(/(\d+)([smh])/g) || [];
        let total = 0;
        p.forEach(x => {
          const n = parseInt(x);
          if (x.includes('h')) total += n * 3600000;
          else if (x.includes('m')) total += n * 60000;
          else total += n * 1000;
        });
        ms = total;
      }
      const d = Math.floor(ms / 86400000);
      const h = Math.floor((ms % 86400000) / 3600000);
      const m = Math.floor((ms % 3600000) / 60000);
      if (d > 0) return d + 'd ' + h + 'h';
      if (h > 0) return h + 'h ' + m + 'm';
      if (m > 0) return m + 'm';
      return '<1m';
    }
    function formatResponseTime(ms) {
      // Format response time in ms or seconds
      if (ms >= 1000) {
        return (ms / 1000).toFixed(2) + 's';
      } else if (ms >= 1) {
        return ms.toFixed(1) + 'ms';
      } else if (ms > 0) {
        return '<1ms';
      }
      return '0ms';
    }
    refreshData();
    setInterval(refreshData, 5000);
  </script>
</body>
</html>`
}
