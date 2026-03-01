let inventory = [];
let selected = null;
let stream = null;
let paused = false;
let composeChildren = [];

const $ = (id) => document.getElementById(id);

async function fetchInventory() {
  const res = await fetch('/v1/inventory');
  const data = await res.json();
  inventory = data.items || [];
  renderItems();
}

function renderItems() {
  const q = $('search').value.toLowerCase();
  const host = $('items');
  host.innerHTML = '';

  inventory
    .filter(i => i.name.toLowerCase().includes(q) || i.target_id.toLowerCase().includes(q))
    .forEach(item => {
      const row = document.createElement('div');
      row.className = 'item';
      if (selected && selected.id === item.id) row.classList.add('active');
      row.innerHTML = `<div>${item.name}</div><div class="meta">${item.type} | ${item.target_id} | ${item.status}</div>`;
      row.onclick = () => selectItem(item);
      host.appendChild(row);
    });
}

async function selectItem(item) {
  selected = item;
  renderItems();

  $('title').textContent = item.name;
  $('subtitle').textContent = item.target_id;
  $('status').textContent = item.status;

  if (item.type === 'container') {
    const res = await fetch(`/v1/containers/${encodeURIComponent(item.id)}`);
    if (!res.ok) {
      $('details').textContent = `Failed to load container details (${res.status})`;
      return;
    }
    const c = await res.json();
    renderStats([
      ['Image', c.image],
      ['Restart count', c.restart_count],
      ['Uptime', c.uptime],
      ['Ports', (c.ports || []).join(', ') || '-'],
      ['Container ID', c.container_id],
      ['Target', c.target_id],
    ]);
    $('details').textContent = JSON.stringify(c, null, 2);
    composeChildren = [];
    renderServices([]);
    startLogs(c.id);
    switchTab('logs');
  } else if (item.type === 'compose') {
    const res = await fetch(`/v1/items/${encodeURIComponent(item.id)}/children`);
    if (!res.ok) {
      $('details').textContent = `Failed to load compose services (${res.status})`;
      return;
    }
    const children = await res.json();
    composeChildren = children;
    renderStats([
      ['Project', item.name],
      ['Services', children.length],
      ['Target', item.target_id],
      ['Status', item.status],
      ['Stack ID', item.id],
    ]);
    $('details').textContent = JSON.stringify(children, null, 2);
    renderServices(children);
    stopLogs();
    switchTab('services');
  } else {
    renderStats([['Error', item.name]]);
    $('details').textContent = JSON.stringify(item, null, 2);
    composeChildren = [];
    renderServices([]);
    stopLogs();
  }
}

function renderStats(items) {
  $('stats').innerHTML = items.map(([k, v]) => `<div class="stat"><div class="k">${k}</div><div class="v">${v}</div></div>`).join('');
}

function stopLogs() {
  if (stream) {
    stream.close();
    stream = null;
  }
}

function startLogs(id) {
  stopLogs();
  $('logs').textContent = '';
  stream = new EventSource(`/v1/containers/${encodeURIComponent(id)}/logs/stream?tail=200`);
  stream.onmessage = (evt) => {
    if (paused) return;
    $('logs').textContent += evt.data + '\n';
    if ($('autoScroll').checked) {
      $('logs').scrollTop = $('logs').scrollHeight;
    }
  };
  stream.onerror = () => {
    $('logs').textContent += '[stream error or disconnected]\n';
  };
}

function renderServices(children) {
  const host = $('services');
  if (!children || children.length === 0) {
    host.innerHTML = '<div class="muted">No compose services for current selection.</div>';
    return;
  }

  host.innerHTML = '';
  children.forEach((c) => {
    const btn = document.createElement('button');
    btn.className = 'service-item';
    btn.innerHTML = `<strong>${c.name}</strong><span class="service-meta">${c.status} | ${c.image || '-'} | ${c.container_id}</span>`;
    btn.onclick = () => selectItem({
      type: 'container',
      id: c.id,
      name: c.name,
      target_id: c.target_id,
      status: c.status,
    });
    host.appendChild(btn);
  });
}

async function post(url) {
  const token = localStorage.getItem('ccm_token') || '';
  const headers = token ? { Authorization: `Bearer ${token}` } : {};
  const res = await fetch(url, { method: 'POST', headers });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `request failed (${res.status})`);
  return body;
}

$('search').addEventListener('input', renderItems);
$('btnPause').onclick = () => {
  paused = !paused;
  $('btnPause').textContent = paused ? 'Resume' : 'Pause';
};
$('btnClear').onclick = () => {
  $('logs').textContent = '';
};
$('btnStart').onclick = async () => {
  if (selected?.type === 'container') await post(`/v1/containers/${encodeURIComponent(selected.id)}/start`);
};
$('btnStop').onclick = async () => {
  if (selected?.type === 'container') await post(`/v1/containers/${encodeURIComponent(selected.id)}/stop`);
};
$('btnRestart').onclick = async () => {
  if (selected?.type === 'container') await post(`/v1/containers/${encodeURIComponent(selected.id)}/restart`);
};
$('btnRedeploy').onclick = async () => {
  if (selected?.type === 'compose') await post(`/v1/compose/${encodeURIComponent(selected.id)}/redeploy`);
};

function switchTab(tab) {
  document.querySelectorAll('.tab').forEach((t) => {
    t.classList.toggle('active', t.dataset.tab === tab);
  });
  document.querySelectorAll('.panel').forEach((p) => p.classList.remove('panel-active'));
  document.getElementById(`panel${tab[0].toUpperCase() + tab.slice(1)}`).classList.add('panel-active');
}

document.querySelectorAll('.tab').forEach(btn => btn.onclick = () => switchTab(btn.dataset.tab));

function tickClock() {
  const now = new Date();
  $('clock').textContent = now.toLocaleTimeString();
}

(async function init() {
  tickClock();
  setInterval(tickClock, 1000);
  renderServices([]);
  await fetchInventory();
  setInterval(fetchInventory, 4000);
})();
