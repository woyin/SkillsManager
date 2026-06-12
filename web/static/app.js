document.addEventListener('DOMContentLoaded', () => {
    const nav = document.querySelectorAll('.sidebar a');
    const views = document.querySelectorAll('.view');

    nav.forEach(link => {
        link.addEventListener('click', e => {
            e.preventDefault();
            const viewId = link.dataset.view;

            nav.forEach(l => l.classList.remove('active'));
            link.classList.add('active');

            views.forEach(v => v.classList.remove('active'));
            document.getElementById(viewId).classList.add('active');

            loadView(viewId);
        });
    });

    loadView('overview');
});

async function fetchJSON(url) {
    const res = await fetch(url);
    return res.json();
}

async function loadView(view) {
    switch(view) {
        case 'overview': return loadOverview();
        case 'registry': return loadRegistry();
        case 'projects': return loadProjects();
        case 'history': return loadHistory();
    }
}

function formatTokens(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
    return String(n);
}

async function loadOverview() {
    const [reg, projects, history, check, aivo] = await Promise.all([
        fetchJSON('/api/registry'),
        fetchJSON('/api/projects'),
        fetchJSON('/api/history'),
        fetchJSON('/api/check'),
        fetchJSON('/api/aivo'),
    ]);

    const skillCount = Object.values(reg.skills || {}).flat().length;
    const mcpCount = (reg.mcp || []).length;
    const projectCount = (projects || []).length;
    const issueCount = (check.issues || []).length;

    let aivoSection = '';
    if (aivo.installed) {
        const tokenDisplay = aivo.total_tokens ? formatTokens(aivo.total_tokens) : '0';
        aivoSection = `
            <h3>aivo</h3>
            <div class="stats">
                <div class="stat-card"><h3>Active Key</h3><div class="value small">${aivo.active_key || 'none'}</div></div>
                <div class="stat-card"><h3>API Keys</h3><div class="value">${aivo.keys_count || 0}</div></div>
                <div class="stat-card"><h3>Tokens Used</h3><div class="value">${tokenDisplay}</div></div>
                <div class="stat-card"><h3>Sessions</h3><div class="value">${aivo.sessions || 0}</div></div>
            </div>
            ${aivo.unhealthy_keys > 0 ? '<p class="warning-text">⚠ ' + aivo.unhealthy_keys + ' unhealthy key(s)</p>' : ''}
        `;
    }

    document.getElementById('overview').innerHTML = `
        <h2>Overview</h2>
        <div class="stats">
            <div class="stat-card"><h3>Skills</h3><div class="value">${skillCount}</div></div>
            <div class="stat-card"><h3>MCP Servers</h3><div class="value">${mcpCount}</div></div>
            <div class="stat-card"><h3>Projects</h3><div class="value">${projectCount}</div></div>
            <div class="stat-card ${issueCount ? 'warning' : ''}"><h3>Health</h3><div class="value">${issueCount ? issueCount : 'OK'}</div></div>
        </div>
        ${renderIssueList(check.issues || [])}
        ${aivoSection}
        <h3>Recent Installs</h3>
        ${renderHistoryTable((history || []).slice(0, 10))}
    `;
}

async function loadRegistry() {
    const reg = await fetchJSON('/api/registry');
    let html = '<h2>Registry</h2>';

    const skills = reg.skill_details || {};
    const specialDirs = ['global', 'codex-only', 'claude-only', 'gemini-only', 'opencode-only', 'hermes-only', 'openclaw-only'];

    for (const [cat, items] of Object.entries(skills)) {
        const isSpecial = specialDirs.includes(cat);
        html += `<div class="category-group">
            <h3>${cat} ${isSpecial ? '<span class="badge special">special</span>' : ''}</h3>
            <table><tr><th>Skill Name</th><th>Source</th><th>Last Updated</th></tr>`;
        items.forEach(item => {
            html += `<tr>
                <td>${item.name}</td>
                <td>${item.source_url || '-'}</td>
                <td>${formatDate(item.last_updated)}</td>
            </tr>`;
        });
        html += '</table></div>';
    }

    html += '<h3>MCP Servers</h3><table><tr><th>Name</th><th>Source</th><th>Last Updated</th></tr>';
    (reg.mcp_details || []).forEach(item => {
        html += `<tr>
            <td>${item.name}</td>
            <td>${item.source_url || '-'}</td>
            <td>${formatDate(item.last_updated)}</td>
        </tr>`;
    });
    html += '</table>';

    document.getElementById('registry').innerHTML = html;
}

