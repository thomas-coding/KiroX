// ===== Kiro CLI 账号管理 =====

var kiroCliState = {
  accounts: [],
  currentEmail: '',
  outputDir: '',
  logPath: '',
  statusByEmail: {},
  runningByEmail: {},
  confirmResolve: null
};

function kiroCliEscape(s) {
  if (s == null) return '';
  return String(s).replace(/[&<>"']/g, function(c) {
    return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
  });
}

function kiroCliStatusLabel(status) {
  var labels = {
    idle: '未检测',
    checking: '预检中',
    available: '可用',
    importing: '导入中',
    imported: '已导入',
    suspended: '已封禁',
    error: '失败'
  };
  return labels[status || 'idle'] || status;
}

function kiroCliStatusColor(status) {
  if (status === 'available' || status === 'imported') return '#10b981';
  if (status === 'checking' || status === 'importing') return '#3b82f6';
  if (status === 'suspended') return '#f59e0b';
  if (status === 'error') return '#ef4444';
  return 'var(--muted)';
}

async function reloadKiroCliAccounts() {
  try {
    var res = await window.go.main.App.LoadKiroCliAccounts();
    if (!res || !res.success) {
      showToast((res && res.error) || '加载账号失败', 'error');
      return;
    }
    kiroCliState.accounts = res.accounts || [];
    kiroCliState.currentEmail = res.currentEmail || '';
    kiroCliState.outputDir = res.outputDir || '';
    kiroCliState.logPath = res.logPath || '';
    var dirEl = document.getElementById('kirocli-output-dir');
    if (dirEl) {
      var text = kiroCliState.outputDir ? '已加载：' + kiroCliState.outputDir : '已加载账号输出目录';
      if (kiroCliState.logPath) text += ' · 日志：' + kiroCliState.logPath;
      dirEl.textContent = text;
    }
    renderKiroCliTable();
    updateKiroCliSummary();
  } catch (e) {
    showToast('加载账号失败: ' + e, 'error');
  }
}

function getKiroCliRowStatus(account) {
  if (kiroCliState.currentEmail && account.email === kiroCliState.currentEmail) {
    var remembered = kiroCliState.statusByEmail[account.email];
    if (!remembered || remembered.status === 'idle' || remembered.status === 'available') {
      return { status: 'imported', error: '' };
    }
  }
  return kiroCliState.statusByEmail[account.email] || { status: 'idle', error: '' };
}

function renderKiroCliTable() {
  var body = document.getElementById('kirocli-table-body');
  if (!body) return;
  if (!kiroCliState.accounts.length) {
    body.innerHTML = '<tr><td colspan="7" style="padding:24px;text-align:center;color:var(--muted);font-size:13px;">输出目录下尚无 Kiro 账号。</td></tr>';
    return;
  }
  body.innerHTML = kiroCliState.accounts.map(function(a, idx) {
    var row = getKiroCliRowStatus(a);
    var status = row.status || 'idle';
    var busy = kiroCliState.runningByEmail[a.email];
    var isCurrent = kiroCliState.currentEmail && a.email === kiroCliState.currentEmail;
    var errTitle = row.error ? ' title="' + kiroCliEscape(row.error) + '"' : '';
    var statusHtml = '<span style="color:' + kiroCliStatusColor(status) + ';font-weight:600;cursor:' + (row.error ? 'help' : 'default') + ';"' + errTitle + '>' + kiroCliStatusLabel(status) + '</span>';
    var currentHtml = isCurrent ? '<span style="color:#10b981;font-weight:700;">当前</span>' : '<span style="color:var(--muted);">-</span>';
    var disabled = busy ? ' disabled' : '';
    return (
      '<tr>' +
        '<td style="padding:8px 12px;color:var(--muted);font-size:12px;">' + (idx + 1) + '</td>' +
        '<td style="padding:8px;font-weight:600;">' + kiroCliEscape(a.email || '') + '</td>' +
        '<td style="padding:8px;font-size:12px;color:var(--muted);">' + kiroCliEscape(a.subscription || '') + '</td>' +
        '<td style="padding:8px;font-size:12px;color:var(--muted);">' + kiroCliEscape(a.time || '') + '</td>' +
        '<td style="padding:8px;font-size:12px;">' + statusHtml + '</td>' +
        '<td style="padding:8px;font-size:12px;">' + currentHtml + '</td>' +
        '<td style="padding:8px 12px;text-align:right;display:flex;gap:4px;justify-content:flex-end;flex-wrap:wrap;">' +
          '<button class="btn btn-secondary btn-sm" onclick="precheckKiroCliAccount(' + idx + ')"' + disabled + '>预检</button>' +
          '<button class="btn btn-dark btn-sm" onclick="importKiroCliAccount(' + idx + ')"' + disabled + '>导入</button>' +
          '<button class="btn btn-danger btn-sm" onclick="deleteKiroCliAccount(' + idx + ')"' + disabled + '>删除</button>' +
        '</td>' +
      '</tr>'
    );
  }).join('');
}

