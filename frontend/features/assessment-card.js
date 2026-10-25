import { el, dataEl } from "../ui.js";

const number = value => typeof value === "number" && Number.isFinite(value) && value >= 0;
const metric = (value, suffix = "") => number(value) ? `${new Intl.NumberFormat("en", { maximumFractionDigits: 1 }).format(value)}${suffix}` : "Unavailable";
const duration = value => number(value) ? `${Math.floor(Math.round(value) / 60)}:${String(Math.round(value) % 60).padStart(2, "0")}` : "Unavailable";
const labels = { COMPLETED: "Completed", FAILED: "Needs attention", PROCESSING: "Processing", QUEUED: "Queued", AWAITING_UPLOAD: "Awaiting upload" };
const kinds = { skipped: "Skipped", substituted: "Substituted", inserted: "Added" };
function addMetric(parent, label, value, hint) {
  const item = el("div", undefined, { class: "assessment-metric" });
  item.append(el("dt", label), el("dd", value));
  if (hint) item.append(el("span", hint, { class: "metric-hint" }));
  parent.append(item);
}

export function assessmentCard(assessment) {
  const card = el("article", undefined, { class: "card assessment-card" });
  const header = el("header", undefined, { class: "assessment-card-header" });
  const heading = el("div");
  const language = assessment.passage?.language;
  const lang = language === "hindi" ? "hi" : language === "english" ? "en" : "";
  heading.append(el("p", language === "hindi" ? "हिन्दी · Hindi reading" : language === "english" ? "English reading" : "Reading assessment", { class: "section-kicker" }),
    assessment.passage?.title ? dataEl("h3", assessment.passage.title) : el("h3", "Imported assessment"));
  const date = new Date(assessment.completedAt || assessment.createdAt);
  heading.append(el("p", Number.isNaN(date.getTime()) ? "Date unavailable" : new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(date), { class: "assessment-date" }));
  header.append(heading, el("span", labels[assessment.state] || "Status unavailable", { class: "review-status", "data-state": assessment.state || "" }));
  card.append(header);
  const result = assessment.result || {};
  const score = result.score || {};
  const errors = Array.isArray(score.errors) ? score.errors : null;
  const counts = kind => errors ? errors.filter(error => error.error_type === kind).length : null;
  const metrics = el("dl", undefined, { class: "assessment-metrics" });
  addMetric(metrics, "Reading time", duration(result.duration ?? assessment.verifiedDuration ?? assessment.duration), "minutes : seconds");
  addMetric(metrics, "Speed", metric(result.wordsPerMinute), "words per minute");
  addMetric(metrics, "Accuracy", metric(score.accuracy_score, "%"), "compared with the passage");
  addMetric(metrics, "Pronunciation indicator", metric(score.pronunciation_score, "%"), "based on recognized words");
  card.append(metrics);
  if (!assessment.result) {
    card.append(el("p", assessment.state === "FAILED" ? "Results are unavailable. Review the message below." : "Reading results will appear when processing is complete.", { class: "assessment-note" }));
    return card;
  }
  const breakdown = el("dl", undefined, { class: "assessment-breakdown" });
  addMetric(breakdown, "Pauses", metric(score.pause_count));
  addMetric(breakdown, "Skipped words", metric(counts("skipped")));
  addMetric(breakdown, "Substituted words", metric(counts("substituted")));
  addMetric(breakdown, "Added words", metric(counts("inserted")));
  card.append(breakdown, el("p", "Automated indicators compare recognized speech with the passage. Review the recording and word differences before drawing conclusions about pronunciation.", { class: "assessment-note" }));
  if (result.provider === "mock" || assessment.provider === "mock") card.append(el("p", "Demonstration result · generated with the mock provider.", { class: "notice" }));
  const transcript = el("section", undefined, { class: "assessment-transcript", "aria-label": "Recognized speech" });
  transcript.append(el("h4", "Recognized speech"), el("p", typeof result.transcript === "string" ? result.transcript || "No speech was recognized." : "Transcript unavailable.", { lang: result.transcript ? (/[\u0900-\u097f]/u.test(result.transcript) ? "hi" : "en") : lang, ...(result.transcript ? { translate: "no" } : {}) }));
  card.append(transcript);
  const review = el("details", undefined, { class: "assessment-word-review" });
  review.append(el("summary", errors ? `Word review · ${errors.length} ${errors.length === 1 ? "difference" : "differences"}` : "Word review"));
  if (!errors) review.append(el("p", "Word-level review is unavailable for this assessment."));
  else if (!errors.length) review.append(el("p", "No word differences detected in the recognized speech."));
  else {
    const list = el("ul", undefined, { class: "word-review-list" });
    for (const error of errors) {
      const item = el("li", undefined, { class: "word-review-item", "data-kind": kinds[error.error_type] ? error.error_type : "unknown" });
      item.append(el("span", kinds[error.error_type] || "Review", { class: "word-review-kind" }));
      const words = el("dl", undefined, { class: "word-review-pair" });
      addMetric(words, "Passage word", error.expected_word || "—");
      addMetric(words, "Recognized word", error.spoken_word || "—");
      for (const word of words.querySelectorAll("dd")) { word.lang = /[\u0900-\u097f]/u.test(word.textContent) ? "hi" : "en"; word.setAttribute("translate", "no"); }
      item.append(words);
      if (number(error.position)) item.append(el("span", error.error_type === "inserted" ? (error.position === 0 ? "Before the first passage word" : `After passage word ${error.position}`) : `Passage word ${error.position + 1}`, { class: "word-review-position" }));
      list.append(item);
    }
    review.append(list);
  }
  card.append(review);
  if (assessment.passage?.text) {
    const reference = el("details", undefined, { class: "assessment-reference" });
    reference.append(el("summary", "Original reading passage"), dataEl("p", assessment.passage.text, { lang }));
    card.append(reference);
  }
  return card;
}
