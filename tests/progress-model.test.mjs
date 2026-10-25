import test from 'node:test';
import assert from 'node:assert/strict';
import { buildProgress, comparisonKey } from '../frontend/features/progress-model.js';

const attempt = (id, studentId, at, accuracy, overrides = {}) => ({ id, studentId, at, accuracy, wpm: accuracy === null ? null : accuracy + 10, passageId: 'en-1', passageVersion: 1, passageTitle: 'At school', language: 'english', provider: 'sarvam', providerVersion: 'saaras:v3', scoringVersion: 'v1', ...overrides });
function fixture() { return {
  from: '2026-10-01', through: '2026-10-24', days: 30,
  schools: [{ id: 'a', name: 'School A' }, { id: 'b', name: 'School B' }],
  students: [{ id: 'one', name: 'One', schoolId: 'a' }, { id: 'two', name: 'Two', schoolId: 'b' }, { id: 'three', name: 'Three', schoolId: 'a' }],
  assessments: [attempt('a', 'one', '2026-10-02T10:00:00Z', 50), attempt('b', 'one', '2026-10-20T10:00:00Z', 70), attempt('c', 'two', '2026-10-03T10:00:00Z', 0), attempt('d', 'two', '2026-10-21T10:00:00Z', 0)],
}; }
test('averages are per learner and change uses a matched cohort; zero remains valid', () => {
  const m = buildProgress(fixture());
  assert.equal(m.summary.assessed, 2); assert.equal(m.summary.enrolled, 3);
  assert.equal(m.summary.accuracy, 35); assert.equal(m.summary.change, 10);
  assert.equal(m.summary.paired, 2); assert.equal(m.summary.improved, 1); assert.equal(m.summary.unchanged, 1);
  assert.equal(m.students[2].status, 'unassessed');
});
test('never compares different passages, scoring versions or providers', () => {
  for (const change of [{ passageId: 'en-2' }, { passageVersion: 2 }, { scoringVersion: 'v2' }, { providerVersion: 'v4' }, { provider: 'other' }]) {
    const data = fixture(); Object.assign(data.assessments[1], change);
    assert.equal(buildProgress(data).students[0].change, null);
  }
  assert.equal(comparisonKey({ passageId: 'legacy' }), null);
});
test('missing score does not become zero or fall back to an older result', () => {
  const data = fixture(); data.assessments[1].accuracy = null;
  const model = buildProgress(data);
  assert.equal(model.students[0].latest.accuracy, null); assert.equal(model.students[0].change, null);
  assert.equal(model.summary.validLatest, 1); assert.equal(model.summary.average, 0);
});
test('school and student filters apply consistently and mock assessments stay separate', () => {
  const data = fixture(); data.assessments.push(attempt('mock', 'one', '2026-10-23T10:00:00Z', 100, { provider: 'mock' }));
  const school = buildProgress(data, { school: 'a' });
  assert.equal(school.summary.enrolled, 2); assert.equal(school.summary.assessed, 1); assert.equal(school.summary.average, 70);
  assert.equal(buildProgress(data, { student: 'one' }).selected.attempts.length, 2);
  const demo = buildProgress(data, { source: 'mock' });
  assert.equal(demo.summary.sessions, 1); assert.equal(demo.summary.paired, 0);
});
test('lower scores remain negative; weekly snapshots do not overweight repeated attempts', () => {
  const data = fixture(); data.assessments[1].accuracy = 30;
  data.assessments.push(attempt('extra', 'one', '2026-10-20T11:00:00Z', 20));
  const model = buildProgress(data);
  assert.equal(model.students[0].change, -30); assert.equal(model.summary.lower, 1);
  assert.equal(model.series.find(p => p.count === 2 && p.title.startsWith('15 Oct')).value, 10);
});
test('student chart keeps mixed passages disconnected until a comparison is chosen', () => {
  const data = fixture(); data.assessments.push(attempt('different', 'one', '2026-10-23T10:00:00Z', 90, { passageId: 'en-2' }));
  const m = buildProgress(data, { student: 'one', passage: comparisonKey(data.assessments[0]) });
  assert.equal(m.series.length, 2); assert.equal(m.summary.paired, 0);
});