async function loadProjects() {
    const [projects, check] = await Promise.all([
        fetchJSON('/api/projects'),
        fetchJSON('/api/check'),
    ]);
    let html = '<h2>Projects</h2>';
    html += renderIssueList((check.issues || []).filter(issue => issue.type === 'missing_project'));

    if (!projects || projects.length === 0) {
        html += '<p>No projects installed yet.</p>';
    } else {
        html += '<table><tr><th>Path</th><th>Profile</th><th>Extra Skills</th><th>Extra MCP</th><th>Last Installed</th></tr>';
        projects.forEach(p => {
            html += `<tr>
                <td>${p.path}</td>
                <td>${p.profile || '-'}</td>
                <td>${(p.extra_skills || []).join(', ') || '-'}</td>
                <td>${(p.extra_mcp || []).join(', ') || '-'}</td>
                <td>${p.last_installed || '-'}</td>
            </tr>`;
        });
        html += '</table>';
    }

    document.getElementById('projects').innerHTML = html;
}

async function loadHistory() {
    const history = await fetchJSON('/api/history');
    document.getElementById('history').innerHTML = `
        <h2>Install History</h2>
        <div class="toolbar">
            <input id="history-filter" data-history-control type="search" placeholder="Filter history">
            <select id="history-sort" data-history-control>
                <option value="newest">Newest first</option>
                <option value="oldest">Oldest first</option>
                <option value="project">Project A-Z</option>
            </select>
        </div>
        <div id="history-table"></div>
    `;
    const render = () => {
        document.getElementById('history-table').innerHTML = renderHistoryTable(applyHistoryControls(history || []));
    };
    document.querySelectorAll('[data-history-control]').forEach(control => {
        control.addEventListener('input', render);
        control.addEventListener('change', render);
    });
    render();
}

function renderHistoryTable(items) {
    if (!items || items.length === 0) return '<p>No installations recorded.</p>';
    let html = '<table><tr><th>Time</th><th>Project</th><th>Profile</th><th>Skills</th><th>MCP</th></tr>';
    items.forEach(i => {
        html += `<tr>
            <td>${i.installed_at}</td>
            <td>${i.project_path}</td>
            <td>${i.profile || '-'}</td>
            <td>${(i.skills || []).join(', ') || '-'}</td>
            <td>${(i.mcp || []).join(', ') || '-'}</td>
        </tr>`;
    });
    html += '</table>';
    return html;
}

function applyHistoryControls(items) {
    const filter = (document.getElementById('history-filter')?.value || '').toLowerCase();
    const sortMode = document.getElementById('history-sort')?.value || 'newest';
    let filtered = items;

    if (filter) {
        filtered = filtered.filter(item => [
            item.project_path,
            item.profile,
            ...(item.skills || []),
            ...(item.mcp || []),
        ].join(' ').toLowerCase().includes(filter));
    }

    return [...filtered].sort((a, b) => {
        if (sortMode === 'oldest') return new Date(a.installed_at) - new Date(b.installed_at);
        if (sortMode === 'project') return (a.project_path || '').localeCompare(b.project_path || '');
        return new Date(b.installed_at) - new Date(a.installed_at);
    });
}

function renderIssueList(issues) {
    if (!issues || issues.length === 0) return '';
    let html = '<div class="issues"><h3>Health Issues</h3><table><tr><th>Type</th><th>Path</th></tr>';
    issues.forEach(issue => {
        html += `<tr><td>${issue.type}</td><td>${issue.path}</td></tr>`;
    });
    html += '</table></div>';
    return html;
}

function formatDate(value) {
    if (!value) return '-';
    return value.replace('T', ' ').replace('Z', ' UTC');
}
