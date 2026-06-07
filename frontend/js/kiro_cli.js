// ===== Kiro 账号生命周期管理 =====

var kiroCliState = {
  accounts: [],
  currentEmail: '',
  outputDir: '',
  gatewayDir: '',
  statePath: '',
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

function kiroLifecycle(account) {
  var live = account.lifecycle || {};
  var email = account.email || '';
  var overlay = kiroCliState.statusByEmail[email];
  if (overlay) {
    live = Object.assign({}, live, overlay);
  }
  if (!live.status) live.status = 'new';
  return live;
}

function kiroCliStatusLabel(status) {
  var labels = {
    new: '新生产',
    checking: '预检中',
    available: '可用',
    gateway_exporting: '生成中',
    gateway_ready: 'Gateway',
    gateway_uploading: '上传中',
    gateway_uploaded: '已上传',
    importing: '导入中',
    cli_imported: 'CLI',
    limited: '限额',
    suspended: '封禁',
    retired: '停用',
    error: '失败'
  };
  return labels[status || 'new'] || status;
}

function kiroCliStatusClass(status) {
  if (status === 'available' || status === 'gateway_ready' || status === 'gateway_uploaded' || status === 'cli_imported') return 'ok';
  if (status === 'checking' || status === 'gateway_exporting' || status === 'gateway_uploading' || status === 'importing') return 'busy';
  if (status === 'limited' || status === 'retired') return 'warn';
  if (status === 'suspended' || status === 'error') return 'bad';
  return 'idle';
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
    kiroCliState.gatewayDir = res.gatewayDir || '';
    kiroCliState.statePath = res.statePath || '';
    kiroCliState.logPath = res.logPath || '';
    kiroCliState.statusByEmail = {};
    var dirEl = document.getElementById('kirocli-output-dir');
    if (dirEl) {
      var parts = [];
      if (kiroCliState.outputDir) parts.push('账号：' + kiroCliState.outputDir);
      if (kiroCliState.gatewayDir) parts.push('Gateway：' + kiroCliState.gatewayDir);
      dirEl.textContent = parts.join(' · ') || '已加载账号目录';
    }
    renderKiroCliTable();
    updateKiroCliSummary();
  } catch (e) {
    showToast('加载账号失败: ' + e, 'error');
  }
}

function getKiroCliRowStatus(account) {
  var live = kiroLifecycle(account);
  if (kiroCliState.currentEmail && account.email === kiroCliState.currentEmail) {
    if (live.status === 'new' || live.status === 'available' || live.status === 'error') {
      live = Object.assign({}, live, { status: 'cli_imported' });
    }
  }
  return live;
}

function kiroShortPath(path) {
  if (!path) return '-';
  var s = String(path);
  if (s.length <= 34) return s;
  return s.slice(0, 14) + '...' + s.slice(-18);
}

function kiroShortArn(arn) {
  if (!arn) return '<span class="kiro-life-muted">未解析</span>';
  var s = String(arn);
  var tail = s.split('/').pop() || s;
  return '<span title="' + kiroCliEscape(s) + '">' + kiroCliEscape(tail) + '</span>';
}

function kiroLastAction(live) {
  var pairs = [
    ['Gateway', live.lastGatewayExportAt],
    ['上传', live.lastGatewayUploadAt],
    ['CLI', live.lastCliImportAt],
    ['预检', live.lastPrecheckAt],
    ['更新', live.updatedAt]
  ];
  for (var i = 0; i < pairs.length; i++) {
    if (pairs[i][1]) return pairs[i][0] + ' · ' + pairs[i][1];
  }
  return '-';
}

