const currentPage = location.pathname.split('/').pop() || 'index.html';

document.head.insertAdjacentHTML('beforeend', `
<style>
    :root {
        color-scheme: dark;
        --bg: #0b1120;
        --bg-soft: rgba(15, 23, 42, 0.72);
        --surface: rgba(17, 24, 39, 0.88);
        --surface-strong: rgba(18, 28, 48, 0.96);
        --line: #22324d;
        --line-strong: #2b3f63;
        --text: #e2e8f0;
        --text-muted: #94a3b8;
        --text-faint: #64748b;
        --accent: #22d3ee;
        --accent-strong: #67e8f9;
        --ok: #34d399;
        --warn: #fbbf24;
        --danger: #fb7185;
        --shadow: 0 20px 50px rgba(2, 6, 23, 0.34);
        --shadow-soft: 0 12px 32px rgba(2, 6, 23, 0.22);
        --radius: 16px;
    }

    *,
    *::before,
    *::after {
        box-sizing: border-box;
    }

    :where(html) {
        min-height: 100%;
        background: var(--bg);
    }

    :where(body) {
        min-height: 100vh;
        margin: 0;
        display: flex;
        flex-direction: column;
        background:
            radial-gradient(1100px 600px at 12% -10%, rgba(34, 211, 238, 0.10), transparent 60%),
            radial-gradient(900px 500px at 100% 0%, rgba(14, 116, 144, 0.10), transparent 55%),
            var(--bg);
        color: var(--text);
        font-family: "Noto Sans SC", "Segoe UI", system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
        -webkit-font-smoothing: antialiased;
        -moz-osx-font-smoothing: grayscale;
    }

    :where(a) {
        color: var(--accent-strong);
        text-decoration: none;
    }

    :where(a:hover) {
        color: #a5f3fc;
    }

    .app-header {
        position: sticky;
        top: 0;
        z-index: 30;
        border-bottom: 1px solid rgba(34, 50, 77, 0.8);
        background: rgba(11, 17, 32, 0.86);
        backdrop-filter: blur(14px);
        box-shadow: 0 12px 30px rgba(2, 6, 23, 0.18);
    }

    .app-nav {
        width: min(1440px, 100%);
        margin: 0 auto;
        padding: 12px 18px;
        display: flex;
        align-items: center;
        gap: 14px;
        overflow-x: auto;
        scrollbar-width: none;
    }

    .app-nav::-webkit-scrollbar {
        display: none;
    }

    .app-brand {
        flex: 0 0 auto;
        display: inline-flex;
        align-items: center;
        gap: 10px;
        min-height: 38px;
        padding: 0 12px 0 4px;
        color: var(--text);
        font-weight: 800;
        letter-spacing: 0;
    }

    .app-brand-mark {
        width: 34px;
        height: 34px;
        border-radius: 12px;
        display: inline-grid;
        place-items: center;
        background: linear-gradient(135deg, #22d3ee 0%, #06b6d4 100%);
        color: #04253a;
        box-shadow: 0 12px 26px rgba(34, 211, 238, 0.18);
        font-size: 13px;
        font-weight: 900;
    }

    .app-nav-links {
        display: flex;
        align-items: center;
        gap: 6px;
        min-width: 0;
    }

    .app-nav a {
        flex: 0 0 auto;
        min-height: 36px;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        border: 1px solid transparent;
        border-radius: 12px;
        padding: 0 12px;
        color: var(--text-muted);
        font-size: 14px;
        font-weight: 650;
        transition: background .16s ease, color .16s ease, border-color .16s ease, transform .16s ease;
    }

    .app-nav a:hover,
    .app-nav a.is-active {
        background: rgba(34, 211, 238, 0.10);
        border-color: rgba(34, 211, 238, 0.20);
        color: var(--accent-strong);
    }

    .app-nav a:active {
        transform: scale(0.98);
    }

    :where(main) {
        width: min(1440px, 100%);
        margin: 0 auto;
        padding: 18px;
        display: flex;
        flex-direction: column;
        gap: 16px;
        background: transparent;
    }

    :where(button) {
        min-height: 40px;
        border: 1px solid var(--line-strong);
        border-radius: 14px;
        background: rgba(15, 23, 42, 0.78);
        color: var(--text);
        padding: 9px 16px;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: 8px;
        font: inherit;
        font-size: 14px;
        font-weight: 700;
        line-height: 1.2;
        cursor: pointer;
        transition: transform .15s ease, box-shadow .15s ease, border-color .15s ease, background .15s ease, color .15s ease;
    }

    :where(button:hover) {
        transform: translateY(-1px);
        border-color: rgba(34, 211, 238, 0.38);
        background: rgba(15, 23, 42, 0.96);
        box-shadow: var(--shadow-soft);
    }

    :where(button:disabled) {
        cursor: not-allowed;
        opacity: 0.48;
        transform: none;
        box-shadow: none;
    }

    :where(input[type="text"], input[type="email"], input[type="password"], input[type="number"], textarea, select) {
        width: 100%;
        border: 1px solid var(--line-strong);
        border-radius: 14px;
        background: rgba(15, 23, 42, 0.72);
        color: var(--text);
        padding: 11px 13px;
        font: inherit;
        font-size: 14px;
        outline: none;
        transition: border-color .18s ease, box-shadow .18s ease, background .18s ease;
    }

    :where(input[type="text"], input[type="email"], input[type="password"], input[type="number"], textarea, select):focus {
        border-color: rgba(34, 211, 238, 0.55);
        background: rgba(15, 23, 42, 0.95);
        box-shadow: 0 0 0 4px rgba(34, 211, 238, 0.12);
    }

    :where(input::placeholder, textarea::placeholder) {
        color: var(--text-faint);
    }

    :where(label) {
        display: inline-flex;
        align-items: center;
        gap: 8px;
        color: var(--text-muted);
        cursor: pointer;
    }

    :where(input[type="checkbox"]) {
        width: 18px;
        height: 18px;
        accent-color: var(--accent);
        cursor: pointer;
    }

    :where(form) {
        display: flex;
        flex-wrap: wrap;
        gap: 10px;
        align-items: center;
    }

    :where(table) {
        width: 100%;
        border-collapse: separate;
        border-spacing: 0;
        border: 1px solid var(--line);
        border-radius: var(--radius);
        overflow: hidden;
        background: var(--surface);
        box-shadow: var(--shadow);
    }

    :where(th, td) {
        padding: 13px 15px;
        text-align: left;
        border-bottom: 1px solid rgba(34, 50, 77, 0.72);
        vertical-align: top;
    }

    :where(th) {
        background: rgba(18, 28, 48, 0.96);
        color: var(--text-muted);
        font-size: 12px;
        font-weight: 800;
        text-transform: uppercase;
    }

    :where(td) {
        color: var(--text);
        font-size: 14px;
        line-height: 1.55;
    }

    :where(tr:last-child td) {
        border-bottom: 0;
    }

    :where(tbody tr:hover) {
        background: rgba(34, 211, 238, 0.06);
    }

    :where(code, pre) {
        font-family: Consolas, "JetBrains Mono", Monaco, monospace;
    }

    :where(pre) {
        border: 1px solid var(--line);
        border-radius: var(--radius);
        background: rgba(2, 6, 23, 0.58);
        color: #dbeafe;
        padding: 16px;
        overflow: auto;
        line-height: 1.6;
    }

    :where(.controls, main > div:first-child) {
        border: 1px solid var(--line);
        border-radius: 18px;
        background: rgba(17, 24, 39, 0.72);
        padding: 14px;
        box-shadow: var(--shadow-soft);
    }

    :where(.info) {
        color: var(--text-muted);
        font-size: 13px;
        line-height: 1.6;
    }

    :where(main > button) {
        width: 100%;
        justify-content: space-between;
        border-color: var(--line);
        background: var(--surface-strong);
        color: var(--text);
        padding: 14px 16px;
        box-shadow: var(--shadow-soft);
    }

    :where(main > button)::after {
        content: "打开";
        border: 1px solid rgba(34, 211, 238, 0.22);
        border-radius: 999px;
        background: rgba(34, 211, 238, 0.10);
        color: var(--accent-strong);
        padding: 5px 9px;
        font-size: 12px;
        font-weight: 800;
    }

    :where(main > button + div) {
        border: 1px solid var(--line);
        border-radius: 18px;
        background: rgba(17, 24, 39, 0.72);
        padding: 14px;
        overflow-x: auto;
        box-shadow: var(--shadow-soft);
    }

    :where(#config) {
        border: 1px solid var(--line);
        border-radius: 18px 18px 0 0;
        overflow: hidden;
        background: #0f172a;
        box-shadow: var(--shadow);
    }

    :where(.trace) {
        color: var(--text-faint) !important;
    }

    :where(.debug) {
        color: var(--text-muted) !important;
    }

    :where(.info) {
        color: var(--accent-strong) !important;
    }

    :where(.warn) {
        color: var(--warn) !important;
    }

    :where(.error) {
        color: var(--danger) !important;
    }

    ::selection {
        background: rgba(34, 211, 238, 0.28);
        color: white;
    }

    ::-webkit-scrollbar {
        width: 10px;
        height: 10px;
    }

    ::-webkit-scrollbar-track {
        background: transparent;
    }

    ::-webkit-scrollbar-thumb {
        background: #22324d;
        border: 2px solid transparent;
        border-radius: 8px;
        background-clip: padding-box;
    }

    ::-webkit-scrollbar-thumb:hover {
        background: #2b3f63;
        background-clip: padding-box;
    }

    @media (max-width: 720px) {
        .app-nav {
            padding: 10px 12px;
            gap: 8px;
        }

        .app-brand {
            padding-right: 4px;
        }

        .app-brand span:last-child {
            display: none;
        }

        .app-nav a {
            min-height: 34px;
            padding: 0 10px;
            font-size: 13px;
        }

        :where(main) {
            padding: 12px;
            gap: 12px;
        }

        :where(form) {
            flex-direction: column;
            align-items: stretch;
        }

        :where(table) {
            display: block;
            overflow-x: auto;
        }

        :where(th, td) {
            padding: 11px 12px;
        }
    }
</style>
`);

const navItems = [
    ['index.html', 'Streams'],
    ['simulate.html', '模拟流'],
    ['add.html', 'Add'],
    ['config.html', 'Config'],
    ['log.html', '日志'],
    ['net.html', 'Network'],
];

const links = navItems.map(([href, label]) => {
    const active = currentPage === href || (currentPage === '' && href === 'index.html');
    return `<a href="${href}"${active ? ' class="is-active"' : ''}>${label}</a>`;
}).join('');

document.body.insertAdjacentHTML('afterbegin', `
<header class="app-header">
    <nav class="app-nav" aria-label="go2rtc navigation">
        <a class="app-brand" href="index.html" aria-label="go2rtc streams">
            <span class="app-brand-mark">g2</span>
            <span>go2rtc</span>
        </a>
        <div class="app-nav-links">${links}</div>
    </nav>
</header>
`);
