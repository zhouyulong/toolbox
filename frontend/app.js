/* No framework, CDN, telemetry, or browser-side provider requests. */
'use strict';

const $ = (selector, root = document) => root.querySelector(selector);
const el = (tag, className, text) => {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
};
const button = (label, className, action) => {
  const node = el('button', className, label);
  node.type = 'button';
  node.addEventListener('click', () => Promise.resolve().then(action).catch(showError));
  return node;
};
const api = new Proxy({}, { get: (_, method) => (...args) => window.go.main.App[method](...args) });
const state = { data: null, toolId: '', session: null, sessions: [], runs: new Map(), drafts: new Map(), epoch: 0, modalCleanup: null };
const currentTool = () => state.data?.tools.find(t => t.id === state.toolId);
const currentRun = () => state.runs.get(state.toolId);
const draftKey = () => `${state.toolId}/${state.session?.id || 'new'}`;
const errorText = err => typeof err === 'string' ? err : err?.message || String(err);
let toastTimer;
function toast(text, error = false) {
  const node = $('#toast'); node.textContent = text; node.className = error ? 'error' : ''; node.hidden = false;
  clearTimeout(toastTimer); toastTimer = setTimeout(() => { node.hidden = true; }, error ? 8000 : 3500);
}
function showError(err) { toast(errorText(err), true); }

function providerName(id) { return state.data.providers.find(p => p.id === id)?.name || '未配置供应商'; }
async function refresh() { state.data = await api.GetState(); renderSidebar(); }
function renderSidebar() {
  $('#version').textContent = `v${state.data.version}`;
  $('#tool-count').textContent = state.data.tools.length;
  $('#provider-count').textContent = state.data.providers.length;
  const tools = $('#tools'); tools.replaceChildren();
  if (!state.data.tools.length) tools.append(el('div', 'empty-small', '把常用提示词变成你的专属工具。'));
  for (const tool of state.data.tools) {
    const node = button('', `tool-button${state.toolId === tool.id ? ' active' : ''}`, () => selectTool(tool.id));
    node.title = tool.loadError || tool.path;
    node.append(el('span', 'tool-mark', tool.loadError ? '!' : Array.from(tool.name)[0] || 'T'));
    const label = el('span', 'tool-label');
    label.append(el('strong', '', tool.name), el('small', '', tool.loadError ? '工作区无法读取' : tool.description || tool.model || '等待配置模型'));
    node.append(label);
    if (state.runs.has(tool.id)) node.append(el('span', 'running-dot'));
    tools.append(node);
  }
  const sessions = $('#sessions'); sessions.replaceChildren();
  $('#new-chat').disabled = !currentTool() || !!currentTool()?.loadError;
  if (!state.sessions.length) sessions.append(el('div', 'empty-small', state.toolId ? '会话会自动保存在这个工具的工作区。' : '选择工具后查看会话。'));
  for (const session of state.sessions) {
    const node = button(session.title, `session-button${state.session?.id === session.id ? ' active' : ''}`, () => selectSession(session.id));
    node.title = `${session.title}\n${new Date(session.updatedAt).toLocaleString()} · ${session.messageCount} 条消息`;
    sessions.append(node);
  }
}

async function selectTool(id) {
  const epoch = ++state.epoch;
  state.toolId = id; state.session = null; state.sessions = [];
  renderSidebar(); renderMain();
  const tool = currentTool(); if (!tool || tool.loadError) return;
  try {
    const sessions = await api.ListSessions(id);
    if (epoch !== state.epoch) return;
    state.sessions = sessions;
    const run = state.runs.get(id);
    const sessionId = run?.sessionId || sessions[0]?.id;
    if (sessionId) {
      const session = await api.GetSession(id, sessionId);
      if (epoch !== state.epoch) return;
      state.session = session;
    }
    renderSidebar(); renderMain();
  } catch (err) { if (epoch === state.epoch) showError(err); }
}