function renderKiroCliTable() {
  var body = document.getElementById('kirocli-table-body');
  if (!body) return;
  if (!kiroCliState.accounts.length) {
    body.innerHTML = '<tr><td colspan="8" style="padding:24px;text-align:center;color:var(--muted);font-size:13px;">输出目录下尚无 Kiro 账号。</td></tr>';
    return;
  }
  body.innerHTML = kiroCliState.accounts.map(function(a, idx) {
    var live = getKiroCliRowStatus(a);
    var status = live.status || 'new';
    var busy = kiroCliState.runningByEmail[a.email];
    var isCurrent = kiroCliState.currentEmail && a.email === kiroCliState.currentEmail;
    var errTitle = live.lastError || live.error || live.note || '';
    var disabled = busy ? ' disabled' : '';
    var gatewayTitle = live.gatewayFile ? ' title="' + kiroCliEscape(live.gatewayFile) + '"' : '';
    var statusHtml = '<span class="kiro-life-pill ' + kiroCliStatusClass(status) + '"' + (errTitle ? ' title="' + kiroCliEscape(errTitle) + '"' : '') + '>' + kiroCliStatusLabel(status) + '</span>';
    var cliHtml = isCurrent ? '<span class="kiro-life-current">当前</span>' : '<span class="kiro-life-muted">-</span>';
    var profileHtml = kiroShortArn(live.profileArn);
    var gatewayHint = live.gatewayFile ? '<div class="kiro-life-sub"' + gatewayTitle + '>' + kiroCliEscape(kiroShortPath(live.gatewayFile)) + '</div>' : '';
    return (
      '<tr>' +
        '<td>' + (idx + 1) + '</td>' +
        '<td><div class="kiro-life-email">' + kiroCliEscape(a.email || '') + '</div><div class="kiro-life-sub">' + kiroCliEscape(a.time || '') + '</div>' + gatewayHint + '</td>' +
        '<td>' + statusHtml + '</td>' +
        '<td class="kiro-life-muted">' + kiroCliEscape(a.subscription || '-') + '</td>' +
        '<td>' + profileHtml + '</td>' +
        '<td>' + cliHtml + '</td>' +
        '<td class="kiro-life-muted">' + kiroCliEscape(kiroLastAction(live)) + '</td>' +
        '<td class="kiro-life-actions">' +
          '<button class="btn btn-secondary btn-sm" onclick="precheckKiroCliAccount(' + idx + ')"' + disabled + '>预检</button>' +
          '<button class="btn btn-dark btn-sm" onclick="exportKiroGatewayAccount(' + idx + ')"' + disabled + '>生成 Gateway JSON</button>' +
          '<button class="btn btn-dark btn-sm" onclick="uploadKiroGatewayAccount(' + idx + ')"' + disabled + '>上传 Gateway</button>' +
          '<button class="btn btn-secondary btn-sm" onclick="importKiroCliAccount(' + idx + ')"' + disabled + '>导入 CLI</button>' +
          '<button class="btn btn-secondary btn-sm" onclick="markKiroAccountLimited(' + idx + ')"' + disabled + '>限额</button>' +
          '<button class="btn btn-secondary btn-sm" onclick="markKiroAccountRetired(' + idx + ')"' + disabled + '>停用</button>' +
          '<button class="btn btn-danger btn-sm" onclick="markKiroAccountSuspended(' + idx + ')"' + disabled + '>封禁</button>' +
          '<button class="btn btn-danger btn-sm" onclick="deleteKiroCliAccount(' + idx + ')"' + disabled + '>删除</button>' +
        '</td>' +
      '</tr>'
    );
  }).join('');
}

function updateKiroCliSummary() {
  var el = document.getElementById('kirocli-progress');
  var total = kiroCliState.accounts.length;
  var counts = { new: 0, available: 0, gateway_ready: 0, bad: 0, cli_imported: 0, limited: 0, suspended: 0, retired: 0, error: 0 };
  kiroCliState.accounts.forEach(function(a) {
    var s = getKiroCliRowStatus(a).status || 'new';
    if (counts[s] == null) counts[s] = 0;
    counts[s]++;
    if (s === 'limited' || s === 'suspended' || s === 'retired' || s === 'error') counts.bad++;
  });
  if (el) {
    var parts = ['共 ' + total + ' 个'];
    if (counts.available) parts.push('可用 ' + counts.available);
    if (counts.gateway_ready) parts.push('Gateway ' + counts.gateway_ready);
    if (counts.gateway_uploaded) parts.push('已上传 ' + counts.gateway_uploaded);
    if (counts.cli_imported) parts.push('CLI ' + counts.cli_imported);
    if (counts.limited) parts.push('限额 ' + counts.limited);
    if (counts.suspended) parts.push('封禁 ' + counts.suspended);
    if (counts.retired) parts.push('停用 ' + counts.retired);
    if (counts.error) parts.push('失败 ' + counts.error);
    el.textContent = parts.join(' · ');
  }
  var ids = {
    'kiro-life-new': counts.new || 0,
    'kiro-life-available': counts.available || 0,
    'kiro-life-gateway': (counts.gateway_ready || 0) + (counts.gateway_uploaded || 0),
    'kiro-life-bad': counts.bad || 0
  };
  Object.keys(ids).forEach(function(id) {
    var node = document.getElementById(id);
    if (node) node.textContent = ids[id];
  });
}

function setKiroCliRowState(email, status, error, extra) {
  var next = Object.assign({}, kiroCliState.statusByEmail[email] || {}, extra || {});
  next.status = status;
  next.error = error || '';
  next.lastError = error || next.lastError || '';
  next.updatedAt = new Date().toLocaleString();
  kiroCliState.statusByEmail[email] = next;
  renderKiroCliTable();
  updateKiroCliSummary();
}

function applyKiroLifecycleState(email, state) {
  if (!state) return;
  kiroCliState.statusByEmail[email] = Object.assign({}, state);
  renderKiroCliTable();
  updateKiroCliSummary();
}

function kiroCliErrorText(res, fallback) {
  var err = (res && res.error) || fallback || '操作失败';
  if (res && res.stage) return '[' + res.stage + '] ' + err;
  return err;
}

