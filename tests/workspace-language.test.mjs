import assert from 'node:assert/strict';
import { translate } from '../frontend/language.js';

// Template arguments are data, including when they equal another catalog key.
assert.equal(translate('Student · Class 3', 'hi'), 'Student · कक्षा 3');
assert.equal(translate('Accuracy: 42 wpm', 'hi'), 'Accuracy: 42 शब्द/मिनट');
assert.equal(translate('View progress for School', 'hi'), 'School की प्रगति देखें');
assert.equal(translate('School: assessment history', 'hi'), 'School: आकलन इतिहास');
assert.equal(translate('Status: CHANGES_REQUESTED — Keep my words', 'hi'), 'स्थिति: बदलाव माँगे गए — Keep my words');
assert.equal(translate('27 Jul – 24 Oct 2026', 'hi'), '27 जुलाई – 24 अक्टूबर 2026');
assert.equal(translate('0% assessed · 0 of 0 learners', 'hi'), '0% आकलित · 0 में से 0 विद्यार्थी');
assert.equal(translate('Word review · 1 difference', 'hi'), 'शब्द समीक्षा · 1 अंतर');
assert.equal(translate('Word review · 2 differences', 'hi'), 'शब्द समीक्षा · 2 अंतर');
assert.equal(translate('Learner progress', 'en'), 'Learner progress');
assert.equal(translate('An unknown source string', 'hi'), 'An unknown source string');
