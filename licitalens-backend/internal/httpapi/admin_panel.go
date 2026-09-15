package httpapi

var adminPanelHTML = []byte(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>LicitaLens Admin</title>
  <style>
    :root { color-scheme: light; font-family: Inter, system-ui, sans-serif; }
    body { margin: 0; background: #f4f6f9; color: #10233f; }
    header { background: #10233f; color: white; padding: 20px 24px; }
    main { max-width: 1100px; margin: 0 auto; padding: 24px; }
    .toolbar { display: flex; gap: 12px; flex-wrap: wrap; margin-bottom: 20px; }
    input, button { border-radius: 10px; border: 1px solid #d0d5dd; padding: 10px 12px; font: inherit; }
    button { background: #2563eb; color: white; border: none; cursor: pointer; font-weight: 700; }
    .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; margin-bottom: 24px; }
    .card { background: white; border: 1px solid #e4e7ec; border-radius: 16px; padding: 16px; }
    .card strong { display: block; font-size: 28px; margin-top: 8px; }
    table { width: 100%; border-collapse: collapse; background: white; border-radius: 16px; overflow: hidden; border: 1px solid #e4e7ec; }
    th, td { padding: 12px 14px; text-align: left; border-bottom: 1px solid #eef1f4; font-size: 14px; }
    th { background: #f8fafc; font-size: 12px; text-transform: uppercase; letter-spacing: .04em; color: #667085; }
    h2 { margin: 28px 0 12px; font-size: 18px; }
    .muted { color: #667085; font-size: 13px; }
    .error { color: #b42318; margin-bottom: 12px; }
  </style>
</head>
<body>
  <header>
    <h1>LicitaLens · Operações comerciais</h1>
    <p class="muted" style="color:#c8d6e8;margin:8px 0 0">Planos vigentes, organizações e histórico de assinaturas</p>
  </header>
  <main>
    <div class="toolbar">
      <input id="adminKey" type="password" placeholder="X-Admin-Key (opcional em demo)" style="min-width:280px" />
      <button id="refresh">Atualizar painel</button>
      <button id="runAlerts" type="button">Rodar alertas agora</button>
    </div>
    <div id="error" class="error"></div>
    <section class="cards" id="cards"></section>
    <h2>Organizações</h2>
    <table>
      <thead><tr><th>Empresa</th><th>Plano</th><th>Status assinatura</th><th>Pipeline</th><th>Membros</th><th>Criada em</th></tr></thead>
      <tbody id="orgs"></tbody>
    </table>
    <h2>Histórico de assinaturas</h2>
    <table>
      <thead><tr><th>Data</th><th>Empresa</th><th>Plano</th><th>Status</th><th>Evento</th></tr></thead>
      <tbody id="history"></tbody>
    </table>
  </main>
  <script>
    const cards = document.getElementById('cards');
    const orgs = document.getElementById('orgs');
    const history = document.getElementById('history');
    const error = document.getElementById('error');
    const adminKey = document.getElementById('adminKey');
    function headers() {
      const value = { Accept: 'application/json' };
      if (adminKey.value.trim()) value['X-Admin-Key'] = adminKey.value.trim();
      return value;
    }
    async function load(path, init) {
      const response = await fetch(path, { ...(init || {}), headers: { ...headers(), ...((init && init.headers) || {}) } });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.message || ('HTTP ' + response.status));
      return body;
    }
    function fmtDate(value) {
      if (!value) return '—';
      return new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value));
    }
    async function refresh() {
      error.textContent = '';
      try {
        const overview = await load('/v1/admin/overview');
        cards.innerHTML = [
          ['Organizações', overview.organizations],
          ['Assinaturas ativas', overview.active_subscriptions],
          ['Plano Essencial', overview.essential_plans],
          ['Plano Pro', overview.pro_plans],
          ['Eventos no histórico', overview.history_events],
        ].map(([label, value]) => '<div class="card"><span class="muted">' + label + '</span><strong>' + value + '</strong></div>').join('');
        const orgData = await load('/v1/admin/organizations');
        orgs.innerHTML = (orgData.data || []).map((item) => '<tr><td>' + item.name + '</td><td>' + (item.plan || '—') + '</td><td>' + (item.subscription_status || '—') + '</td><td>' + (item.pipeline_deals ?? 0) + '</td><td>' + item.member_count + '</td><td>' + fmtDate(item.created_at) + '</td></tr>').join('') || '<tr><td colspan="6">Nenhuma organização</td></tr>';
        const historyData = await load('/v1/admin/subscription-history');
        history.innerHTML = (historyData.data || []).map((item) => '<tr><td>' + fmtDate(item.recorded_at) + '</td><td>' + (item.organization_name || item.organization_id) + '</td><td>' + item.plan + '</td><td>' + item.status + '</td><td>' + (item.stripe_event_id || '—') + '</td></tr>').join('') || '<tr><td colspan="5">Sem eventos ainda</td></tr>';
      } catch (cause) {
        error.textContent = cause.message || 'Falha ao carregar painel';
      }
    }
    document.getElementById('refresh').addEventListener('click', refresh);
    document.getElementById('runAlerts').addEventListener('click', async () => {
      error.textContent = '';
      try {
        const result = await load('/v1/admin/notifications/run', { method: 'POST' });
        error.style.color = '#027a48';
        error.textContent = 'Alertas: oportunidades ' + result.opportunity_alerts_sent + ', follow-ups ' + result.followup_reminders_sent + ' (ver Mailpit)';
      } catch (cause) {
        error.style.color = '#b42318';
        error.textContent = cause.message || 'Falha ao rodar alertas';
      }
    });
    refresh();
  </script>
</body>
</html>`)
