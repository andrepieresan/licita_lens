package httpapi

var adminPanelHTML = []byte(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>LicitaLens · Operações</title>
  <style>
    :root { color-scheme: light; font-family: Inter, system-ui, sans-serif; }
    body { margin: 0; background: #f4f6f9; color: #10233f; }
    header { background: #10233f; color: white; padding: 20px 24px; display: flex; justify-content: space-between; align-items: center; gap: 16px; flex-wrap: wrap; }
    main { max-width: 1100px; margin: 0 auto; padding: 24px; }
    .toolbar { display: flex; gap: 12px; flex-wrap: wrap; margin-bottom: 20px; align-items: center; }
    input, select, button, textarea { border-radius: 10px; border: 1px solid #d0d5dd; padding: 10px 12px; font: inherit; }
    button { background: #2563eb; color: white; border: none; cursor: pointer; font-weight: 700; }
    button.secondary { background: #475467; }
    button.linkish { background: transparent; color: #2563eb; border: none; padding: 0; font-weight: 600; }
    .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; margin-bottom: 24px; }
    .card { background: white; border: 1px solid #e4e7ec; border-radius: 16px; padding: 16px; }
    .card strong { display: block; font-size: 28px; margin-top: 8px; }
    table { width: 100%; border-collapse: collapse; background: white; border-radius: 16px; overflow: hidden; border: 1px solid #e4e7ec; }
    th, td { padding: 12px 14px; text-align: left; border-bottom: 1px solid #eef1f4; font-size: 14px; vertical-align: top; }
    th { background: #f8fafc; font-size: 12px; text-transform: uppercase; letter-spacing: .04em; color: #667085; }
    h2 { margin: 28px 0 12px; font-size: 18px; }
    .muted { color: #667085; font-size: 13px; }
    .error { color: #b42318; margin-bottom: 12px; }
    .login-card { max-width: 420px; margin: 48px auto; background: white; border: 1px solid #e4e7ec; border-radius: 16px; padding: 24px; display: grid; gap: 12px; }
    .login-card h2 { margin: 0 0 8px; }
    label { display: grid; gap: 6px; font-size: 13px; font-weight: 600; }
    dialog { border: 1px solid #e4e7ec; border-radius: 16px; padding: 20px; max-width: 480px; }
    dialog form { display: grid; gap: 12px; }
  </style>
</head>
<body>
  <header>
    <div>
      <h1 style="margin:0;font-size:22px">LicitaLens · Operações</h1>
      <p class="muted" style="color:#c8d6e8;margin:8px 0 0">Operadores da plataforma — planos e organizações</p>
    </div>
    <div id="operatorBar" style="display:none">
      <span id="operatorName" class="muted" style="color:#c8d6e8"></span>
      <button id="logout" type="button" class="secondary" style="margin-left:12px">Sair</button>
    </div>
  </header>
  <main>
    <section id="loginSection" class="login-card">
      <h2>Entrar como operador</h2>
      <p class="muted">Conta da equipe LicitaLens (não é login de cliente).</p>
      <label>E-mail<input id="loginEmail" type="email" autocomplete="username" /></label>
      <label>Senha<input id="loginPassword" type="password" autocomplete="current-password" /></label>
      <button id="loginBtn" type="button">Entrar</button>
      <p class="muted">Integrações legadas podem usar <code>X-Admin-Key</code> nas APIs.</p>
    </section>
    <section id="appSection" style="display:none">
      <div class="toolbar">
        <button id="refresh" type="button">Atualizar painel</button>
        <button id="runAlerts" type="button" class="secondary">Rodar alertas agora</button>
      </div>
      <div id="error" class="error"></div>
      <section class="cards" id="cards"></section>
      <h2>Organizações</h2>
      <table>
        <thead><tr><th>Empresa</th><th>Plano</th><th>Status</th><th>Consumo</th><th>Membros</th><th></th></tr></thead>
        <tbody id="orgs"></tbody>
      </table>
      <h2>Histórico de assinaturas</h2>
      <table>
        <thead><tr><th>Data</th><th>Empresa</th><th>Plano</th><th>Status</th><th>Referência</th></tr></thead>
        <tbody id="history"></tbody>
      </table>
    </section>
  </main>
  <dialog id="editDialog">
    <form method="dialog" id="editForm">
      <h3 style="margin:0">Atualizar plano</h3>
      <p class="muted" id="editOrgName"></p>
      <label>Plano
        <select id="editPlan" required>
          <option value="essential">Essencial</option>
          <option value="pro">Pro</option>
        </select>
      </label>
      <label>Status
        <select id="editStatus" required>
          <option value="trialing">Trial</option>
          <option value="active">Ativo</option>
          <option value="past_due">Inadimplente</option>
          <option value="canceled">Cancelado</option>
        </select>
      </label>
      <label>Fim do período (RFC3339, opcional)<input id="editPeriodEnd" type="text" placeholder="2026-12-31T23:59:59Z" /></label>
      <label>Nota interna<textarea id="editNote" rows="2" placeholder="Ex.: contrato 2026 renovado"></textarea></label>
      <div style="display:flex;gap:8px;justify-content:flex-end">
        <button type="button" class="secondary" id="editCancel">Cancelar</button>
        <button type="submit">Salvar</button>
      </div>
    </form>
  </dialog>
  <script>
    const loginSection = document.getElementById('loginSection');
    const appSection = document.getElementById('appSection');
    const operatorBar = document.getElementById('operatorBar');
    const operatorName = document.getElementById('operatorName');
    const error = document.getElementById('error');
    const cards = document.getElementById('cards');
    const orgs = document.getElementById('orgs');
    const history = document.getElementById('history');
    const editDialog = document.getElementById('editDialog');
    let editingOrgId = '';
    let csrfToken = '';

    function escapeHtml(value) {
      return String(value ?? '').replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));
    }
    function fmtDate(value) {
      if (!value) return '—';
      return new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value));
    }
    async function api(path, init) {
      const headers = { Accept: 'application/json', ...(init && init.headers || {}) };
      if (csrfToken && init && init.method && init.method !== 'GET') headers['X-CSRF-Token'] = csrfToken;
      const response = await fetch(path, { credentials: 'include', ...(init || {}), headers });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.message || ('HTTP ' + response.status));
      return body;
    }
    function showApp(operator) {
      loginSection.style.display = 'none';
      appSection.style.display = 'block';
      operatorBar.style.display = 'block';
      operatorName.textContent = operator ? (operator.full_name + ' · ' + operator.email) : 'Operador';
    }
    function showLogin() {
      loginSection.style.display = 'grid';
      appSection.style.display = 'none';
      operatorBar.style.display = 'none';
    }
    async function bootstrapSession() {
      try {
        const me = await api('/v1/admin/auth/me');
        showApp(me.operator);
        await refresh();
      } catch (_) {
        showLogin();
      }
    }
    document.getElementById('loginBtn').addEventListener('click', async () => {
      error.textContent = '';
      try {
        const result = await api('/v1/admin/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            email: document.getElementById('loginEmail').value.trim(),
            password: document.getElementById('loginPassword').value,
          }),
        });
        csrfToken = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith('licitalens_platform_csrf='))?.split('=')[1] || '';
        showApp(result.operator);
        await refresh();
      } catch (cause) {
        error.textContent = cause.message || 'Falha no login';
      }
    });
    document.getElementById('logout').addEventListener('click', async () => {
      try { await api('/v1/admin/auth/logout', { method: 'POST' }); } catch (_) {}
      csrfToken = '';
      showLogin();
    });
    async function refresh() {
      error.textContent = '';
      try {
        const overview = await api('/v1/admin/overview');
        cards.innerHTML = [
          ['Organizações', overview.organizations],
          ['Assinaturas ativas', overview.active_subscriptions],
          ['Plano Essencial', overview.essential_plans],
          ['Plano Pro', overview.pro_plans],
          ['Eventos no histórico', overview.history_events],
        ].map(([label, value]) => '<div class="card"><span class="muted">' + escapeHtml(label) + '</span><strong>' + escapeHtml(value) + '</strong></div>').join('');
        const orgData = await api('/v1/admin/organizations');
        const rows = await Promise.all((orgData.data || []).map(async (item) => {
          let usageText = '—';
          try {
            const detail = await api('/v1/admin/organizations/' + encodeURIComponent(item.id));
            const u = detail.usage || {};
            usageText = 'IA ' + (u.ai_analyses?.used ?? 0) + '/' + (u.ai_analyses?.limit ?? '—') + ' · alertas ' + (u.daily_alerts?.used ?? 0) + '/' + (u.daily_alerts?.limit ?? '—');
          } catch (_) {}
          return '<tr><td>' + escapeHtml(item.name) + '</td><td>' + escapeHtml(item.plan || '—') + '</td><td>' + escapeHtml(item.subscription_status || '—') + '</td><td class="muted">' + escapeHtml(usageText) + '</td><td>' + escapeHtml(item.member_count) + '</td><td><button type="button" class="linkish" data-org="' + escapeHtml(item.id) + '" data-name="' + escapeHtml(item.name) + '" data-plan="' + escapeHtml(item.plan || 'essential') + '" data-status="' + escapeHtml(item.subscription_status || 'trialing') + '">Editar plano</button></td></tr>';
        }));
        orgs.innerHTML = rows.join('') || '<tr><td colspan="6">Nenhuma organização</td></tr>';
        orgs.querySelectorAll('button[data-org]').forEach((button) => button.addEventListener('click', () => openEdit(button.dataset)));
        const historyData = await api('/v1/admin/subscription-history');
        history.innerHTML = (historyData.data || []).map((item) => '<tr><td>' + escapeHtml(fmtDate(item.recorded_at)) + '</td><td>' + escapeHtml(item.organization_name || item.organization_id) + '</td><td>' + escapeHtml(item.plan) + '</td><td>' + escapeHtml(item.status) + '</td><td class="muted">' + escapeHtml(item.stripe_event_id || '—') + '</td></tr>').join('') || '<tr><td colspan="5">Sem eventos ainda</td></tr>';
      } catch (cause) {
        if ((cause.message || '').includes('401')) { showLogin(); return; }
        error.textContent = cause.message || 'Falha ao carregar painel';
      }
    }
    function openEdit(dataset) {
      editingOrgId = dataset.org;
      document.getElementById('editOrgName').textContent = dataset.name;
      document.getElementById('editPlan').value = dataset.plan || 'essential';
      document.getElementById('editStatus').value = dataset.status || 'active';
      document.getElementById('editPeriodEnd').value = '';
      document.getElementById('editNote').value = '';
      editDialog.showModal();
    }
    document.getElementById('editCancel').addEventListener('click', () => editDialog.close());
    document.getElementById('editForm').addEventListener('submit', async (event) => {
      event.preventDefault();
      error.textContent = '';
      const payload = {
        plan: document.getElementById('editPlan').value,
        status: document.getElementById('editStatus').value,
        note: document.getElementById('editNote').value.trim(),
      };
      const periodEnd = document.getElementById('editPeriodEnd').value.trim();
      if (periodEnd) payload.current_period_end = periodEnd;
      try {
        await api('/v1/admin/organizations/' + encodeURIComponent(editingOrgId) + '/subscription', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        editDialog.close();
        await refresh();
      } catch (cause) {
        error.textContent = cause.message || 'Falha ao salvar plano';
      }
    });
    document.getElementById('refresh').addEventListener('click', refresh);
    document.getElementById('runAlerts').addEventListener('click', async () => {
      error.textContent = '';
      try {
        const result = await api('/v1/admin/notifications/run', { method: 'POST' });
        error.style.color = '#027a48';
        error.textContent = 'Alertas: oportunidades ' + result.opportunity_alerts_sent + ', follow-ups ' + result.followup_reminders_sent;
      } catch (cause) {
        error.style.color = '#b42318';
        error.textContent = cause.message || 'Falha ao rodar alertas';
      }
    });
    bootstrapSession();
  </script>
</body>
</html>`)
