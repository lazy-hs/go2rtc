const extensionAPI = globalThis.browser || globalThis.chrome;

const STORAGE_DEFAULTS = {
    servers: [],
    activeServerId: '',
};

const LEGACY_DEFAULTS = {
    baseURL: 'http://localhost:1984',
    username: '',
    password: '',
    configured: false,
    pendingPermission: false,
};

const elements = {
    activeServerName: document.getElementById('active-server-name'),
    activeServerURL: document.getElementById('active-server-url'),
    activeServerVersion: document.getElementById('active-server-version'),
    activeStatusDot: document.getElementById('active-status-dot'),
    addServer: document.getElementById('add-server'),
    authorizeServer: document.getElementById('authorize-server'),
    baseURL: document.getElementById('base-url'),
    cancelDialog: document.getElementById('cancel-dialog'),
    closeDialog: document.getElementById('close-dialog'),
    copyToast: document.getElementById('copy-toast'),
    copyToastMessage: document.getElementById('copy-toast-message'),
    deleteServer: document.getElementById('delete-server'),
    dialog: document.getElementById('server-dialog'),
    dialogStatus: document.getElementById('dialog-status'),
    dialogTitle: document.getElementById('server-dialog-title'),
    editServer: document.getElementById('edit-server'),
    emptyAddServer: document.getElementById('empty-add-server'),
    emptyState: document.getElementById('empty-state'),
    filterButtons: [...document.querySelectorAll('.filter-button')],
    globalWarning: document.getElementById('global-warning'),
    healthSummary: document.getElementById('health-summary'),
    lastUpdated: document.getElementById('last-updated'),
    openConsole: document.getElementById('open-console'),
    password: document.getElementById('password'),
    refresh: document.getElementById('refresh'),
    saveServer: document.getElementById('save-server'),
    serverCount: document.getElementById('server-count'),
    serverDashboard: document.getElementById('server-dashboard'),
    serverForm: document.getElementById('server-form'),
    serverID: document.getElementById('server-id'),
    serverList: document.getElementById('server-list'),
    serverName: document.getElementById('server-name'),
    serverTemplate: document.getElementById('server-template'),
    sidebarAddServer: document.getElementById('sidebar-add-server'),
    statConsumers: document.getElementById('stat-consumers'),
    statDisabled: document.getElementById('stat-disabled'),
    statEnabled: document.getElementById('stat-enabled'),
    statTotal: document.getElementById('stat-total'),
    statusMessage: document.getElementById('status-message'),
    streamCount: document.getElementById('stream-count'),
    streamFilter: document.getElementById('stream-filter'),
    streamList: document.getElementById('stream-list'),
    streamTemplate: document.getElementById('stream-template'),
    username: document.getElementById('username'),
};

let servers = [];
let activeServerId = '';
let serverHealth = new Map();
let streamData = {};
let streamOrder = [];
let streamState = null;
let rtspConfig = {enabled: true, path: '/', port: '8554', username: '', password: ''};
let currentFilter = 'all';
let dashboardRequest = 0;
let refreshTimer = 0;
let healthTimer = 0;
let copyToastTimer = 0;

function hasOwn(object, key) {
    return Object.prototype.hasOwnProperty.call(object, key);
}

function createID() {
    if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
    return `server-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function normalizeBaseURL(value) {
    let input = value.trim();
    if (!input) throw new Error('请输入 go2rtc 服务地址');
    if (!/^[a-z][a-z\d+.-]*:\/\//i.test(input)) input = `http://${input}`;

    const url = new URL(input);
    if (!['http:', 'https:'].includes(url.protocol)) {
        throw new Error('服务地址仅支持 HTTP 或 HTTPS');
    }
    if (url.username || url.password) {
        throw new Error('请在独立的用户名和密码输入框中填写凭据');
    }

    const path = url.pathname.replace(/\/+$/, '');
    return `${url.origin}${path === '/' ? '' : path}`;
}

function permissionPattern(baseURL) {
    const url = new URL(baseURL);
    return `${url.protocol}//${url.hostname}/*`;
}

function endpoint(server, path) {
    return `${server.baseURL}/${path.replace(/^\/+/, '')}`;
}

function encodeBasicAuth(username, password) {
    const bytes = new TextEncoder().encode(`${username}:${password}`);
    let binary = '';
    for (const byte of bytes) binary += String.fromCharCode(byte);
    return `Basic ${btoa(binary)}`;
}

