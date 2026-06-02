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

async function loadOverview() {
    const [reg, projects, history] = await Promise.all([
        fetchJSON('/api/registry'),
        fetchJSON('/api/projects'),
        fetchJSON('/api/history'),
    ]);

    const skillCount = Object.values(reg.skills || {}).flat().length;
    const mcpCount = (reg.mcp || []).length;
    const projectCount = (projects || []).length;

    document.getElementById('overview').innerHTML = `
        <h2>Overview</h2>
        <div class="stats">
            <div class="stat-card"><h3>Skills</h3><div class="value">${skillCount}</div></div>
            <div class="stat-card"><h3>MCP Servers</h3><div class="value">${mcpCount}</div></div>
            <div class="stat-card"><h3>Projects</h3><div class="value">${projectCount}</div></div>
            <div class="stat-card"><h3>Total Installs</h3><div class="value">${(history || []).length}</div></div>
        </div>
        <h3>Recent Installs</h3>
        ${renderHistoryTable((history || []).slice(0, 10))}
    `;
}

async function loadRegistry() {
    const reg = await fetchJSON('/api/registry');
    let html = '<h2>Registry</h2>';

    const skills = reg.skills || {};
    const specialDirs = ['global', 'codex-only', 'claude-only'];

    for (const [cat, names] of Object.entries(skills)) {
        const isSpecial = specialDirs.includes(cat);
        html += `<div class="category-group">
            <h3>${cat} ${isSpecial ? '<span class="badge special">special</span>' : ''}</h3>
            <table><tr><th>Skill Name</th></tr>`;
        names.forEach(n => html += `<tr><td>${n}</td></tr>`);
        html += '</table></div>';
    }

    html += '<h3>MCP Servers</h3><table><tr><th>Name</th></tr>';
    (reg.mcp || []).forEach(n => html += `<tr><td>${n}</td></tr>`);
    html += '</table>';

    document.getElementById('registry').innerHTML = html;
}

async function loadProjects() {
    const projects = await fetchJSON('/api/projects');
    let html = '<h2>Projects</h2>';

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
        ${renderHistoryTable(history || [])}
    `;
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