async function selectSession(id) {
  const toolId = state.toolId, epoch = ++state.epoch;
  const session = await api.GetSession(toolId, id);
  if (epoch !== state.epoch) return;
  state.session = session; renderSidebar(); renderMain();
}
function newChat() { if (!currentTool() || currentTool().loadError) return; ++state.epoch; state.session = null; renderSidebar(); renderMain(); $('#prompt')?.focus(); }

function renderMain() {
  const main = $('#main'); main.replaceChildren();
  const tool = currentTool();
  if (!tool) { renderWelcome(main); return; }
  const header = el('header', 'workspace-header');
  const titles = el('div'); titles.append(el('h1', 'workspace-title', tool.name));
  const sub = el('div', 'workspace-subtitle');
  sub.append(el('span', 'pill', tool.model || '尚未选择模型'), el('span', '', providerName(tool.providerId)));
  titles.append(sub); header.append(titles);
  const actions = el('div', 'header-actions');
  actions.append(button('工具设置', '', () => openToolModal(tool)));
  if (state.session) {
    actions.append(button('导出', '', async () => { const path = await api.ExportSession(tool.id, state.session.id); if (path) toast('会话已导出'); }));
    actions.append(button('更多 ···', '', openSessionMenu));
  }
  header.append(actions); main.append(header);
  const path = el('div', 'workspace-path', `独立工作区  ${tool.path}`); path.title = tool.path; main.append(path);
  if (tool.loadError) {
    const area = el('div', 'welcome'); area.append(el('h2', '', '工作区暂时无法读取'), el('p', '', tool.loadError), el('p', '', '请检查磁盘或目录是否可用。移动目录后可移除登记，再从新位置导入。'));
    area.append(button('重试读取', 'primary', async () => { await refresh(); await selectTool(tool.id); }), button('移除登记', 'danger', () => forgetTool(tool)));
    main.append(area); return;
  }
  const messages = el('section', 'messages'); messages.id = 'messages'; messages.setAttribute('aria-label', '会话消息'); main.append(messages);
  renderMessages(true);
  const area = el('div', 'composer-area'), composer = el('div', 'composer');
  const input = el('textarea'); input.id = 'prompt'; input.placeholder = '输入你的问题…'; input.setAttribute('aria-label', '提问内容'); input.value = state.drafts.get(draftKey()) || '';
  input.addEventListener('input', () => state.drafts.set(draftKey(), input.value));
  input.addEventListener('keydown', event => { if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) { event.preventDefault(); if (!currentRun()) send(); } });
  const footer = el('div', 'composer-footer'); footer.append(el('span', '', 'Enter 发送 · Shift + Enter 换行'));
  const sendButton = button('发送 ↑', 'primary', send); sendButton.id = 'send-button'; footer.append(sendButton);
  composer.append(input, footer); area.append(composer, el('div', 'composer-note', '会话保存在本地 · 请求仅发送至你配置的模型供应商'));
  main.append(area); updateComposer();
}

function renderWelcome(main) {
  const welcome = el('div', 'welcome');
  welcome.append(el('div', 'eyebrow', 'YOUR PERSONAL LLM WORKSPACE'), el('h1', '', '常用的提示词，\n变成顺手的工具。'), el('p', '', '为写作、阅读、代码或任何日常任务，建立独立的问答空间。工具做什么，由你的 system 提示词决定。'));
  const actions = el('div', 'welcome-actions'); actions.append(button('＋ 创建第一个工具', 'primary', () => openToolModal()), button('配置模型供应商', '', () => openProviders())); welcome.append(actions);
  const steps = el('div', 'steps');
  for (const [number, title, text] of [['01', '接入你的模型', '使用云端 API，或接入 Ollama、LM Studio 等本地兼容服务。'], ['02', '定义工具的性质', '编写 system 提示词，指定模型，为工具选择独立工作区。'], ['03', '打开就能问', '流式回复、历史会话、Markdown 导出。没有账户和额外工作流。']]) {
    const step = el('div', 'step'); step.append(el('div', 'step-number', number), el('h3', '', title), el('p', '', text)); steps.append(step);
  }
  welcome.append(steps, el('div', 'local-note', '轻量原生窗口  /  数据由自己保管  /  无登录 · 无遥测')); main.append(welcome);
}

