// ===== Kiro Gateway 远程面板 =====

var kiroGatewayState = {
  gateway: null,
  loading: false
};

function gatewayStatusLabel(status) {
  var labels = {
    running: '运行中',
    degraded: '异常',
    cooldown: '冷却',
    unknown: '未知'
  };
  return labels[status || 'unknown'] || status;
}

function gatewayStatusClass(status) {
  if (status === 'running') return 'ok';
  if (status === 'degraded') return 'warn';
  if (status === 'cooldown') return 'bad';
  return 'idle';
}

function gatewayEscape(s) {
  return kiroCliEscape(s);
}

async function reloadKiroGatewayStatus() {
  if (kiroGatewayState.loading) return;
  kiroGatewayState.loading = true;
  var progress = document.getElementById('gateway-progress');
  if (progress) progress.textContent = '刷新中...';
  try {
    var res = await window.go.main.App.GetKiroGatewayStatus();
    if (!res || !res.success) {
      showToast((res && res.error) || '读取 Gateway 状态失败', 'error');
      renderGatewayError((res && res.error) || '读取失败');
      return;
    }
    kiroGatewayState.gateway = res.gateway;
    renderKiroGatewayStatus();
  } catch (e) {
    showToast('读取 Gateway 状态失败: ' + e, 'error');
    renderGatewayError(String(e));
  } finally {
    kiroGatewayState.loading = false;
  }
}

function renderGatewayError(message) {
  var body = document.getElementById('gateway-table-body');
  if (body) {
    body.innerHTML = '<tr><td colspan="8" style="padding:24px;text-align:center;color:#ef4444;font-size:13px;">' + gatewayEscape(message) + '</td></tr>';
  }
  var progress = document.getElementById('gateway-progress');
  if (progress) progress.textContent = '读取失败';
  setGatewayStat('gateway-stat-health', '失败');
  setGatewayStat('gateway-stat-accounts', '0');
  setGatewayStat('gateway-stat-bad', '0');
  setGatewayStat('gateway-stat-refresh', '-');
}

function setGatewayStat(id, value) {
  var node = document.getElementById(id);
  if (node) node.textContent = value;
}

function renderKiroGatewayStatus() {
  var gw = kiroGatewayState.gateway || {};
  var accounts = gw.accounts || [];
  var bad = accounts.filter(function(a) { return a.status !== 'running'; }).length;
  var meta = document.getElementById('gateway-meta');
  if (meta) {
    meta.textContent = (gw.name || 'old') + ' · ' + (gw.host || '-') + ' · ' + (gw.baseUrl || '-');
  }
  var progress = document.getElementById('gateway-progress');
  if (progress) {
    progress.textContent = '账号 ' + accounts.length + ' 个 · 容器 ' + (gw.containerText || '-');
  }
  setGatewayStat('gateway-stat-health', gw.healthy ? 'OK' : '异常');
  setGatewayStat('gateway-stat-accounts', String(accounts.length));
  setGatewayStat('gateway-stat-bad', String(bad));
  setGatewayStat('gateway-stat-refresh', gw.refreshedAt || '-');

  var body = document.getElementById('gateway-table-body');
  if (!body) return;
  if (!accounts.length) {
    body.innerHTML = '<tr><td colspan="8" style="padding:24px;text-align:center;color:var(--muted);font-size:13px;">服务器 accounts 目录没有账号 JSON。</td></tr>';
    return;
  }
  body.innerHTML = accounts.map(function(a, idx) {
    var profile = a.hasProfileArn ? gatewayEscape(a.profileArnTail || '已解析') : '<span class="kiro-life-muted">缺失</span>';
    var stats = (a.successfulRequests || 0) + ' 成功 / ' + (a.failedRequests || 0) + ' 失败';
    var email = a.email ? '<div class="kiro-life-sub">' + gatewayEscape(a.email) + '</div>' : '';
    return (
      '<tr>' +
        '<td>' + (idx + 1) + '</td>' +
        '<td><div class="kiro-life-email">' + gatewayEscape(a.file || '') + '</div>' + email + '<div class="kiro-life-sub">' + gatewayEscape(a.region || '-') + '</div></td>' +
        '<td><span class="kiro-life-pill ' + gatewayStatusClass(a.status) + '">' + gatewayStatusLabel(a.status) + '</span></td>' +
        '<td>' + profile + '</td>' +
        '<td class="kiro-life-muted">' + (a.failures || 0) + '</td>' +
        '<td class="kiro-life-muted">' + gatewayEscape(stats) + '<div class="kiro-life-sub">总计 ' + (a.totalRequests || 0) + '</div></td>' +
        '<td class="kiro-life-muted">' + gatewayEscape(a.lastFailureTime || '-') + '</td>' +
        '<td class="kiro-life-actions"><button class="btn btn-danger btn-sm" onclick="deleteRemoteGatewayAccount(' + idx + ')">删除</button></td>' +
      '</tr>'
    );
  }).join('');
}

async function deleteRemoteGatewayAccount(idx) {
  var gw = kiroGatewayState.gateway || {};
  var account = (gw.accounts || [])[idx];
  if (!account) return;
  var confirmed = await confirmKiroCliAction(
    '删除远程 Gateway 账号',
    '将删除 old 服务器上的 ' + account.file + '。此操作不会删除 KiroX 本地账号，删除后需要重启 Gateway 才会完全卸载运行态。',
    '确认删除'
  );
  if (!confirmed) return;
  try {
    var res = await window.go.main.App.DeleteKiroGatewayAccount(account.file);
    if (res && res.success) {
      showToast('已删除远程账号: ' + account.file);
      await reloadKiroGatewayStatus();
    } else {
      showToast('删除失败: ' + ((res && res.error) || '未知错误'), 'error');
    }
  } catch (e) {
    showToast('删除失败: ' + e, 'error');
  }
}

async function restartKiroGateway() {
  var confirmed = await confirmKiroCliAction(
    '重启 Kiro Gateway',
    '将重启 old 服务器上的 kiro-gateway 容器。正在使用中的请求可能会中断。',
    '确认重启'
  );
  if (!confirmed) return;
  try {
    var res = await window.go.main.App.RestartKiroGateway();
    if (res && res.success) {
      showToast('Gateway 已重启');
      await reloadKiroGatewayStatus();
    } else {
      showToast('重启失败: ' + ((res && res.error) || '未知错误'), 'error');
    }
  } catch (e) {
    showToast('重启失败: ' + e, 'error');
  }
}

async function testKiroGatewayChat() {
  try {
    var res = await window.go.main.App.KiroGatewayChatSmokeTest('claude-sonnet-4.5');
    if (res && res.success) {
      showToast('Gateway Chat 测试通过');
      await reloadKiroGatewayStatus();
    } else {
      showToast('Chat 测试失败: ' + ((res && res.error) || '未知错误'), 'error');
    }
  } catch (e) {
    showToast('Chat 测试失败: ' + e, 'error');
  }
}
