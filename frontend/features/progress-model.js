export const numeric = value => typeof value === 'number' && Number.isFinite(value);
export const mean = values => {
  const numbers = values.filter(numeric);
  return numbers.length ? numbers.reduce((sum, n) => sum + n, 0) / numbers.length : null;
};
export const round = value => numeric(value) ? Math.round(value * 10) / 10 : null;
export const dateLabel = value => new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(new Date(value));

export function comparisonKey(a) {
  const parts = [a.passageId, a.passageVersion, a.language, a.provider, a.providerVersion, a.scoringVersion];
  return parts.every(x => x !== undefined && x !== null && x !== '') ? JSON.stringify(parts) : null;
}
export function schoolKey(student, schools) {
  return schools.some(s => s.id === student.schoolId) ? student.schoolId : 'unassigned';
}
function learner(student, attempts, metric) {
  const latest = attempts.at(-1) || null;
  const key = latest && comparisonKey(latest);
  const baseline = key && numeric(latest[metric]) ? attempts.find(a => comparisonKey(a) === key && new Date(a.at) < new Date(latest.at) && numeric(a[metric])) : null;
  const change = baseline ? round(latest[metric] - baseline[metric]) : null;
  const status = !latest ? 'unassessed' : !numeric(change) ? 'repeat' : change > 0 ? 'improved' : change < 0 ? 'lower' : 'unchanged';
  return { ...student, attempts, latest, baseline, change, status };
}
function summarize(students, metric) {
  const assessed = students.filter(s => s.latest);
  const paired = students.filter(s => numeric(s.change));
  const latestValues = assessed.map(s => s.latest[metric]).filter(numeric);
  return {
    enrolled: students.length, assessed: assessed.length,
    sessions: students.reduce((sum, s) => sum + s.attempts.length, 0),
    average: mean(latestValues), validLatest: latestValues.length,
    accuracy: mean(assessed.map(s => s.latest.accuracy)), wpm: mean(assessed.map(s => s.latest.wpm)),
    change: mean(paired.map(s => s.change)), paired: paired.length,
    baseline: mean(paired.map(s => s.baseline[metric])), followup: mean(paired.map(s => s.latest[metric])),
    improved: paired.filter(s => s.change > 0).length,
    unchanged: paired.filter(s => s.change === 0).length,
    lower: paired.filter(s => s.change < 0).length,
    repeat: assessed.length - paired.length,
  };
}

export function buildProgress(data, filters = {}) {
  const metric = filters.metric || 'accuracy';
  const schoolOptions = [...data.schools];
  if (data.students.some(s => schoolKey(s, data.schools) === 'unassigned')) schoolOptions.push({ id: 'unassigned', name: 'School not linked' });
  const byStudent = new Map(data.students.map(s => [s.id, []]));
  const assessments = data.assessments.filter(a => filters.source === 'mock' ? a.provider === 'mock' : a.provider !== 'mock');
  for (const a of assessments) if (byStudent.has(a.studentId)) byStudent.get(a.studentId).push(a);
  for (const rows of byStudent.values()) rows.sort((a, b) => new Date(a.at) - new Date(b.at) || a.id.localeCompare(b.id));
  const schoolStudents = data.students.filter(s => !filters.school || schoolKey(s, data.schools) === filters.school);
  const students = schoolStudents.filter(s => !filters.student || s.id === filters.student).map(s => learner(s, byStudent.get(s.id), metric));
  const summary = summarize(students, metric);
  const schools = schoolOptions.filter(s => !filters.school || s.id === filters.school).map(s => ({ ...s, ...summarize(students.filter(student => schoolKey(student, data.schools) === s.id), metric) }));
  const selected = filters.student ? students[0] : null;
  const selectedIDs = new Set(students.map(s => s.id));
  const attempts = assessments.filter(a => selectedIDs.has(a.studentId));
  const series = [];
  if (selected) {
    for (const a of selected.attempts) {
      if (filters.passage && comparisonKey(a) !== filters.passage) continue;
      series.push({ at: a.at, label: dateLabel(a.at), value: numeric(a[metric]) ? a[metric] : null, count: 1, title: a.passageTitle || 'Imported passage', key: comparisonKey(a) });
    }
  } else {
    const day = 86400000, start = new Date(data.from + 'T00:00:00Z').getTime(), end = new Date(data.through + 'T00:00:00Z').getTime() + day;
    const interval = (data.days <= 30 ? 7 : data.days <= 90 ? 14 : 30) * day;
    for (let at = start; at < end; at += interval) {
      const windowEnd = Math.min(at + interval, end), latest = new Map();
      for (const a of attempts) {
        const when = new Date(a.at).getTime();
        if (when >= at && when < windowEnd && (!latest.has(a.studentId) || new Date(latest.get(a.studentId).at) < when)) latest.set(a.studentId, a);
      }
      const values = [...latest.values()].map(a => a[metric]).filter(numeric);
      series.push({ at: new Date(at).toISOString(), label: dateLabel(at), value: mean(values), count: values.length, title: `${dateLabel(at)}–${dateLabel(windowEnd - day)}`, key: 'cohort' });
    }
  }
  return { metric, summary, students, schools, schoolOptions, schoolStudents, selected, series };
}
