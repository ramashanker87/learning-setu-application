import { api, session } from '../api.js';
import { el, dataEl, action, select, field } from '../ui.js';
import { buildProgress, comparisonKey, dateLabel, numeric } from './progress-model.js';
import { trendChart, changeRing, format, changeText } from './progress-charts.js';

const statuses = { improved: 'Improved', unchanged: 'Unchanged', lower: 'Lower latest score', repeat: 'Needs comparable repeat', unassessed: 'Not assessed yet' };
function panel(title, subtitle, className = '') {
  const node = el('section', undefined, { class: `insight-card ${className}` });
  const heading = el('div', undefined, { class: 'insight-card-heading' });
  heading.append(el('h2', title)); if (subtitle) heading.append(el('p', subtitle)); node.append(heading); return node;
}
function badge(change, metric) {
  return el('span', changeText(change, metric), { class: `growth-badge ${!numeric(change) ? 'growth-neutral' : change > 0 ? 'growth-positive' : change < 0 ? 'growth-lower' : 'growth-neutral'}` });
}
function statusBadge(status) {
  return el('span', statuses[status], { class: `reading-status status-${status}` });
}
function statusCounts(summary) {
  return Object.keys(statuses).map(key => [key, key === 'unassessed' ? summary.enrolled - summary.assessed : summary[key]]);
}
function statusBar(summary) {
  const bar = el('div', undefined, { class: 'reading-status-bar', role: 'img', 'aria-label': statusCounts(summary).map(([key, count]) => `${statuses[key]}: ${count}`).join(', ') });
  for (const [key, count] of statusCounts(summary)) {
    if (!count) continue;
    const segment = el('span', undefined, { class: `status-${key}`, title: `${statuses[key]}: ${count}` });
    segment.style.width = `${count / summary.enrolled * 100}%`; bar.append(segment);
  }
  return bar;
}
function coverage(summary) {
  const percent = summary.enrolled ? Math.round(summary.assessed / summary.enrolled * 100) : 0;
  const wrap = el('div', undefined, { class: 'reading-coverage' });
  wrap.append(el('span', `${percent}% assessed · ${summary.assessed} of ${summary.enrolled} learners`));
  const meter = el('progress', undefined, { max: '100', value: percent, 'aria-label': 'Assessment coverage', 'aria-valuetext': `${summary.assessed} of ${summary.enrolled} learners assessed` });
  wrap.append(meter); return wrap;
}
function resetOptions(control, entries, value) {
  control.replaceChildren(...entries.map(([key, label, verbatim]) => el('option', label, { value: key, ...(verbatim ? { translate: "no" } : {}) })));
  control.value = entries.some(([key]) => key === value) ? value : '';
}