// Render a safe, intentionally small Markdown subset. All model text is inserted
// through text nodes; HTML, scripts, and image/network embeds are never executed.
function inline(node, text) {
  const pattern = /(`[^`\n]+`|\*\*[^*\n]+\*\*)/g; let cursor = 0;
  for (const match of text.matchAll(pattern)) {
    node.append(document.createTextNode(text.slice(cursor, match.index)));
    node.append(el(match[0].startsWith('`') ? 'code' : 'strong', '', match[0].startsWith('`') ? match[0].slice(1, -1) : match[0].slice(2, -2)));
    cursor = match.index + match[0].length;
  }
  node.append(document.createTextNode(text.slice(cursor)));
}
function markdown(text) {
  const node = el('div', 'message-content');
  const lines = text.replace(/\r\n/g, '\n').split('\n'); let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (/^\s*```/.test(line)) {
      const code = []; i++;
      while (i < lines.length && !/^\s*```/.test(lines[i])) code.push(lines[i++]);
      const pre = el('pre'); pre.append(el('code', '', code.join('\n'))); node.append(pre); i++; continue;
    }
    if (!line.trim()) { i++; continue; }
    const heading = /^(#{1,4})\s+(.+)$/.exec(line);
    if (heading) { const h = el(`h${heading[1].length}`); inline(h, heading[2]); node.append(h); i++; continue; }
    if (/^\s*[-*]\s+/.test(line) || /^\s*\d+\.\s+/.test(line)) {
      const ordered = /^\s*\d+\./.test(line), list = el(ordered ? 'ol' : 'ul');
      const re = ordered ? /^\s*\d+\.\s+/ : /^\s*[-*]\s+/;
      while (i < lines.length && re.test(lines[i])) { const item = el('li'); inline(item, lines[i++].replace(re, '')); list.append(item); }
      node.append(list); continue;
    }
    if (/^>\s?/.test(line)) { const quote = el('blockquote'); inline(quote, line.replace(/^>\s?/, '')); node.append(quote); i++; continue; }
    const paragraph = [line]; i++;
    while (i < lines.length && lines[i].trim() && !/^\s*(```|#{1,4}\s|[-*]\s|\d+\.\s|>)/.test(lines[i])) paragraph.push(lines[i++]);
    const p = el('p'); inline(p, paragraph.join('\n')); node.append(p);
  }
  return node;
}

function renderMessages(forceScroll = false) {
  const list = $('#messages'); if (!list) return;
  const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 100;
  list.replaceChildren();
  const tool = currentTool(), run = currentRun();
  const messages = run && run.sessionId === state.session?.id ? run.messages : state.session?.messages || [];
  if (!messages.length) {
    const empty = el('div', 'empty-chat'); empty.append(el('div', 'large-mark', Array.from(tool.name)[0] || 'T'), el('h2', '', tool.name), el('p', '', tool.description || '这是你的独立问答空间。输入问题，开始一次新的对话。'));
    const preview = el('div', 'system-preview'); preview.append(el('small', '', 'SYSTEM · 这个工具的定义'), el('p', '', tool.system || '尚未设置 system 提示词，可在「工具设置」中定义。')); empty.append(preview); list.append(empty); return;
  }
  for (const message of messages) {
    const row = el('article', `message ${message.role}`); row.append(el('div', 'avatar', message.role === 'user' ? '你' : 'AI'));
    if (message.pending) row.dataset.streaming = 'yes';
    const body = el('div', 'message-body'), meta = el('div', 'message-meta');
    meta.append(el('strong', '', message.role === 'user' ? '你' : tool.name));
    if (message.model) meta.append(el('span', '', message.model));
    if (message.content) meta.append(button('复制', 'copy-button', async () => { await window.runtime.ClipboardSetText(message.content); toast('已复制'); }));
    body.append(meta);
    if (message.content) body.append(message.role === 'user' || message.pending ? plainText(message.content) : markdown(message.content));
    else if (message.pending) body.append(el('div', 'typing', '正在等待模型回复…'));
    if (message.error) body.append(el('div', 'message-error', message.error));
    row.append(body); list.append(row);
  }
  if (forceScroll || nearBottom) list.scrollTop = list.scrollHeight;
}
function plainText(text) { const node = el('div', 'message-content'); node.append(el('p', '', text)); return node; }
function updateComposer() {
  const sendButton = $('#send-button'); if (!sendButton) return;
  const run = currentRun(); sendButton.textContent = run?.creating ? '准备中…' : run ? '停止生成 ■' : '发送 ↑'; sendButton.className = run ? 'danger' : 'primary'; sendButton.disabled = !!run?.creating;
}