function requestHeaders(server, json = false) {
    const headers = new Headers();
    if (server.username) {
        headers.set('Authorization', encodeBasicAuth(server.username, server.password));
    }
    if (json) headers.set('Content-Type', 'application/json');
    return headers;
}

async function fetchAPI(server, path, init = {}, timeoutMS = 8000) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), timeoutMS);
    try {
        const response = await fetch(endpoint(server, path), {
            cache: 'no-store',
            ...init,
            headers: init.headers || requestHeaders(server, Boolean(init.body)),
            signal: controller.signal,
        });
        if (!response.ok) {
            const detail = (await response.text()).trim();
            if (response.status === 401) throw new Error('认证失败，请检查用户名和密码');
            throw new Error(detail || `请求失败（HTTP ${response.status}）`);
        }
        return response;
    } catch (error) {
        if (error.name === 'AbortError') throw new Error('连接超时，请确认 go2rtc 正在运行');
        throw error;
    } finally {
        clearTimeout(timeout);
    }
}

async function fetchJSON(server, path, init, timeoutMS) {
    return (await fetchAPI(server, path, init, timeoutMS)).json();
}

function getActiveServer() {
    return servers.find(server => server.id === activeServerId) || null;
}

async function persistState() {
    await extensionAPI.storage.local.set({servers, activeServerId});
}

async function hasServerPermission(server) {
    return extensionAPI.permissions.contains({origins: [permissionPattern(server.baseURL)]});
}

function serverDisplayName(baseURL) {
    const url = new URL(baseURL);
    return url.host || 'go2rtc';
}

function showStatus(message, error = false) {
    elements.statusMessage.hidden = !message;
    elements.statusMessage.textContent = message;
    elements.statusMessage.classList.toggle('is-error', error);
}

function showDialogStatus(message, error = false) {
    elements.dialogStatus.hidden = !message;
    elements.dialogStatus.textContent = message;
    elements.dialogStatus.classList.toggle('is-error', error);
}

function setActiveIndicator(state) {
    elements.activeStatusDot.className = 'status-dot';
    if (state) elements.activeStatusDot.classList.add(`is-${state}`);
}

function healthLabel(state) {
    if (state === 'online') return '在线';
    if (state === 'offline') return '离线';
    if (state === 'unauthorized') return '待授权';
    return '检测中';
}

function renderServerList() {
    elements.serverList.replaceChildren();
    elements.serverCount.textContent = `${servers.length} 个连接`;

    for (const server of servers) {
        const item = elements.serverTemplate.content.firstElementChild.cloneNode(true);
        const health = serverHealth.get(server.id) || {state: 'checking'};
        item.classList.add(`is-${health.state}`);
        item.classList.toggle('is-active', server.id === activeServerId);
        item.querySelector('.server-item-name').textContent = server.name;
        item.querySelector('.server-item-url').textContent = server.baseURL;
        item.querySelector('.server-item-state').textContent = healthLabel(health.state);
        item.title = `${server.name}\n${server.baseURL}`;
        item.addEventListener('click', () => selectServer(server.id));
        elements.serverList.appendChild(item);
    }

    const online = [...serverHealth.values()].filter(value => value.state === 'online').length;
    const pending = [...serverHealth.values()].filter(value => value.state === 'unauthorized').length;
    if (!servers.length) elements.healthSummary.textContent = '尚未配置服务端';
    else if (pending) elements.healthSummary.textContent = `${online} 个在线 · ${pending} 个待授权`;
    else elements.healthSummary.textContent = `${online} / ${servers.length} 个服务端在线`;
}

function resetDashboard() {
    streamData = {};
    streamOrder = [];
    streamState = null;
    rtspConfig = {enabled: true, path: '/', port: '8554', username: '', password: ''};
    elements.streamList.replaceChildren();
    elements.statTotal.textContent = '0';
    elements.statEnabled.textContent = '0';
    elements.statDisabled.textContent = '0';
    elements.statConsumers.textContent = '0';
    elements.streamCount.textContent = '0 路';
    elements.lastUpdated.textContent = '尚未刷新';
    elements.globalWarning.hidden = true;
}