export async function readingDashboard(root, organization, openStudents, openHistory) {
  root.classList.add('reading-dashboard');
  const filters = { language: 'english', days: '90', school: '', student: '', metric: 'accuracy', source: session.config?.authMode === 'local' ? 'mock' : 'real', passage: '' };
  let data, request = 0, page = 0, search = '', status = '';
  const heading = el('div', undefined, { class: 'progress-heading' });
  const intro = el('div');
  intro.append(el('p', `${organization.displayName} / LEARNING INSIGHTS`, { class: 'progress-eyebrow' }), el('h1', 'Reading progress'), el('p', 'A clearer picture of every learner’s reading journey.', { class: 'progress-subtitle' }));
  const tools = el('div', undefined, { class: 'progress-tools' });
  const refresh = action('↻ Refresh', () => load()); refresh.className = 'insight-secondary';
  const directory = action('Student directory →', openStudents); directory.className = 'insight-primary';
  tools.append(refresh, directory); heading.append(intro, tools);
  const controls = el('section', undefined, { class: 'progress-filters', 'aria-label': 'Progress dashboard filters' });
  const school = select(controls, 'progressSchool', 'School', [['', 'All schools']]);
  const student = select(controls, 'progressStudent', 'Student', [['', 'All students']]);
  const language = select(controls, 'progressLanguage', 'Reading language', [['english', 'English'], ['hindi', 'हिन्दी · Hindi']]);
  const period = select(controls, 'progressPeriod', 'Date range', [['30', 'Last 30 days'], ['90', 'Last 90 days'], ['180', 'Last 6 months'], ['365', 'Last 12 months']], '90');
  const source = select(controls, 'progressSource', 'Assessment data', [['real', 'Real assessments'], ['mock', 'Demo assessments']], filters.source);
  for (const control of controls.querySelectorAll('select')) control.setAttribute('aria-label', control.parentElement.firstChild.textContent);
  const context = el('div', undefined, { class: 'progress-context' });
  const scope = el('p', undefined, { role: 'status' });
  const metricButtons = el('div', undefined, { class: 'metric-switch', role: 'group', 'aria-label': 'Chart metric' });
  for (const [key, label] of [['accuracy', 'Accuracy'], ['wpm', 'Reading speed']]) {
    const button = action(label, () => { filters.metric = key; page = 0; render(); });
    button.dataset.metric = key; metricButtons.append(button);
  }
  context.append(scope, metricButtons);
  const content = el('div', undefined, { class: 'progress-content', 'aria-live': 'polite' });
  root.append(heading, controls, context, content);
  school.onchange = () => { filters.school = school.value; filters.student = ''; filters.passage = ''; page = 0; search = ''; render(); };
  student.onchange = () => { filters.student = student.value; filters.passage = ''; page = 0; render(); };
  language.onchange = () => { filters.language = language.value; filters.passage = ''; return load(); };
  period.onchange = () => { filters.days = period.value; filters.passage = ''; return load(); };
  source.onchange = () => { filters.source = source.value; filters.passage = ''; page = 0; render(); };

  async function load() {
    const current = ++request, org = session.org;
    content.replaceChildren(el('div', 'Gathering your reading insights…', { class: 'insight-empty', role: 'status' }));
    content.setAttribute('aria-busy', 'true'); scope.textContent = 'Loading assessments…';
    try {
      const next = await api(`/tenant/reading-progress?language=${filters.language}&days=${filters.days}`);
      if (request !== current || org !== session.org) return;
      if (!Array.isArray(next.students) || !Array.isArray(next.schools) || !Array.isArray(next.assessments)) throw new Error('Reading progress is temporarily unavailable.');
      data = next; page = 0; render();
    } catch (error) {
      if (request !== current || org !== session.org) return;
      data = null; scope.textContent = '';
      const failure = panel('We couldn’t load reading progress', error.message, 'progress-failure');
      failure.setAttribute('role', 'alert'); failure.append(action('Try loading again', load)); content.replaceChildren(failure);
    } finally { if (request === current) content.removeAttribute('aria-busy'); }
  }

  function render() {
    if (!data) return;
    if (!data.students.some(s => s.id === filters.student)) filters.student = '';
    let model = buildProgress(data, filters);
    if (filters.school && !model.schoolOptions.some(s => s.id === filters.school)) { filters.school = ''; model = buildProgress(data, filters); }
    resetOptions(school, [['', 'All schools'], ...model.schoolOptions.map(s => [s.id, s.name, s.id !== 'unassigned'])], filters.school);
    resetOptions(student, [['', 'All students'], ...model.schoolStudents.map(s => [s.id, `${s.name} · Class ${s.className || '—'}`])], filters.student);
    for (const button of metricButtons.children) button.setAttribute('aria-pressed', String(button.dataset.metric === filters.metric));
    const schoolName = model.schoolOptions.find(s => s.id === filters.school)?.name;
    scope.replaceChildren(model.selected?.name || schoolName ? dataEl('span', model.selected?.name || schoolName) : el('span', 'Organization overview'), ' · ', el('span', `${dateLabel(data.from)} – ${dateLabel(data.through)} ${data.through.slice(0, 4)}`), ' · UTC');
    content.replaceChildren();
    if (filters.source === 'mock') content.append(el('p', 'DEMO DATA · These scores use synthetic transcription and do not measure a learner’s actual reading.', { class: 'progress-demo' }));
    if (!model.summary.enrolled) {
      const empty = panel('Your reading story starts here', 'Add students and complete their first reading assessments. This dashboard will show real results as they become available.', 'progress-welcome');
      empty.append(el('div', '01  Add learners     →     02  Assess reading     →     03  Follow progress', { class: 'progress-empty-steps' }), action('Open student directory', openStudents)); content.append(empty); return;
    }
    const summary = model.summary, unit = filters.metric === 'accuracy' ? '%' : ' wpm';
    const cards = el('div', undefined, { class: 'progress-kpis' });
    const definitions = [
      ['Learners assessed', `${summary.assessed}`, `of ${summary.enrolled} learners in this view`, `${summary.sessions} completed sessions`, 'blue'],
      ['Latest reading accuracy', format(summary.accuracy, '%'), 'Average of each learner’s latest score', 'Recognition-based indicator', 'violet'],
      ['Comparable progress', numeric(summary.change) ? changeText(summary.change, filters.metric) : '—', `${summary.paired} learners with comparable repeats`, filters.metric === 'accuracy' ? 'pp = percentage points' : 'Change in words per minute', 'teal'],
      ['Latest reading speed', format(summary.wpm), 'Words per minute · latest per learner', 'Read alongside accuracy', 'amber'],
    ];
    for (const [title, value, description, footnote, color] of definitions) {
      const kpi = el('article', undefined, { class: `progress-kpi kpi-${color}` });
      kpi.append(el('p', title, { class: 'kpi-label' }), el('strong', value, { class: 'kpi-value' }), el('p', description, { class: 'kpi-description' }), el('span', footnote, { class: 'kpi-footnote' })); cards.append(kpi);
    }
    content.append(cards);
    const overview = panel(model.selected ? 'Reading status' : 'Reading status at a glance', `${model.selected ? 'Student' : schoolName ? 'School' : 'Organization'} view · ${filters.metric === 'accuracy' ? 'Accuracy' : 'Reading speed'} · Selected date range`, 'reading-status-overview');
    if (model.selected) {
      overview.append(statusBadge(model.selected.status), el('p', {
        improved: 'The latest comparable reading score increased. Continue regular reading practice.',
        unchanged: 'The latest comparable reading score is unchanged. Review the reading history to plan practice.',
        lower: 'The latest comparable reading score decreased. Review the recording and word differences before planning support.',
        repeat: 'Complete another reading of the same passage with matching assessment versions to measure progress.',
        unassessed: 'Complete a first reading assessment to begin tracking progress in this view.',
      }[model.selected.status], { class: 'insight-caption' }));
    } else {
      overview.append(coverage(summary), statusBar(summary));
      const breakdown = el('div', undefined, { class: 'reading-status-breakdown' });
      for (const [key, count] of statusCounts(summary)) {
        const item = action('', () => { status = status === key ? '' : key; page = 0; render(); content.querySelector('.learner-controls')?.scrollIntoView({ block: 'center' }); });
        item.className = `reading-status-tile status-${key}`;
        item.setAttribute('aria-pressed', String(status === key));
        item.setAttribute('aria-label', `${statuses[key]}: ${count} learners. Filter learner progress`);
        item.append(el('strong', count), el('span', statuses[key]), el('small', `${Math.round(count / summary.enrolled * 100)}% of learners`));
        breakdown.append(item);
      }
      overview.append(breakdown, el('p', 'Select a status to filter learners below. Coverage shows assessment participation; progress compares repeat readings of the same passage.', { class: 'insight-caption' }));
    }
    content.append(overview);
    const visual = el('div', undefined, { class: 'progress-visual-grid' });
    const trajectory = panel(model.selected ? `${model.selected.name}’s reading journey` : 'Reading over time', model.selected ? 'Explore individual assessment scores.' : 'Latest score per learner in each period; the group may change.');
    if (model.selected) {
      const options = new Map();
      for (const a of model.selected.attempts) { const key = comparisonKey(a); if (key) options.set(key, `${a.passageTitle || 'Reading passage'} · v${a.passageVersion} · ${a.providerVersion}`); }
      if (filters.passage && !options.has(filters.passage)) { filters.passage = ''; model = buildProgress(data, filters); }
      const picker = select(trajectory, 'progressPassage', 'Passage comparison', [['', 'All passages · individual scores'], ...[...options].map(([key, label]) => [key, label, true])], filters.passage);
      picker.onchange = () => { filters.passage = picker.value; render(); };
    }
    trajectory.append(trendChart(model.series, filters.metric, !!model.selected, !model.selected || !!filters.passage));
    trajectory.append(el('p', model.selected ? 'Select one passage to connect comparable readings. Different passages are shown as separate scores.' : 'This trend describes assessment results, not a matched learning gain. Use Comparable progress for repeat-reading change.', { class: 'insight-caption' }));
    const change = panel('Change across learners', `${summary.paired} learners with comparable repeat readings`);
    change.append(changeRing(summary));
    const legend = el('div', undefined, { class: 'change-legend' });
    for (const [key, count] of [['improved', summary.improved], ['unchanged', summary.unchanged], ['lower', summary.lower]]) {
      const row = el('div', undefined, { class: `legend-${key}` }); row.append(el('span', statuses[key]), el('strong', count)); legend.append(row);
    }
    change.append(legend, el('p', `${summary.repeat} assessed learners need a comparable repeat before change can be calculated.`, { class: 'insight-caption' }));
    visual.append(trajectory, change); content.append(visual);
    if (model.selected) {
      const detail = panel('A closer look', `${model.selected.schoolName || schoolName || 'School not linked'} · Class ${model.selected.className || '—'}`);
      const comparison = el('div', undefined, { class: 'student-comparison' });
      for (const [label, value] of [['First comparable reading', model.selected.baseline?.[filters.metric]], ['Latest reading', model.selected.latest?.[filters.metric]]]) {
        const item = el('div'); item.append(el('span', label), el('strong', format(value, unit))); comparison.append(item);
      }
      comparison.append(badge(model.selected.change, filters.metric)); detail.append(comparison);
      const comparisonNote = el('p', undefined, { class: 'insight-caption' });
      if (model.selected.baseline) comparisonNote.append(el('span', `${dateLabel(model.selected.baseline.at)} → ${dateLabel(model.selected.latest.at)}`), ' · ', model.selected.latest.passageTitle ? dataEl('span', model.selected.latest.passageTitle) : el('span', 'Same passage'));
      else comparisonNote.textContent = 'A repeat of the latest passage, with the same scoring and transcription versions, is needed to measure change.';
      detail.append(comparisonNote);
      detail.append(action('View assessment history →', () => openHistory({ sk: model.selected.id, name: model.selected.name })));
      content.append(detail);
    } else {
      const schools = panel('School comparison', 'Latest per-learner scores, with change calculated only for comparable repeat readings.');
      const list = el('div', undefined, { class: 'school-comparison' });
      if (!model.schools.length) list.append(el('p', 'Add schools to organize your learning insights.'));
      const max = filters.metric === 'accuracy' ? 100 : Math.max(100, ...model.schools.map(s => s.average || 0));
      for (const s of model.schools) {
        const row = el('div', undefined, { class: 'school-comparison-row' });
        const name = action(s.name, () => { filters.school = s.id; filters.student = ''; page = 0; render(); }); name.className = 'school-drilldown'; if (s.id !== 'unassigned') name.setAttribute('translate', 'no');
        const info = el('div'); info.append(name, el('p', `${s.assessed}/${s.enrolled} learners assessed · ${s.paired} comparable`, { class: 'insight-caption' }));
        const track = el('div', undefined, { class: 'school-track', 'aria-hidden': 'true' }); const fill = el('span'); fill.style.width = `${numeric(s.average) ? Math.max(0, Math.min(100, s.average / max * 100)) : 0}%`; track.append(fill);
        info.append(coverage(s), statusBar(s));
        row.append(info, track, el('strong', format(s.average, unit)), badge(s.change, filters.metric)); list.append(row);
      }
      schools.append(list, el('p', 'School grouping uses each learner’s current school. “School not linked” includes older records without a current school link.', { class: 'insight-caption' })); content.append(schools);
      const learners = panel('Learner progress', 'Find a student, then open their individual reading journey.');
      const tableControls = el('div', undefined, { class: 'learner-controls' });
      const query = field(tableControls, 'progressSearch', 'Find a learner', search, 'search', false); query.placeholder = 'Search name, school or class';
      const state = select(tableControls, 'progressStatus', 'Progress filter', [['', 'All learners'], ...Object.entries(statuses)], status);
      const tableArea = el('div');
      query.oninput = () => { search = query.value; page = 0; table(); };
      state.onchange = () => { status = state.value; page = 0; table(); };
      learners.append(tableControls, tableArea); content.append(learners);
      function table() {
        const q = search.trim().toLocaleLowerCase();
        const rows = model.students.filter(s => (!status || s.status === status) && `${s.name} ${s.schoolName || ''} ${s.className || ''}`.toLocaleLowerCase().includes(q));
        const size = 10, pages = Math.max(1, Math.ceil(rows.length / size)); page = Math.min(page, pages - 1);
        tableArea.replaceChildren();
        const scroll = el('div', undefined, { class: 'progress-table-scroll', tabindex: '0', role: 'region', 'aria-label': 'Learner progress table' });
        const tableNode = el('table', undefined, { class: 'progress-table' }); const head = el('thead'), header = el('tr');
        for (const label of ['Learner', 'Reading status', 'Latest score', 'Change', 'Sessions', 'Last assessed', '']) header.append(el('th', label, { scope: 'col' }));
        head.append(header); tableNode.append(head); const body = el('tbody');
        for (const s of rows.slice(page * size, (page + 1) * size)) {
          const row = el('tr'), person = el('th', undefined, { scope: 'row' });
          person.append(dataEl('span', s.name), el('small', `${s.schoolName || 'School not linked'} · Class ${s.className || '—'}`));
          const growth = el('td'); growth.append(badge(s.change, filters.metric));
          const open = action('View progress →', () => { filters.student = s.id; filters.passage = ''; render(); heading.scrollIntoView({ block: 'start' }); }); open.setAttribute('aria-label', `View progress for ${s.name}`); open.className = 'learner-drilldown';
          const cell = el('td'); cell.append(open);
          const readingStatus = el('td'); readingStatus.append(statusBadge(s.status));
          row.append(person, readingStatus, el('td', format(s.latest?.[filters.metric], unit)), growth, el('td', s.attempts.length), el('td', s.latest ? dateLabel(s.latest.at) : 'Not yet'), cell); body.append(row);
        }
        tableNode.append(body); scroll.append(tableNode); tableArea.append(scroll);
        if (!rows.length) tableArea.append(el('p', 'No learners match these filters.', { class: 'insight-empty' }));
        const footer = el('div', undefined, { class: 'progress-pagination' });
        footer.append(el('span', `${rows.length ? page * size + 1 : 0}–${Math.min((page + 1) * size, rows.length)} of ${rows.length} learners`));
        const prev = action('← Previous', () => { page--; table(); }), next = action('Next →', () => { page++; table(); });
        prev.disabled = page === 0; next.disabled = page >= pages - 1; footer.append(prev, next); tableArea.append(footer);
      }
      table();
    }
    const method = el('details', undefined, { class: 'progress-method' });
    method.append(el('summary', 'How to read these insights'), el('p', 'Only completed assessments in the selected language and UTC date range are included. Each learner contributes one latest score to overview averages. Missing scores remain unavailable, never zero. Demo results are kept separate from real assessments.'), el('p', 'Comparable progress uses each learner’s latest assessment and their earliest earlier assessment of that exact passage and passage version, with matching provider, provider version and scoring version. Both scores must be available. It describes performance on repeated reading, not a standardized learning gain.'), el('p', 'Accuracy and speed come from recognized speech. Transcription mistakes, including English words written in another script, can distort scores. Review the recording and word differences before drawing conclusions; faster reading alone does not establish better comprehension.'));
    content.append(method, el('p', `Updated ${new Date(data.generatedAt).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', timeZone: 'UTC' })} UTC · ${filters.source === 'mock' ? 'Demonstration' : 'Organization'} assessments`, { class: 'progress-updated' }));
  }
  await load();
}
