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
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
        throw new Error(body.error || `HTTP ${res.status}`);
    }
    return body;
}

function escapeHTML(value) {
    return String(value ?? '')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

async function loadView(view) {
    try {
        switch (view) {
            case 'overview': return await loadOverview();
            case 'registry': return await loadRegistry();
            case 'profiles': return await loadProfiles();
            case 'projects': return await loadProjects();
            case 'history': return await loadHistory();
        }
    } catch (err) {
        const container = document.getElementById(view);
        if (container) {
            container.innerHTML = `<div class="error-banner">Failed to load view: ${escapeHTML(err.message)}</div>`;
        }
    }
}

function formatTokens(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
    return String(n);
}

async function loadOverview() {
    const [reg, profiles, projects, history, check, aivo] = await Promise.all([
        fetchJSON('/api/registry'),
        fetchJSON('/api/profiles'),
        fetchJSON('/api/projects'),
        fetchJSON('/api/history'),
        fetchJSON('/api/check'),
        fetchJSON('/api/aivo'),
    ]);

    const skillCount = Object.values(reg.skills || {}).flat().length;
    const mcpCount = (reg.mcp || []).length;
    const profileCount = (profiles || []).length;
    const projectCount = (projects || []).length;
    const issueCount = (check.issues || []).length;

    let aivoSection = '';
    if (aivo.installed) {
        const tokenDisplay = aivo.total_tokens ? formatTokens(aivo.total_tokens) : '0';
        aivoSection = `
            <h3>aivo</h3>
            <div class="stats">
                <div class="stat-card"><h3>Active Key</h3><div class="value small">${escapeHTML(aivo.active_key || 'none')}</div></div>
                <div class="stat-card"><h3>API Keys</h3><div class="value">${escapeHTML(aivo.keys_count || 0)}</div></div>
                <div class="stat-card"><h3>Tokens Used</h3><div class="value">${escapeHTML(tokenDisplay)}</div></div>
                <div class="stat-card"><h3>Sessions</h3><div class="value">${escapeHTML(aivo.sessions || 0)}</div></div>
            </div>
            ${aivo.unhealthy_keys > 0 ? '<p class="warning-text">⚠ ' + escapeHTML(aivo.unhealthy_keys) + ' unhealthy key(s)</p>' : ''}
        `;
    }

    document.getElementById('overview').innerHTML = `
        <h2>Overview</h2>
        <div class="stats">
            <div class="stat-card"><h3>Skills</h3><div class="value">${skillCount}</div></div>
            <div class="stat-card"><h3>MCP Servers</h3><div class="value">${mcpCount}</div></div>
            <div class="stat-card"><h3>Profiles</h3><div class="value">${profileCount}</div></div>
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
    const container = document.getElementById('registry');
    container.innerHTML = `
        <h2>Registry</h2>
        <div class="toolbar">
            <input id="registry-filter" type="search" placeholder="Filter skills by name, category, origin, or lint status">
        </div>
        <div id="skill-detail"></div>
        <div id="registry-content"></div>
    `;

    const renderList = () => {
        const query = (document.getElementById('registry-filter')?.value || '').trim().toLowerCase();
        document.getElementById('registry-content').innerHTML = renderRegistryContent(reg, query);
    };

    document.getElementById('registry-filter').addEventListener('input', renderList);
    container.onclick = e => {
        const btn = e.target.closest('[data-skill-name]');
        if (btn) {
            loadSkillDetail(btn.dataset.skillCategory || '', btn.dataset.skillName);
            return;
        }
        if (e.target.closest('[data-close-detail]')) {
            document.getElementById('skill-detail').innerHTML = '';
        }
    };

    renderList();
}

function renderRegistryContent(reg, query) {
    const skills = reg.skill_details || {};
    const qualityMap = reg.skill_quality || {};
    const specialDirs = ['global', 'codex-only', 'claude-only', 'gemini-only', 'opencode-only', 'hermes-only', 'openclaw-only'];
    let html = '';

    for (const [cat, items] of Object.entries(skills)) {
        const filtered = (items || []).filter(item => {
            if (!query) return true;
            const q = qualityMap[`${cat}/${item.name}`] || {};
            const lintLabel = q.errors > 0 ? 'error' : q.warnings > 0 ? 'warning' : 'ok';
            return [cat, item.name, item.source_url, q.update_class, lintLabel]
                .join(' ')
                .toLowerCase()
                .includes(query);
        });
        if (filtered.length === 0) continue;

        const isSpecial = specialDirs.includes(cat);
        html += `<div class="category-group">
            <h3>${escapeHTML(cat)} ${isSpecial ? '<span class="badge special">special</span>' : ''}</h3>
            <table><tr><th>Skill Name</th><th>Score</th><th>Lint</th><th>Origin</th><th>Source</th><th>Last Updated</th></tr>`;
        filtered.forEach(item => {
            const q = qualityMap[`${cat}/${item.name}`];
            html += `<tr>
                <td><button type="button" class="link-btn" data-skill-category="${escapeHTML(cat)}" data-skill-name="${escapeHTML(item.name)}">${escapeHTML(item.name)}</button></td>
                <td>${renderScoreBadge(q ? q.score : null)}</td>
                <td>${renderLintSummary(q)}</td>
                <td>${renderOriginBadge(q ? q.update_class : '')}</td>
                <td>${escapeHTML(item.source_url || '-')}</td>
                <td>${escapeHTML(formatDate(item.last_updated))}</td>
            </tr>`;
        });
        html += '</table></div>';
    }

    if (!html) {
        html = '<p>No matching skills in registry.</p>';
    }

    html += '<h3>MCP Servers</h3><table><tr><th>Name</th><th>Source</th><th>Last Updated</th></tr>';
    (reg.mcp_details || []).forEach(item => {
        html += `<tr>
            <td>${escapeHTML(item.name)}</td>
            <td>${escapeHTML(item.source_url || '-')}</td>
            <td>${escapeHTML(formatDate(item.last_updated))}</td>
        </tr>`;
    });
    html += '</table>';
    return html;
}

async function loadSkillDetail(category, name) {
    const panel = document.getElementById('skill-detail');
    panel.innerHTML = '<div class="detail-panel"><p>Loading skill details...</p></div>';
    try {
        const params = new URLSearchParams({ name });
        if (category) params.set('category', category);
        const detail = await fetchJSON('/api/skill?' + params.toString());
        panel.innerHTML = renderSkillDetail(detail);
    } catch (err) {
        panel.innerHTML = `<div class="error-banner">Failed to load skill "${escapeHTML(name)}": ${escapeHTML(err.message)}</div>`;
    }
}

function renderSkillDetail(detail) {
    const score = detail.score || { total: 0, breakdown: {}, notes: [] };
    const breakdownEntries = Object.entries(score.breakdown || {})
        .map(([k, v]) => `<span class="badge">${escapeHTML(k)}: ${escapeHTML(v)}</span>`)
        .join(' ');
    const findings = (detail.lint && detail.lint.findings) || [];
    const findingsHTML = findings.length === 0
        ? '<p class="muted">No lint findings.</p>'
        : '<ul class="finding-list">' + findings.map(f =>
            `<li class="finding-${escapeHTML(f.level)}"><span class="badge badge-${escapeHTML(f.level)}">${escapeHTML(f.level)}</span> ${escapeHTML(f.message)}</li>`
        ).join('') + '</ul>';

    const origin = detail.origin || {};
    const originMeta = [
        origin.update_class ? `Class: ${origin.update_class}` : '',
        origin.source_kind ? `Kind: ${origin.source_kind}` : '',
        origin.source ? `Source: ${origin.source}` : '',
        origin.ref ? `Ref: ${origin.ref}${origin.ref_kind ? ' (' + origin.ref_kind + ')' : ''}` : '',
        origin.commit ? `Commit: ${origin.commit.slice(0, 12)}` : '',
    ].filter(Boolean).map(line => `<li>${escapeHTML(line)}</li>`).join('');

    const contentBlock = detail.content
        ? `<pre class="markdown-preview">${escapeHTML(detail.content)}</pre>${detail.content_truncated ? '<p class="muted">Content truncated at 256 KiB.</p>' : ''}`
        : '<p class="muted">SKILL.md not found.</p>';

    return `
        <div class="detail-panel">
            <div class="panel-header">
                <h3>${escapeHTML(detail.category)}/${escapeHTML(detail.name)}</h3>
                <button type="button" class="btn-secondary" data-close-detail>Close</button>
            </div>
            <div class="detail-grid">
                <div>
                    <h4>Score</h4>
                    <p>${renderScoreBadge(score.total)} ${breakdownEntries}</p>
                    ${(score.notes || []).length ? `<p class="muted">${escapeHTML(score.notes.join(' · '))}</p>` : ''}
                </div>
                <div>
                    <h4>Origin</h4>
                    ${originMeta ? `<ul class="meta-list">${originMeta}</ul>` : '<p class="muted">No provenance recorded.</p>'}
                </div>
            </div>
            <h4>Lint Findings</h4>
            ${findingsHTML}
            <h4>SKILL.md</h4>
            ${contentBlock}
        </div>
    `;
}

async function loadProfiles() {
    const profiles = await fetchJSON('/api/profiles');
    let html = '<h2>Profiles</h2>';
    if (!profiles || profiles.length === 0) {
        html += '<p>No profiles configured.</p>';
    } else {
        html += '<table><tr><th>Profile</th><th>Status</th><th>Skills</th><th>MCP</th><th>Issues</th></tr>';
        profiles.forEach(p => {
            const statusBadge = p.valid
                ? '<span class="badge badge-ok">valid</span>'
                : '<span class="badge badge-error">invalid</span>';
            const issuesText = (p.issues || []).length > 0
                ? `<ul class="finding-list">${p.issues.map(i => `<li>${escapeHTML(i)}</li>`).join('')}</ul>`
                : '-';
            html += `<tr>
                <td><strong>${escapeHTML(p.name)}</strong></td>
                <td>${statusBadge}</td>
                <td>${escapeHTML((p.skills || []).join(', ') || '-')}</td>
                <td>${escapeHTML((p.mcp || []).join(', ') || '-')}</td>
                <td>${issuesText}</td>
            </tr>`;
        });
        html += '</table>';
    }
    document.getElementById('profiles').innerHTML = html;
}

async function loadProjects() {
    const [projects, check] = await Promise.all([
        fetchJSON('/api/projects'),
        fetchJSON('/api/check'),
    ]);
    const container = document.getElementById('projects');
    let html = `
        <h2>Projects</h2>
        <div class="toolbar">
            <button type="button" class="btn-secondary" data-plan-current>Preview Current Workspace Plan</button>
        </div>
        ${renderIssueList((check.issues || []).filter(issue => issue.type === 'missing_project'))}
    `;

    if (!projects || projects.length === 0) {
        html += '<p>No projects installed yet.</p>';
    } else {
        html += '<table><tr><th>Path</th><th>Profile</th><th>Extra Skills</th><th>Extra MCP</th><th>Last Installed</th><th>Curation Plan</th></tr>';
        projects.forEach(p => {
            html += `<tr>
                <td>${escapeHTML(p.path)}</td>
                <td>${escapeHTML(p.profile || '-')}</td>
                <td>${escapeHTML((p.extra_skills || []).join(', ') || '-')}</td>
                <td>${escapeHTML((p.extra_mcp || []).join(', ') || '-')}</td>
                <td>${escapeHTML(p.last_installed || '-')}</td>
                <td><button type="button" class="btn-secondary" data-plan-project="${escapeHTML(p.path)}">Preview Plan</button></td>
            </tr>`;
        });
        html += '</table>';
    }
    html += '<div id="plan-preview"></div>';
    container.innerHTML = html;

    container.onclick = e => {
        const projectBtn = e.target.closest('[data-plan-project]');
        if (projectBtn) {
            loadPlanPreview(projectBtn.dataset.planProject);
            return;
        }
        if (e.target.closest('[data-plan-current]')) {
            loadPlanPreview('');
            return;
        }
        if (e.target.closest('[data-close-plan]')) {
            document.getElementById('plan-preview').innerHTML = '';
        }
    };
}

async function loadPlanPreview(projectPath) {
    const panel = document.getElementById('plan-preview');
    panel.innerHTML = '<div class="detail-panel"><p>Loading Curation Plan preview...</p></div>';
    try {
        const url = projectPath ? '/api/plan?project=' + encodeURIComponent(projectPath) : '/api/plan';
        const plan = await fetchJSON(url);
        panel.innerHTML = renderPlanPreview(plan);
    } catch (err) {
        panel.innerHTML = `<div class="error-banner">Failed to preview plan: ${escapeHTML(err.message)}</div>`;
    }
}

function renderPlanPreview(plan) {
    const statusBadge = plan.bootstrap
        ? '<span class="badge badge-warning">bootstrap</span>'
        : plan.check
            ? '<span class="badge badge-ok">satisfied</span>'
            : '<span class="badge badge-error">needs action</span>';
    const bl = plan.baseline || {};
    const baselineDesc = plan.bootstrap
        ? 'No explicit target yet (choose with --profile or --skill in CLI)'
        : [
            bl.profile ? `profile=${bl.profile}` : '',
            (bl.skills || []).length ? `skills=${bl.skills.join(', ')}` : '',
            (bl.mcp || []).length ? `mcp=${bl.mcp.join(', ')}` : '',
        ].filter(Boolean).join(' · ') || '(empty curated baseline)';

    const proposals = plan.proposals || [];
    let proposalsHTML = '<p class="muted">No proposals.</p>';
    if (proposals.length > 0) {
        proposalsHTML = '<table><tr><th>Action</th><th>Agent</th><th>Skill</th><th>Ownership</th><th>Reason</th></tr>';
        proposals.forEach(pr => {
            const ownerLabel = pr.action === 'remove'
                ? (pr.owned ? '<span class="badge badge-ok">owned</span>' : '<span class="badge badge-warning">cleanup candidate</span>')
                : '-';
            proposalsHTML += `<tr>
                <td><span class="badge badge-action-${escapeHTML(pr.action)}">${escapeHTML(pr.action.toUpperCase())}</span></td>
                <td>${escapeHTML(pr.agent)}</td>
                <td>${escapeHTML(pr.skill)}</td>
                <td>${ownerLabel}</td>
                <td>${escapeHTML(pr.reason)}</td>
            </tr>`;
        });
        proposalsHTML += '</table>';
    }

    const warningsHTML = (plan.warnings || []).length > 0
        ? `<div class="warning-text">${plan.warnings.map(w => `<div>⚠ ${escapeHTML(w)}</div>`).join('')}</div>`
        : '';

    return `
        <div class="detail-panel plan-panel">
            <div class="panel-header">
                <h3>Curation Plan Preview ${statusBadge}</h3>
                <button type="button" class="btn-secondary" data-close-plan>Close</button>
            </div>
            <p class="muted"><strong>Project:</strong> ${escapeHTML(plan.project)} · <strong>Agents:</strong> ${escapeHTML((plan.agents || []).join(', ') || '-')}</p>
            <p class="muted"><strong>Baseline:</strong> ${escapeHTML(baselineDesc)}</p>
            ${warningsHTML}
            ${proposalsHTML}
            <p class="readonly-hint">Read-only preview. Run <code>sm plan --apply</code> in your terminal to apply changes.</p>
        </div>
    `;
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
            <td>${escapeHTML(i.installed_at)}</td>
            <td>${escapeHTML(i.project_path)}</td>
            <td>${escapeHTML(i.profile || '-')}</td>
            <td>${escapeHTML((i.skills || []).join(', ') || '-')}</td>
            <td>${escapeHTML((i.mcp || []).join(', ') || '-')}</td>
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
        html += `<tr><td>${escapeHTML(issue.type)}</td><td>${escapeHTML(issue.path)}</td></tr>`;
    });
    html += '</table></div>';
    return html;
}

function renderScoreBadge(score) {
    if (score === null || score === undefined) return '-';
    const cls = score >= 80 ? 'badge-ok' : score >= 60 ? 'badge-warning' : 'badge-error';
    return `<span class="badge ${cls}">${escapeHTML(score)}</span>`;
}

function renderLintSummary(q) {
    if (!q) return '-';
    if (q.errors > 0) return `<span class="badge badge-error">${q.errors} err${q.warnings > 0 ? ', ' + q.warnings + ' warn' : ''}</span>`;
    if (q.warnings > 0) return `<span class="badge badge-warning">${q.warnings} warn</span>`;
    return '<span class="badge badge-ok">ok</span>';
}

function renderOriginBadge(updateClass) {
    if (!updateClass) return '-';
    return `<span class="badge badge-origin-${escapeHTML(updateClass)}">${escapeHTML(updateClass)}</span>`;
}

function formatDate(value) {
    if (!value) return '-';
    return value.replace('T', ' ').replace('Z', ' UTC');
}