function showSelectedServer() {
    const server = getActiveServer();
    const hasServer = Boolean(server);
    elements.emptyState.hidden = hasServer;
    elements.serverDashboard.hidden = !hasServer;
    resetDashboard();
    if (!server) {
        return;
    }

    elements.activeServerName.textContent = server.name;
    elements.activeServerName.title = server.name;
    elements.activeServerURL.textContent = server.baseURL;
    elements.activeServerURL.title = server.baseURL;
    elements.activeServerVersion.textContent = '连接中';
    elements.authorizeServer.hidden = true;
    setActiveIndicator('');
}

async function selectServer(serverID) {
    if (serverID === activeServerId && getActiveServer()) return;
    activeServerId = serverID;
    await persistState();
    renderServerList();
    showSelectedServer();
    showStatus('');
    await loadDashboard();
}

function streamStats(value) {
    return {
        producers: Array.isArray(value?.producers) ? value.producers.length : 0,
        consumers: Array.isArray(value?.consumers) ? value.consumers.length : 0,
    };
}

function mergeStreamData(runtimeStreams, simulateInfo) {
    const configuredStreams = simulateInfo?.configured_streams || {};
    const merged = {};

    for (const [name, sources] of Object.entries(configuredStreams)) {
        merged[name] = {
            configuredSources: Array.isArray(sources) ? sources : [],
            producers: [],
            consumers: [],
        };
    }
    for (const [name, runtime] of Object.entries(runtimeStreams || {})) {
        merged[name] = {
            ...runtime,
            configuredSources: merged[name]?.configuredSources || [],
        };
    }

    const configuredOrder = Array.isArray(simulateInfo?.configured_order)
        ? simulateInfo.configured_order.filter(name => hasOwn(merged, name))
        : [];
    const remaining = Object.keys(merged)
        .filter(name => !configuredOrder.includes(name))
        .sort((a, b) => a.localeCompare(b, 'zh-CN'));

    return {streams: merged, order: [...configuredOrder, ...remaining]};
}

function isStreamEnabled(name) {
    if (!streamState) return true;
    return streamState.streams_enabled !== false && !streamState.disabled_streams?.includes(name);
}

function redactRTSPCredentials(value) {
    try {
        const url = new URL(value);
        url.username = '';
        url.password = '';
        return `${url.protocol}//${url.host}${url.pathname}${url.search}`;
    } catch {
        return String(value).replace(/:\/\/[^/@\s]+@/, '://');
    }
}

function videoName(value) {
    const withoutOptions = String(value || '').split('#', 1)[0].trim();
    const withoutPrefix = withoutOptions.replace(/^(?:ffmpeg|file):/i, '');
    const normalized = withoutPrefix.replace(/\\/g, '/').replace(/\/+$/, '');
    const name = normalized.slice(normalized.lastIndexOf('/') + 1);
    if (!name) return '未识别视频';
    try {
        return decodeURIComponent(name);
    } catch {
        return name;
    }
}