function confirmKiroCliAction(title, message, okText) {
  var modal = document.getElementById('kirocli-confirm-modal');
  var titleEl = document.getElementById('kirocli-confirm-title');
  var messageEl = document.getElementById('kirocli-confirm-message');
  var okEl = document.getElementById('kirocli-confirm-ok');
  if (!modal || !titleEl || !messageEl || !okEl) return Promise.resolve(false);
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
      setKiroCliRowState(account.email, 'available', '', { lastPrecheckAt: new Date().toLocaleString(), lastError: '' });
      showToast('预检通过: ' + account.email);
      await reloadKiroCliAccounts();
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

async function exportKiroGatewayAccount(idx) {
  var account = kiroCliState.accounts[idx];
  if (!account) return;
  kiroCliState.runningByEmail[account.email] = true;
  setKiroCliRowState(account.email, 'gateway_exporting', '');
  try {
    var res = await window.go.main.App.ExportKiroGatewayAccount(account.email);
    if (res && res.success) {
      applyKiroLifecycleState(account.email, res.state);
      showToast('Gateway JSON 已生成: ' + (res.path || account.email));
      await reloadKiroCliAccounts();
    } else {
      var status = res && res.suspended ? 'suspended' : 'error';
      var err = kiroCliErrorText(res, '生成 Gateway JSON 失败');
      setKiroCliRowState(account.email, status, err);
      showToast('生成失败: ' + err, 'error');
    }
  } catch (e) {
    setKiroCliRowState(account.email, 'error', String(e));
    showToast('生成失败: ' + e, 'error');
  } finally {
    delete kiroCliState.runningByEmail[account.email];
    renderKiroCliTable();
  }
}

async function uploadKiroGatewayAccount(idx) {
  var account = kiroCliState.accounts[idx];
  if (!account) return;
  var live = getKiroCliRowStatus(account);
  if (!live.gatewayFile) {
    showToast('请先生成 Gateway JSON: ' + account.email, 'error');
    return;
  }
  var confirmed = await confirmKiroCliAction(
    '上传到 Gateway',
    '将上传 ' + account.email + ' 的 Gateway JSON 到 old 服务器，并重启 gateway 容器。',
    '确认上传'
  );
  if (!confirmed) return;
  kiroCliState.runningByEmail[account.email] = true;
  setKiroCliRowState(account.email, 'gateway_uploading', '');
  try {
    var res = await window.go.main.App.UploadKiroGatewayAccount(account.email);
    if (res && res.success) {
      applyKiroLifecycleState(account.email, res.state);
      showToast('已上传到 Gateway: ' + account.email);
      await reloadKiroCliAccounts();
      if (typeof reloadKiroGatewayStatus === 'function') {
        await reloadKiroGatewayStatus();
      }
    } else {
      var err = kiroCliErrorText(res, '上传 Gateway 失败');
      setKiroCliRowState(account.email, 'error', err);
      showToast('上传失败: ' + err, 'error');
    }
  } catch (e) {
    setKiroCliRowState(account.email, 'error', String(e));
    showToast('上传失败: ' + e, 'error');
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
      setKiroCliRowState(account.email, 'cli_imported', '', { lastCliImportAt: new Date().toLocaleString(), lastError: '' });
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

async function markKiroAccount(idx, status, title, message, note) {
  var account = kiroCliState.accounts[idx];
  if (!account) return;
  var confirmed = await confirmKiroCliAction(title, message, '确认标记');
  if (!confirmed) return;
  try {
    var res = await window.go.main.App.SetKiroAccountLifecycle(account.email, status, note || '');
    if (res && res.success) {
      applyKiroLifecycleState(account.email, res.state);
      showToast('状态已更新: ' + account.email);
    } else {
      showToast('更新失败: ' + kiroCliErrorText(res, '更新失败'), 'error');
    }
  } catch (e) {
    showToast('更新失败: ' + e, 'error');
  }
}

function markKiroAccountLimited(idx) {
  markKiroAccount(idx, 'limited', '标记限额用完', '账号会保留在列表中，但标记为限额。Gateway 侧如果已部署，需要另行移除或替换对应 JSON。', 'quota exhausted');
}

function markKiroAccountSuspended(idx) {
  markKiroAccount(idx, 'suspended', '标记封禁', '账号会保留在列表中并标为封禁。确认后可使用“删除已封禁”批量清理。', 'temporarily suspended');
}

function markKiroAccountRetired(idx) {
  markKiroAccount(idx, 'retired', '标记停用', '账号会保留在列表中，但标记为停用。适合手工下线、迁移或不再计划使用的账号。', 'retired manually');
}

async function deleteKiroCliAccount(idx) {
  var account = kiroCliState.accounts[idx];
  if (!account) return;
  var confirmed = await confirmKiroCliAction(
    '确认删除账号',
    '将从 KiroX 账号输出文件中删除 ' + account.email + '，并移除本地生命周期状态。此操作不会修改官方 Kiro CLI 当前登录态，也不会自动删除服务器 Gateway 文件。',
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
    '将从 KiroX 账号输出文件中删除 ' + emails.length + ' 个已标记为封禁的账号。此操作不会修改官方 Kiro CLI 当前登录态，也不会自动删除服务器 Gateway 文件。',
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

async function copyKiroGatewayDir() {
  try {
    var dir = kiroCliState.gatewayDir || await window.go.main.App.KiroGatewayExportDir();
    if (!dir) {
      showToast('Gateway 输出目录尚未生成', 'error');
      return;
    }
    await navigator.clipboard.writeText(dir);
    showToast('已复制 Gateway 目录');
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
