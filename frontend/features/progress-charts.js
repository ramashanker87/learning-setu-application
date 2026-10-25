import { el, dataEl } from '../ui.js';
import { numeric } from './progress-model.js';

const ns = 'http://www.w3.org/2000/svg';
export function svg(tag, attrs = {}, text) {
  const node = document.createElementNS(ns, tag);
  for (const [key, value] of Object.entries(attrs)) node.setAttribute(key, value);
  if (text !== undefined) node.textContent = text;
  return node;
}
export const format = (value, suffix = '') => numeric(value) ? `${new Intl.NumberFormat('en', { maximumFractionDigits: 1 }).format(value)}${suffix}` : '—';
export const changeText = (value, metric) => numeric(value) ? `${value > 0 ? '+' : ''}${format(value)} ${metric === 'accuracy' ? 'pp' : 'wpm'}` : 'Needs a repeat';

export function trendChart(points, metric, individual = false, connect = true) {
  const container = el('div', undefined, { class: 'progress-chart' });
  const unit = metric === 'accuracy' ? '%' : ' wpm';
  const available = points.filter(p => numeric(p.value));
  if (!available.length) {
    container.append(el('div', 'No reading scores in this view yet. Complete an assessment to start the timeline.', { class: 'insight-empty chart-empty' }));
    return container;
  }
  const width = 680, height = 250, left = 42, right = 24, top = 18, bottom = 40;
  const max = metric === 'accuracy' ? 100 : Math.max(20, Math.ceil(Math.max(...available.map(p => p.value)) / 20) * 20);
  const x = i => left + (points.length === 1 ? .5 : i / (points.length - 1)) * (width - left - right);
  const y = v => top + (1 - v / max) * (height - top - bottom);
  const chart = svg('svg', { viewBox: `0 0 ${width} ${height}`, role: 'img', 'aria-label': `${individual ? 'Individual assessment' : 'Period average'} ${metric === 'accuracy' ? 'reading accuracy' : 'reading speed'} chart` });
  chart.append(svg('title', {}, 'Reading results over time. Focus a point for its value, or open the chart data table.'));
  for (let i = 0; i <= 4; i++) {
    const value = max * i / 4;
    chart.append(svg('line', { x1: left, x2: width - right, y1: y(value), y2: y(value), stroke: '#e7edf4', 'stroke-dasharray': i ? '3 4' : '' }), svg('text', { x: left - 10, y: y(value) + 4, 'text-anchor': 'end', class: 'chart-label' }, format(value)));
  }
  const segments = []; let segment = [];
  points.forEach((p, i) => {
    if (!numeric(p.value) || (segment.length && (!connect || (individual && (!p.key || p.key !== segment.at(-1).key))))) { if (segment.length) segments.push(segment); segment = []; }
    if (numeric(p.value)) segment.push({ ...p, index: i });
  });
  if (segment.length) segments.push(segment);
  for (const part of segments) {
    if (part.length < 2) continue;
    const path = part.map((p, i) => `${i ? 'L' : 'M'}${x(p.index)},${y(p.value)}`).join(' ');
    chart.append(svg('path', { d: `${path} L${x(part.at(-1).index)},${y(0)} L${x(part[0].index)},${y(0)} Z`, fill: '#eef3ff' }), svg('path', { d: path, fill: 'none', stroke: '#5266db', 'stroke-width': 3, 'stroke-linejoin': 'round' }));
  }
  const detail = el('p', 'Select a point to explore its score.', { class: 'chart-detail', 'aria-live': 'polite' });
  points.forEach((point, i) => {
    if (i === 0 || i === points.length - 1 || (points.length <= 8 || i % Math.ceil(points.length / 6) === 0)) chart.append(svg('text', { x: x(i), y: height - 12, 'text-anchor': 'middle', class: 'chart-label' }, point.label));
    if (!numeric(point.value)) return;
    const text = `${point.title}: ${format(point.value, unit)}${individual ? '' : ` · ${point.count} learners`}`;
    const dot = svg('circle', { cx: x(i), cy: y(point.value), r: 6, fill: '#fff', stroke: '#5266db', 'stroke-width': 3, tabindex: '0', role: 'button', 'aria-label': text, class: 'chart-point' });
    dot.append(svg('title', {}, text));
    const reveal = () => { detail.textContent = text; };
    dot.addEventListener('focus', reveal); dot.addEventListener('pointerenter', reveal); dot.addEventListener('click', reveal);
    dot.addEventListener('keydown', event => { if (['Enter', ' '].includes(event.key)) { event.preventDefault(); reveal(); } });
    chart.append(dot);
  });
  const scroll = el('div', undefined, { class: 'chart-scroll', tabindex: '0', 'aria-label': 'Reading chart, scroll horizontally on a small screen' });
  scroll.append(chart); container.append(scroll, detail);
  const data = el('details', undefined, { class: 'chart-data' }); data.append(el('summary', 'View chart data'));
  const table = el('table'); const header = el('tr');
  for (const title of ['Period / passage', metric === 'accuracy' ? 'Accuracy' : 'Speed', 'Learners']) header.append(el('th', title, { scope: 'col' }));
  const head = el('thead'); head.append(header); table.append(head); const body = el('tbody');
  for (const p of points) { const row = el('tr'); row.append(dataEl('td', `${p.label} · ${p.title}`), el('td', format(p.value, unit)), el('td', p.count)); body.append(row); }
  table.append(body); data.append(table); container.append(data);
  return container;
}

export function changeRing(summary) {
  const wrap = el('div', undefined, { class: 'change-ring' });
  const chart = svg('svg', { viewBox: '0 0 180 180', role: 'img', 'aria-label': `${summary.improved} improved, ${summary.unchanged} unchanged, ${summary.lower} lower latest scores out of ${summary.paired} comparable learners` });
  chart.append(svg('circle', { cx: 90, cy: 90, r: 66, fill: 'none', stroke: '#edf1f6', 'stroke-width': 17 }));
  const circumference = 2 * Math.PI * 66;
  let offset = 0;
  for (const [count, color] of [[summary.improved, '#228577'], [summary.unchanged, '#7b8ca6'], [summary.lower, '#c27b33']]) {
    if (!count) continue;
    const length = count / summary.paired * circumference;
    chart.append(svg('circle', { cx: 90, cy: 90, r: 66, fill: 'none', stroke: color, 'stroke-width': 17, 'stroke-dasharray': `${length} ${circumference - length}`, 'stroke-dashoffset': -offset, transform: 'rotate(-90 90 90)' }));
    offset += length;
  }
  chart.append(svg('text', { x: 90, y: 87, 'text-anchor': 'middle', class: 'ring-value' }, summary.paired ? `${Math.round(summary.improved / summary.paired * 100)}%` : '—'), svg('text', { x: 90, y: 109, 'text-anchor': 'middle', class: 'ring-label' }, summary.paired ? 'improved' : 'awaiting repeats'));
  wrap.append(chart); return wrap;
}