function updateKiroCliSummary() {
  var el = document.getElementById('kirocli-progress');
  if (!el) return;
  var total = kiroCliState.accounts.length;
  var available = 0, suspended = 0, imported = 0, failed = 0;
  kiroCliState.accounts.forEach(function(a) {
    var s = getKiroCliRowStatus(a).status;
    if (s === 'available') available++;
    if (s === 'suspended') suspended++;
    if (s === 'imported') imported++;
    if (s === 'error') failed++;
  });
  var parts = ['共 ' + total + ' 个'];
  if (available) parts.push('可用 ' + available);
  if (imported) parts.push('已导入 ' + imported);
  if (suspended) parts.push('封禁 ' + suspended);
  if (failed) parts.push('失败 ' + failed);
  el.textContent = parts.join(' · ');
}

function setKiroCliRowState(email, status, error) {
  kiroCliState.statusByEmail[email] = { status: status, error: error || '' };
  renderKiroCliTable();
  updateKiroCliSummary();
}

function kiroCliErrorText(res, fallback) {
  var err = (res && res.error) || fallback || '操作失败';
  if (res && res.stage) {
    return '[' + res.stage + '] ' + err;
  }
  return err;
}

function confirmKiroCliAction(title, message, okText) {
  var modal = document.getElementById('kirocli-confirm-modal');
  var titleEl = document.getElementById('kirocli-confirm-title');
  var messageEl = document.getElementById('kirocli-confirm-message');
  var okEl = document.getElementById('kirocli-confirm-ok');
  if (!modal || !titleEl || !messageEl || !okEl) {
    return Promise.resolve(false);
  }
  titleEl.textContent = title;
  messageEl.textContent = message;
  okEl.textContent = okText || '确认';
  modal.classList.add('show');
  return new Promise(function(resolve) {
    kiroCliState.confirmResolve = resolve;
  });
}

function closeKiroCliConfirm(confirmed) {
  var modal = document.getElementById('kirocli-confirm-modal');
  if (modal) modal.classList.remove('show');
  var resolve = kiroCliState.confirmResolve;
  kiroCliState.confirmResolve = null;
  if (resolve) resolve(!!confirmed);
}

async function precheckKiroCliAccount(idx) {
  var account = kiroCliState.accounts[idx];
  if (!account) return;
  kiroCliState.runningByEmail[account.email] = true;
  setKiroCliRowState(account.email, 'checking', '');
  try {
    var res = await window.go.main.App.PrecheckKiroCliAccount(account.email);
    if (res && res.success) {
      setKiroCliRowState(account.email, 'available', '');
      showToast('预检通过: ' + account.email);
    } else {
      var status = res && res.suspended ? 'suspended' : 'error';
      var err = kiroCliErrorText(res, '预检失败');
      setKiroCliRowState(account.email, status, err);
      showToast('预检失败: ' + err, 'error');
    }
  } catch (e) {
    setKiroCliRowState(account.email, 'error', String(e));
    showToast('预检失败: ' + e, 'error');
  } finally {
    delete kiroCliState.runningByEmail[account.email];
    renderKiroCliTable();
  }
}