function sourceLabel(value) {
    if (!value) return '未报告来源';
    const source = String(value).trim();
    const rtspMatch = source.match(/rtsp:\/\/[^\s#"']+/i);
    if (rtspMatch) return redactRTSPCredentials(rtspMatch[0]);
    return videoName(source);
}

function primarySource(stream) {
    const configuredSource = stream?.configuredSources?.[0];
    const runtimeSource = stream?.producers?.find(producer => producer?.url)?.url;
    return sourceLabel(configuredSource || runtimeSource);
}

function buildRTSPURL(server, name) {
    if (!rtspConfig.enabled || !rtspConfig.port) throw new Error('当前服务端未启用 RTSP 服务');
    const {hostname} = new URL(server.baseURL);
    const username = String(rtspConfig.username || '').trim();
    const password = String(rtspConfig.password || '');
    const credentials = username
        ? `${encodeURIComponent(username)}${password ? `:${encodeURIComponent(password)}` : ''}@`
        : '';
    const basePath = `/${String(rtspConfig.path || '/').replace(/^\/+|\/+$/g, '')}`.replace(/\/$/, '');
    return `rtsp://${credentials}${hostname}:${rtspConfig.port}${basePath}/${encodeURIComponent(name)}`;
}

async function copyText(value) {
    if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
        return;
    }
    const textarea = document.createElement('textarea');
    textarea.value = value;
    textarea.setAttribute('readonly', '');
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    const copied = document.execCommand('copy');
    textarea.remove();
    if (!copied) throw new Error('浏览器未允许写入剪贴板');
}

function showCopyToast(message, error = false) {
    clearTimeout(copyToastTimer);
    elements.copyToastMessage.textContent = message;
    elements.copyToast.classList.toggle('is-error', error);
    elements.copyToast.hidden = false;
    requestAnimationFrame(() => elements.copyToast.classList.add('is-visible'));
    copyToastTimer = setTimeout(() => {
        elements.copyToast.classList.remove('is-visible');
        setTimeout(() => {
            if (!elements.copyToast.classList.contains('is-visible')) elements.copyToast.hidden = true;
        }, 180);
    }, 2200);
}

async function copyRTSPURL(server, name, button) {
    try {
        await copyText(buildRTSPURL(server, name));
        button.classList.add('is-copied');
        button.setAttribute('aria-label', '已复制 RTSP 地址');
        button.title = '已复制';
        showCopyToast(`已复制“${name}”的 RTSP 地址`);
        setTimeout(() => {
            if (!button.isConnected) return;
            button.classList.remove('is-copied');
            button.setAttribute('aria-label', '复制 RTSP 地址');
            button.title = '复制 RTSP 地址';
        }, 1600);
    } catch (error) {
        showCopyToast(error.message || '复制 RTSP 地址失败', true);
    }
}

function renderStreams() {
    const query = elements.streamFilter.value.trim().toLocaleLowerCase();
    const names = streamOrder.length
        ? streamOrder.filter(name => hasOwn(streamData, name))
        : Object.keys(streamData).sort((a, b) => a.localeCompare(b, 'zh-CN'));
    const visibleNames = names.filter(name => {
        const matchesName = name.toLocaleLowerCase().includes(query);
        const enabled = isStreamEnabled(name);
        const matchesState = currentFilter === 'all'
            || (currentFilter === 'enabled' && enabled)
            || (currentFilter === 'disabled' && !enabled);
        return matchesName && matchesState;
    });

    const enabledCount = names.filter(isStreamEnabled).length;
    const consumers = names.reduce((total, name) => total + streamStats(streamData[name]).consumers, 0);
    elements.statTotal.textContent = String(names.length);
    elements.statEnabled.textContent = String(enabledCount);
    elements.statDisabled.textContent = String(names.length - enabledCount);
    elements.statConsumers.textContent = String(consumers);
    elements.streamCount.textContent = visibleNames.length === names.length
        ? `${names.length} 路`
        : `${visibleNames.length} / ${names.length} 路`;
    elements.streamList.replaceChildren();

    if (!visibleNames.length) {
        const message = document.createElement('div');
        message.className = 'list-message';
        message.textContent = names.length ? '没有匹配的视频流' : '当前服务端没有配置视频流';
        elements.streamList.appendChild(message);
        return;
    }

    const server = getActiveServer();
    for (const name of visibleNames) {
        const stream = streamData[name];
        const row = elements.streamTemplate.content.firstElementChild.cloneNode(true);
        const enabled = isStreamEnabled(name);
        const {producers, consumers: viewerCount} = streamStats(stream);
        const toggle = row.querySelector('.stream-toggle');

        row.classList.toggle('is-enabled', enabled);
        row.classList.toggle('is-disabled', !enabled);
        row.querySelector('.stream-name').textContent = name;
        row.querySelector('.stream-name').title = name;
        row.querySelector('.stream-detail').textContent = `${producers} 个活动源`;
        const source = primarySource(stream);
        row.querySelector('.stream-source').textContent = source;
        row.querySelector('.stream-source').title = source;
        row.querySelector('.stream-consumers').textContent = String(viewerCount);
        row.querySelector('.stream-status-label').textContent = enabled ? '已启用' : '已停用';
        toggle.checked = enabled;
        toggle.disabled = !streamState || streamState.streams_enabled === false;
        toggle.setAttribute('aria-label', `${enabled ? '停用' : '启用'} ${name}`);
        toggle.addEventListener('change', () => changeStreamState(name, toggle.checked, toggle));

        row.querySelector('.viewer-button').addEventListener('click', () => {
            const url = new URL(`${server.baseURL}/stream.html`);
            url.searchParams.set('src', name);
            url.searchParams.set('mode', 'webrtc,mse,hls,mjpeg');
            openTab(url.href);
        });
        const copyButton = row.querySelector('.copy-button');
        copyButton.disabled = !rtspConfig.enabled || !rtspConfig.port;
        if (copyButton.disabled) copyButton.title = 'RTSP 服务未启用';
        copyButton.addEventListener('click', () => copyRTSPURL(server, name, copyButton));
        elements.streamList.appendChild(row);
    }
}

function updateStateWarning() {
    if (!streamState || streamState.streams_enabled !== false) {
        elements.globalWarning.hidden = true;
        elements.globalWarning.textContent = '';
        return;
    }
    elements.globalWarning.hidden = false;
    elements.globalWarning.textContent = 'Web 控制台中的任务总开关当前处于关闭状态，请先在 Web 端开启后再调整单路任务。';
}

async function changeStreamState(name, enabled, input) {
    const server = getActiveServer();
    if (!server) return;
    input.disabled = true;
    try {
        streamState = await fetchJSON(server, `api/streams/state?src=${encodeURIComponent(name)}`, {
            method: 'PUT',
            body: JSON.stringify({enabled}),
            headers: requestHeaders(server, true),
        });
        renderStreams();
        showStatus(`任务“${name}”已${enabled ? '启用' : '停用'}`);
    } catch (error) {
        input.checked = !enabled;
        showStatus(error.message || '更新任务状态失败', true);
    } finally {
        input.disabled = false;
    }
}

async function loadDashboard(showLoading = true) {
    const server = getActiveServer();
    if (!server) return;
    const requestID = ++dashboardRequest;

    if (showLoading) {
        elements.refresh.disabled = true;
        elements.activeServerVersion.textContent = '连接中';
        setActiveIndicator('');
        showStatus('');
    }

    try {
        const permitted = await hasServerPermission(server);
        if (!permitted) {
            if (requestID !== dashboardRequest) return;
            resetDashboard();
            serverHealth.set(server.id, {state: 'unauthorized'});
            setActiveIndicator('unauthorized');
            elements.activeServerVersion.textContent = '待授权';
            elements.authorizeServer.hidden = false;
            elements.streamList.innerHTML = '<div class="list-message">授权该服务端后即可读取视频流。</div>';
            showStatus('尚未获得该主机的访问权限，请点击“授权访问”。', true);
            renderServerList();
            return;
        }

        const [info, streams, state, simulateInfo, onvifInfo] = await Promise.all([
            fetchJSON(server, 'api'),
            fetchJSON(server, 'api/streams'),
            fetchJSON(server, 'api/streams/state').catch(() => null),
            fetchJSON(server, 'api/simulate').catch(() => null),
            fetchJSON(server, 'api/simulate/onvif').catch(() => null),
        ]);
        if (requestID !== dashboardRequest) return;

        const merged = mergeStreamData(streams, simulateInfo);
        streamData = merged.streams;
        streamOrder = merged.order;
        streamState = state || (simulateInfo ? {
            streams_enabled: simulateInfo.streams_enabled !== false,
            disabled_streams: simulateInfo.disabled_streams || [],
        } : null);
        const effectiveRTSP = onvifInfo?.effective || onvifInfo?.config || {};
        rtspConfig = {
            enabled: simulateInfo?.rtsp_enabled !== false,
            path: simulateInfo?.rtsp_path || '/',
            port: String(effectiveRTSP.rtsp_port || simulateInfo?.rtsp_port || '8554'),
            username: effectiveRTSP.rtsp_username || '',
            password: effectiveRTSP.rtsp_password || '',
        };
        serverHealth.set(server.id, {state: 'online', version: info.version || ''});
        setActiveIndicator('online');
        elements.activeServerVersion.textContent = info.version ? `v${info.version}` : '在线';
        elements.authorizeServer.hidden = true;
        elements.lastUpdated.textContent = `更新于 ${new Date().toLocaleTimeString('zh-CN', {hour: '2-digit', minute: '2-digit', second: '2-digit'})}`;
        updateStateWarning();
        renderStreams();
        renderServerList();
    } catch (error) {
        if (requestID !== dashboardRequest) return;
        resetDashboard();
        serverHealth.set(server.id, {state: 'offline', error: error.message});
        setActiveIndicator('error');
        elements.activeServerVersion.textContent = '连接失败';
        elements.authorizeServer.hidden = true;
        elements.streamList.innerHTML = '<div class="list-message">无法读取视频流，请检查服务地址、认证信息和 go2rtc 运行状态。</div>';
        showStatus(error.message || '连接 go2rtc 失败', true);
        renderServerList();
    } finally {
        if (requestID === dashboardRequest) elements.refresh.disabled = false;
    }
}

async function checkServerHealth(server) {
    try {
        if (!await hasServerPermission(server)) return {state: 'unauthorized'};
        const info = await fetchJSON(server, 'api', undefined, 6000);
        return {state: 'online', version: info.version || ''};
    } catch (error) {
        return {state: 'offline', error: error.message};
    }
}

async function refreshAllHealth() {
    if (!servers.length) return;
    const results = await Promise.all(servers.map(async server => [server.id, await checkServerHealth(server)]));
    for (const [serverID, health] of results) serverHealth.set(serverID, health);
    renderServerList();

    const active = serverHealth.get(activeServerId);
    if (active?.state === 'offline') setActiveIndicator('error');
    if (active?.state === 'unauthorized') setActiveIndicator('unauthorized');
}

async function requestServerPermission(server, closeOnSuccess = false) {
    const origin = permissionPattern(server.baseURL);
    const granted = await extensionAPI.permissions.contains({origins: [origin]})
        || await extensionAPI.permissions.request({origins: [origin]});
    if (!granted) throw new Error('需要站点访问权限才能连接此 go2rtc 服务');

    serverHealth.set(server.id, {state: 'checking'});
    if (closeOnSuccess) closeServerDialog();
    renderServerList();
    await loadDashboard();
}

function openServerDialog(server = null) {
    showDialogStatus('');
    elements.serverID.value = server?.id || '';
    elements.serverName.value = server?.name || '';
    elements.baseURL.value = server?.baseURL || 'http://localhost:2984';
    elements.username.value = server?.username || '';
    elements.password.value = server?.password || '';
    elements.dialogTitle.textContent = server ? '编辑服务端' : '添加服务端';
    elements.deleteServer.hidden = !server;
    elements.saveServer.textContent = '保存并授权';
    elements.dialog.hidden = false;
    document.body.classList.add('dialog-open');
    requestAnimationFrame(() => elements.serverName.focus());
}

function closeServerDialog() {
    elements.dialog.hidden = true;
    document.body.classList.remove('dialog-open');
    elements.serverForm.reset();
    showDialogStatus('');
}

async function saveServer(event) {
    event.preventDefault();
    elements.saveServer.disabled = true;
    showDialogStatus('');
    let savedServer = null;

    try {
        const baseURL = normalizeBaseURL(elements.baseURL.value);
        const existingID = elements.serverID.value;
        const existing = servers.find(server => server.id === existingID);
        const duplicate = servers.find(server => server.baseURL === baseURL && server.id !== existingID);
        if (duplicate) throw new Error(`该地址已保存为“${duplicate.name}”`);

        const server = {
            id: existing?.id || createID(),
            name: elements.serverName.value.trim() || serverDisplayName(baseURL),
            baseURL,
            username: elements.username.value.trim(),
            password: elements.password.value,
            createdAt: existing?.createdAt || new Date().toISOString(),
        };
        savedServer = server;

        if (existing) servers = servers.map(item => item.id === server.id ? server : item);
        else servers = [...servers, server];
        activeServerId = server.id;
        await persistState();
        serverHealth.set(server.id, {state: 'unauthorized'});
        renderServerList();
        showSelectedServer();
        showDialogStatus('配置已保存，正在请求站点访问权限…');
        await requestServerPermission(server, true);
    } catch (error) {
        showDialogStatus(error.message || '保存服务端失败', true);
        if (savedServer) await loadDashboard(false);
    } finally {
        elements.saveServer.disabled = false;
    }
}

async function deleteCurrentServer() {
    const serverID = elements.serverID.value;
    const server = servers.find(item => item.id === serverID);
    if (!server || !globalThis.confirm(`确定删除服务端“${server.name}”吗？`)) return;

    servers = servers.filter(item => item.id !== serverID);
    serverHealth.delete(serverID);
    if (activeServerId === serverID) activeServerId = servers[0]?.id || '';
    await persistState();
    closeServerDialog();
    renderServerList();
    showSelectedServer();
    if (getActiveServer()) await loadDashboard();
}

async function openTab(url) {
    if (extensionAPI?.tabs?.create) {
        await extensionAPI.tabs.create({url});
        return;
    }
    globalThis.open(url, '_blank', 'noopener');
}

async function migrateLegacySettings(stored) {
    if (Array.isArray(stored.servers) && stored.servers.length) return stored;
    if (!stored.configured && !stored.pendingPermission) return stored;

    const baseURL = normalizeBaseURL(stored.baseURL || LEGACY_DEFAULTS.baseURL);
    const server = {
        id: createID(),
        name: serverDisplayName(baseURL),
        baseURL,
        username: stored.username || '',
        password: stored.password || '',
        createdAt: new Date().toISOString(),
    };
    const migrated = {servers: [server], activeServerId: server.id};
    await extensionAPI.storage.local.set(migrated);
    await extensionAPI.storage.local.remove(Object.keys(LEGACY_DEFAULTS));
    return {...stored, ...migrated};
}

function scheduleRefresh() {
    clearInterval(refreshTimer);
    clearInterval(healthTimer);
    refreshTimer = setInterval(() => {
        if (!document.hidden && getActiveServer() && elements.dialog.hidden) loadDashboard(false);
    }, 5000);
    healthTimer = setInterval(() => {
        if (!document.hidden) refreshAllHealth();
    }, 15000);
}

elements.addServer.addEventListener('click', () => openServerDialog());
elements.sidebarAddServer.addEventListener('click', () => openServerDialog());
elements.emptyAddServer.addEventListener('click', () => openServerDialog());
elements.closeDialog.addEventListener('click', closeServerDialog);
elements.cancelDialog.addEventListener('click', closeServerDialog);
elements.serverForm.addEventListener('submit', saveServer);
elements.deleteServer.addEventListener('click', deleteCurrentServer);
elements.editServer.addEventListener('click', () => openServerDialog(getActiveServer()));
elements.refresh.addEventListener('click', () => loadDashboard());
elements.authorizeServer.addEventListener('click', async () => {
    const server = getActiveServer();
    if (!server) return;
    elements.authorizeServer.disabled = true;
    showStatus('正在请求站点访问权限…');
    try {
        await requestServerPermission(server);
    } catch (error) {
        showStatus(error.message || '授权失败', true);
    } finally {
        elements.authorizeServer.disabled = false;
    }
});
elements.openConsole.addEventListener('click', () => {
    const server = getActiveServer();
    if (server) openTab(`${server.baseURL}/simulate.html`);
});
elements.streamFilter.addEventListener('input', renderStreams);
for (const button of elements.filterButtons) {
    button.addEventListener('click', () => {
        currentFilter = button.dataset.filter;
        for (const item of elements.filterButtons) item.classList.toggle('is-active', item === button);
        renderStreams();
    });
}
elements.dialog.addEventListener('click', event => {
    if (event.target === elements.dialog) closeServerDialog();
});
document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && !elements.dialog.hidden) closeServerDialog();
});
document.addEventListener('visibilitychange', () => {
    if (!document.hidden && getActiveServer()) {
        loadDashboard(false);
        refreshAllHealth();
    }
});

async function initialize() {
    if (!extensionAPI?.storage?.local || !extensionAPI?.permissions) {
        elements.emptyState.querySelector('h1').textContent = '请在浏览器扩展环境中打开此页面';
        elements.emptyState.querySelector('p').textContent = '重新加载已解压的扩展后，点击工具栏图标即可进入多服务端控制台。';
        return;
    }

    let stored = await extensionAPI.storage.local.get({...STORAGE_DEFAULTS, ...LEGACY_DEFAULTS});
    stored = await migrateLegacySettings(stored);
    servers = Array.isArray(stored.servers) ? stored.servers : [];
    activeServerId = servers.some(server => server.id === stored.activeServerId)
        ? stored.activeServerId
        : servers[0]?.id || '';
    if (activeServerId !== stored.activeServerId) await persistState();

    renderServerList();
    showSelectedServer();
    scheduleRefresh();
    if (getActiveServer()) {
        await loadDashboard();
        refreshAllHealth();
    }
}

initialize().catch(error => {
    showStatus(error.message || '扩展初始化失败', true);
    setActiveIndicator('error');
});