async function send() {
  const running = currentRun();
  if (running) { await api.CancelRequest(running.id); toast('正在停止请求…'); return; }
  const input = $('#prompt'), tool = currentTool(); if (!input || !tool) return;
  const content = input.value; if (!content.trim()) return;
  if (!tool.providerId || !tool.model) { toast('先在工具设置中选择供应商和模型', true); openToolModal(tool); return; }
  // Install a reservation before awaiting disk I/O to prevent double sends.
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  const id = Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
  const key = draftKey(), toolId = tool.id, sendEpoch = state.epoch;
  const run = { id, sessionId: state.session?.id || '', messages: [], creating: true };
  state.runs.set(toolId, run); updateComposer();
  try {
    const session = state.session || await api.NewSession(toolId);
    run.sessionId = session.id; run.creating = false;
    run.messages = [...session.messages, { role: 'user', content }, { role: 'assistant', content: '', model: tool.model, pending: true }];
    state.drafts.delete(key);
    if (state.toolId === toolId && state.epoch === sendEpoch) { state.session = session; input.value = ''; renderSidebar(); renderMain(); }
    const result = await api.SendMessage(toolId, session.id, content, id);
    if (state.toolId === toolId && state.session?.id === session.id) state.session = result.session;
    if (result.error && !result.cancelled) showError(result.error);
  } catch (err) {
    state.drafts.set(key, content);
    if (state.toolId === toolId && run.sessionId) {
      try { state.session = await api.GetSession(toolId, run.sessionId); } catch (_) { /* preserve the last visible session */ }
    }
    showError(err);
  } finally {
    state.runs.delete(toolId);
    if (state.toolId === toolId) {
      try { state.sessions = await api.ListSessions(toolId); } catch (err) { showError(err); }
      renderSidebar(); renderMain();
    } else renderSidebar();
  }
}
let deltaFrame = 0;
let lastPaint = 0;
function paintDelta() {
  const wait = 80 - (performance.now() - lastPaint);
  if (wait > 0) { deltaFrame = requestAnimationFrame(paintDelta); return; }
  deltaFrame = 0; lastPaint = performance.now();
  const run = currentRun(), list = $('#messages');
  if (!run || run.sessionId !== state.session?.id || !list) return;
  const body = $('[data-streaming="yes"] .message-body', list);
  if (!body) { renderMessages(); return; }
  const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 100;
  const text = run.messages[run.messages.length - 1].content;
  const paragraph = $('.message-content p', body);
  if (paragraph) paragraph.textContent = text;
  else { $('.typing', body)?.remove(); body.append(plainText(text)); }
  if (nearBottom) list.scrollTop = list.scrollHeight;
}
function onDelta(delta) {
  const run = state.runs.get(delta.toolId); if (!run || run.id !== delta.requestId || !run.messages.length) return;
  const message = run.messages[run.messages.length - 1]; message.content += delta.text;
  if (state.toolId === delta.toolId && state.session?.id === delta.sessionId && !deltaFrame) {
    deltaFrame = requestAnimationFrame(paintDelta);
  }
}