async function importKiroCliAccount(idx) {
  var account = kiroCliState.accounts[idx];
  if (!account) return;
  kiroCliState.runningByEmail[account.email] = true;
  setKiroCliRowState(account.email, 'importing', '');
  try {
    var res = await window.go.main.App.ImportKiroCliAccount(account.email);
    if (res && res.success) {
      kiroCliState.currentEmail = account.email;
      setKiroCliRowState(account.email, 'imported', '');
      showToast('已导入官方 Kiro CLI: ' + account.email);
      await reloadKiroCliAccounts();
    } else {
      var status = res && res.suspended ? 'suspended' : 'error';
      var err = kiroCliErrorText(res, '导入失败');
      setKiroCliRowState(account.email, status, err);
      showToast('导入失败: ' + err, 'error');
    }
  } catch (e) {
    setKiroCliRowState(account.email, 'error', String(e));
    showToast('导入失败: ' + e, 'error');
  } finally {
    delete kiroCliState.runningByEmail[account.email];
    renderKiroCliTable();
  }
}

async function deleteKiroCliAccount(idx) {
  var account = kiroCliState.accounts[idx];
  if (!account) return;
  var confirmed = await confirmKiroCliAction(
    '确认删除账号',
    '将从 KiroX 账号输出文件中删除 ' + account.email + '。此操作不会修改官方 Kiro CLI 当前登录态。',
    '确认删除'
  );
  if (!confirmed) return;
  try {
    var res = await window.go.main.App.DeleteKiroCliAccount(account.email);
    if (res && res.success) {
      delete kiroCliState.statusByEmail[account.email];
      showToast('已删除账号: ' + account.email);
      await reloadKiroCliAccounts();
    } else {
      showToast('删除失败: ' + kiroCliErrorText(res, '删除失败'), 'error');
    }
  } catch (e) {
    showToast('删除失败: ' + e, 'error');
  }
}

async function deleteSuspendedKiroCliAccounts() {
  var emails = kiroCliState.accounts
    .filter(function(a) { return getKiroCliRowStatus(a).status === 'suspended'; })
    .map(function(a) { return a.email; });
  if (!emails.length) {
    showToast('没有已封禁账号');
    return;
  }
  var confirmed = await confirmKiroCliAction(
    '确认删除已封禁账号',
    '将从 KiroX 账号输出文件中删除 ' + emails.length + ' 个已标记为封禁的账号。此操作不会修改官方 Kiro CLI 当前登录态。',
    '确认删除'
  );
  if (!confirmed) return;
  try {
    var res = await window.go.main.App.DeleteSuspendedKiroCliAccounts(emails);
    if (res && res.success) {
      emails.forEach(function(email) { delete kiroCliState.statusByEmail[email]; });
      showToast('已删除 ' + (res.removed || 0) + ' 个已封禁账号');
      await reloadKiroCliAccounts();
    } else {
      showToast('删除失败: ' + kiroCliErrorText(res, '删除失败'), 'error');
    }
  } catch (e) {
    showToast('删除失败: ' + e, 'error');
  }
}

async function copyKiroCliStartCommand() {
  try {
    var cmd = await window.go.main.App.KiroCliStartCommand();
    await navigator.clipboard.writeText(cmd);
    showToast('已复制 Kiro CLI 启动命令');
  } catch (e) {
    showToast('复制失败: ' + e, 'error');
  }
}

async function copyKiroCliLogPath() {
  try {
    if (!kiroCliState.logPath) {
      var res = await window.go.main.App.LoadKiroCliAccounts();
      kiroCliState.logPath = (res && res.logPath) || '';
    }
    if (!kiroCliState.logPath) {
      showToast('日志路径尚未生成', 'error');
      return;
    }
    await navigator.clipboard.writeText(kiroCliState.logPath);
    showToast('已复制日志路径');
  } catch (e) {
    showToast('复制失败: ' + e, 'error');
  }
}