function closeModal() {
  if (state.modalCleanup) state.modalCleanup();
  state.modalCleanup = null; $('#modal-root').replaceChildren();
}
function modal(title, wide = false) {
  closeModal();
  const previousFocus = document.activeElement, backdrop = el('div', 'modal-backdrop'), panel = el('section', `modal${wide ? ' wide' : ''}`);
  panel.setAttribute('role', 'dialog'); panel.setAttribute('aria-modal', 'true'); panel.setAttribute('aria-label', title);
  const header = el('div', 'modal-header'); header.append(el('h2', '', title), button('×', 'icon-button', closeModal));
  const body = el('div', 'modal-body'), footer = el('div', 'modal-footer'); panel.append(header, body, footer); backdrop.append(panel); $('#modal-root').append(backdrop);
  const onKey = event => {
    if (event.key === 'Escape') closeModal();
    if (event.key === 'Tab') {
      const nodes = [...panel.querySelectorAll('button,input,select,textarea,a[href]')].filter(n => !n.disabled && n.offsetParent !== null);
      const first = nodes[0], last = nodes[nodes.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    }
  };
  document.addEventListener('keydown', onKey);
  state.modalCleanup = () => { document.removeEventListener('keydown', onKey); previousFocus?.focus(); };
  requestAnimationFrame(() => panel.querySelector('input,textarea,select,button')?.focus());
  return { body, footer };
}
function field(label, type = 'text', value = '', hint = '') {
  const wrapper = el('label', 'form-field'); wrapper.append(el('span', '', label));
  const input = type === 'textarea' ? el('textarea') : type === 'select' ? el('select') : el('input');
  if (!['textarea', 'select'].includes(type)) input.type = type;
  input.value = value; wrapper.append(input); if (hint) wrapper.append(el('p', 'hint', hint)); return { wrapper, input };
}
function options(select, entries, selected) {
  select.replaceChildren(); for (const [value, label] of entries) { const option = el('option', '', label); option.value = value; select.append(option); }
  select.value = selected; if (select.selectedIndex < 0 && select.options.length) select.selectedIndex = 0;
}
function checkbox(label, checked) { const wrapper = el('label', 'checkbox-label'), input = el('input'); input.type = 'checkbox'; input.checked = checked; wrapper.append(input, document.createTextNode(label)); return { wrapper, input }; }
function formAction(footer, label, action, errorNode) {
  const save = button(label, 'primary', async () => {
    save.disabled = true; errorNode.textContent = '';
    try { await action(); } catch (err) { errorNode.textContent = errorText(err); } finally { save.disabled = false; }
  }); footer.append(save); return save;
}

function openToolModal(tool = null) {
  const { body, footer } = modal(tool ? '工具设置' : '创建工具');
  const p = state.data.providers[0];
  const defaults = { name: '', description: '', system: '', providerId: p?.id || '', model: p?.models[0] || '', path: '', temperature: 0.7, maxTokens: 4096, historyTurns: 10, sendTemperature: true, tokenParameter: 'max_tokens' };
  const value = tool || defaults;
  const name = field('工具名称', 'text', value.name), description = field('简短说明', 'text', value.description);
  name.input.maxLength = 80; name.input.placeholder = '例如：论文润色、代码问答、英语教练'; description.input.placeholder = '帮助自己记住这个工具的用途';
  body.append(name.wrapper, description.wrapper);
  const system = field('System 提示词 · 由你定义工具的性质', 'textarea', value.system, '每次请求都携带这段提示词。OpenAI 兼容接口使用 system，其他协议映射为原生系统指令字段。');
  system.input.rows = 6; system.input.placeholder = '你是一名……\n你的任务是……\n输出要求：……'; body.append(system.wrapper);
  const row = el('div', 'form-row'), provider = field('模型供应商', 'select'), model = field('模型 ID', 'text', value.model);
  options(provider.input, [['', '请选择'], ...state.data.providers.map(p => [p.id, p.name])], value.providerId);
  const suggestions = el('datalist'); suggestions.id = 'model-suggestions'; model.input.setAttribute('list', suggestions.id);
  const updateModels = change => { const models = state.data.providers.find(p => p.id === provider.input.value)?.models || []; suggestions.replaceChildren(...models.map(m => { const o = el('option'); o.value = m; return o; })); if (change) model.input.value = models[0] || ''; };
  provider.input.addEventListener('change', () => updateModels(true)); updateModels(false);
  row.append(provider.wrapper, model.wrapper); body.append(row, suggestions);
  if (!state.data.providers.length) body.append(el('div', 'notice', '还没有供应商。可以先创建工具，再到左下角「模型供应商」配置。'));
  const path = field('独立工作区', 'text', value.path, tool ? '工作区路径不可直接修改。移动目录后，请移除登记并从新路径导入。' : `留空则自动建立独立目录：${state.data.defaultWorkspaceDir} / 随机 ID。自选目录必须为空。`);
  if (tool) path.input.readOnly = true;
  else { const pathRow = el('div', 'path-row'); path.input.replaceWith(pathRow); pathRow.append(path.input, button('选择目录', '', async () => { const selected = await api.ChooseWorkspace(); if (selected) path.input.value = selected; })); }
  body.append(path.wrapper);
  const advanced = el('details'); advanced.append(el('summary', 'hint', '请求参数与上下文'));
  const three = el('div', 'form-row three');
  const temperature = field('Temperature', 'number', value.temperature), maxTokens = field('最大输出 token', 'number', value.maxTokens), historyTurns = field('携带历史轮数', 'number', value.historyTurns);
  temperature.input.min = 0; temperature.input.max = 2; temperature.input.step = 0.1; maxTokens.input.min = 1; maxTokens.input.max = 128000; historyTurns.input.min = 0; historyTurns.input.max = 100;
  three.append(temperature.wrapper, maxTokens.wrapper, historyTurns.wrapper); advanced.append(three);
  const sendTemp = checkbox('发送 temperature 参数（不支持该参数的模型请关闭）', value.sendTemperature); advanced.append(sendTemp.wrapper);
  const token = field('OpenAI 兼容接口的输出限制参数', 'select', '', '部分推理模型要求 max_completion_tokens；Anthropic 和 Gemini 会自动映射各自的字段。');
  options(token.input, [['max_tokens', 'max_tokens'], ['max_completion_tokens', 'max_completion_tokens']], value.tokenParameter); advanced.append(token.wrapper, el('p', 'hint', '历史轮数为 0 时仅发送本次提问；失败或停止的回复不会加入后续上下文。参数范围还需符合所选模型的要求。')); body.append(advanced);
  const error = el('div', 'form-error'); body.append(error);
  if (tool) footer.append(button('移除登记', 'danger left', () => forgetTool(tool)));
  footer.append(button('取消', '', closeModal));
  formAction(footer, tool ? '保存设置' : '创建工具', async () => {
    const input = { ...value, name: name.input.value, description: description.input.value, system: system.input.value, providerId: provider.input.value, model: model.input.value.trim(), path: path.input.value.trim(), temperature: Number(temperature.input.value), maxTokens: Number(maxTokens.input.value), historyTurns: Number(historyTurns.input.value), sendTemperature: sendTemp.input.checked, tokenParameter: token.input.value };
    if (![input.temperature, input.maxTokens, input.historyTurns].every(Number.isFinite) || !Number.isInteger(input.maxTokens) || !Number.isInteger(input.historyTurns)) throw new Error('请填写有效的请求参数，token 和历史轮数必须为整数');
    const saved = tool ? await api.SaveTool(input) : await api.CreateTool(input);
    await refresh(); closeModal(); await selectTool(saved.id); toast(tool ? '工具设置已保存' : '工具已创建');
  }, error);
}

function confirmDialog(title, text, label, action) {
  const { body, footer } = modal(title); body.append(el('p', 'hint', text)); const error = el('div', 'form-error'); body.append(error);
  footer.append(button('取消', '', closeModal)); formAction(footer, label, action, error);
}
function forgetTool(tool) {
  confirmDialog('移除工具登记', `移除「${tool.name}」在工具箱中的登记。工作区文件会保留在 ${tool.path}，以后可再次导入。`, '移除登记', async () => {
    await api.ForgetTool(tool.id); await refresh(); state.toolId = ''; state.session = null; state.sessions = []; ++state.epoch; closeModal(); renderSidebar(); renderMain();
  });
}
async function importTool() { const path = await api.ChooseWorkspace(); if (!path) return; const tool = await api.ImportTool(path); await refresh(); await selectTool(tool.id); toast('工作区已导入，请确认供应商配置'); }

function openProviders(selectedId = '') {
  const { body, footer } = modal('模型供应商', true), layout = el('div', 'provider-layout'), list = el('div', 'provider-list'), editor = el('div');
  body.append(el('div', 'notice', 'API Key 仅保存在本机配置文件中，不会发送到其他供应商。留空可接入无需密钥的本地服务。'), layout); layout.append(list, editor);
  let selected = state.data.providers.find(p => p.id === selectedId) || null;
  for (const provider of state.data.providers) {
    const node = button('', `provider-item${provider.id === selectedId ? ' active' : ''}`, () => openProviders(provider.id)); node.append(el('strong', '', provider.name), el('small', '', `${provider.kind} · ${provider.models.length} 个模型`)); list.append(node);
  }
  list.append(button('＋ 添加供应商', 'primary', () => openProviders()));
  const value = selected || { id: '', name: '', kind: 'openai', baseUrl: 'https://api.openai.com/v1', models: [], timeoutSeconds: 180 };
  const name = field('名称', 'text', value.name), kind = field('接口协议', 'select'); options(kind.input, [['openai', 'OpenAI 兼容（含本地模型）'], ['anthropic', 'Anthropic Messages'], ['gemini', 'Google Gemini']], value.kind);
  const base = field('Base URL', 'text', value.baseUrl, '填写接口前缀，不包含 /chat/completions、/messages 或 /models/...。');
  kind.input.addEventListener('change', () => { base.input.value = { openai: 'https://api.openai.com/v1', anthropic: 'https://api.anthropic.com/v1', gemini: 'https://generativelanguage.googleapis.com/v1beta' }[kind.input.value]; });
  const key = field('API Key', 'password', '', selected?.hasApiKey ? '已保存密钥。留空保留原密钥；填写新值替换。' : '可留空；配置文件以本机明文保存，备份配置时请妥善保管。'); key.input.autocomplete = 'new-password';
  const clearKey = checkbox('删除已保存的密钥', false);
  const models = field('模型 ID · 每行一个', 'textarea', value.models.join('\n'), '使用供应商实际支持的模型 ID，不会自动拉取列表。工具也可手动填写其他模型 ID。'); models.input.rows = 4; models.input.placeholder = 'model-id-1\nmodel-id-2';
  const timeout = field('请求超时（秒）', 'number', value.timeoutSeconds, '包含完整流式生成时间，范围 10–1800 秒。'); timeout.input.min = 10; timeout.input.max = 1800;
  editor.append(name.wrapper, kind.wrapper, base.wrapper, key.wrapper); if (selected?.hasApiKey) editor.append(clearKey.wrapper); editor.append(models.wrapper, timeout.wrapper);
  const error = el('div', 'form-error'); editor.append(error);
  if (selected) footer.append(button('删除供应商', 'danger left', () => confirmDialog('删除供应商', `删除「${selected.name}」及其密钥。使用中的供应商需先从工具设置中更换。`, '删除', async () => { await api.DeleteProvider(selected.id); await refresh(); openProviders(); renderMain(); })));
  footer.append(button('关闭', '', closeModal));
  formAction(footer, '保存供应商', async () => {
    const seconds = Number(timeout.input.value); if (!Number.isInteger(seconds)) throw new Error('超时必须为整数秒');
    const saved = await api.SaveProvider({ id: value.id, name: name.input.value, kind: kind.input.value, baseUrl: base.input.value, apiKey: key.input.value, models: models.input.value.split(/\r?\n/), timeoutSeconds: seconds, clearApiKey: clearKey.input.checked });
    await refresh(); openProviders(saved.id); renderMain(); toast('供应商配置已保存');
  }, error);
}

function openSessionMenu() {
  const toolId = state.toolId, session = state.session; if (!session) return;
  const { body, footer } = modal('管理会话'); const title = field('会话标题', 'text', session.title); title.input.maxLength = 120; body.append(title.wrapper);
  const error = el('div', 'form-error'); body.append(error);
  footer.append(button('删除会话', 'danger left', () => confirmDialog('删除会话', `永久删除会话「${session.title}」及其本地 JSON 文件。`, '删除', async () => { await api.DeleteSession(toolId, session.id); closeModal(); await selectTool(toolId); })));
  footer.append(button('取消', '', closeModal)); formAction(footer, '保存标题', async () => { await api.RenameSession(toolId, session.id, title.input.value); closeModal(); await selectSession(session.id); state.sessions = await api.ListSessions(toolId); renderSidebar(); }, error);
}

function openHelp() {
  const { body, footer } = modal('使用指南'); body.classList.add('help');
  const sections = [
    ['1. 配置模型', '在左下角「模型供应商」填写协议、Base URL、API Key 和模型 ID。OpenAI 兼容服务使用 /v1 前缀；Ollama 通常为 http://localhost:11434/v1；LM Studio 通常为 http://localhost:1234/v1。本地服务必须已启动。'],
    ['2. 创建你的工具', '工具的用途由 system 提示词决定。为每个工具选择供应商、模型和独立空目录；也可留空让程序生成目录。这里没有 agent、插件执行或额外工具调用。'],
    ['3. 问答与历史', 'Enter 发送，Shift + Enter 换行。「停止生成」取消当前请求并保存已有输出。历史轮数只计算成功的完整问答，0 表示每次独立问答。修改 system 和模型只影响后续请求；旧消息仍保留。'],
    ['4. 数据与迁移', '工具目录包含 toolbox.workspace.json 和 chats/*.json。退出程序后复制整个目录即可备份；供应商及密钥另外保存在配置目录。换机导入工作区后，需重新选择本机供应商。移除工具只取消登记，删除会话会删除对应文件。不要同时用多个程序实例写入同一目录。'],
    ['5. 参数兼容', '不同模型的 token 和 temperature 限制不同。若收到参数错误，请在工具设置中关闭 temperature，或切换 max_completion_tokens。Gemini 和 Anthropic 会自动使用各自的系统指令与 token 字段。'],
    ['6. 文本与隐私', '只支持文本问答；不支持图片、附件、联网检索或 agent。Markdown 展示为安全的基础子集（标题、列表、粗体、行内代码、代码块、引用）；表格、链接等保留文本。无遥测或自动更新；请求内容会发往你配置的供应商。API Key 在本机配置文件中明文保存。'],
    ['7. 版本与维护', '源码附有 README、docs/USER_GUIDE.md、docs/DEVELOPMENT.md、docs/ARCHITECTURE.md、CHANGELOG.md 和跨平台发布流程。当前数据格式为 schemaVersion 1。更新前请备份工作区和配置。']
  ];
  for (const [title, text] of sections) body.append(el('h3', '', title), el('p', '', text));
  body.append(el('h3', '', '本机配置目录'), el('code', '', state.data.configDir), el('p', 'hint', `ToolboxSelf v${state.data.version}`)); footer.append(button('知道了', 'primary', closeModal));
}

$('#new-tool').addEventListener('click', () => openToolModal());
$('#import-tool').addEventListener('click', () => importTool().catch(showError));
$('#new-chat').addEventListener('click', newChat);
$('#providers-button').addEventListener('click', () => openProviders());
$('#help-button').addEventListener('click', openHelp);

async function init() {
  try {
    if (!window.go?.main?.App || !window.runtime) throw new Error('未连接桌面运行时。请通过 ToolboxSelf 桌面程序打开，不要直接打开 HTML 文件。');
    window.runtime.EventsOn('chat:delta', onDelta);
    await refresh();
    if (state.data.tools.length) await selectTool(state.data.tools[0].id); else renderMain();
  } catch (err) {
    const node = el('div', 'fatal'); node.append(el('h1', '', '应用初始化失败'), el('pre', '', errorText(err)), el('p', 'hint', '为避免覆盖已有数据，程序不会自动重置损坏或版本不兼容的配置。请参阅随源码提供的排错文档。')); $('#main').replaceChildren(node);
    for (const button of document.querySelectorAll('.sidebar button')) button.disabled = true;
  }
}
init();
