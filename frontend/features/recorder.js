import { getLanguage } from "../language.js";
import { api, upload, session } from "../api.js";
import { el, dataEl, select, action } from "../ui.js";

const clock = seconds => `${String(Math.floor(seconds / 60)).padStart(2, "0")}:${String(Math.floor(seconds % 60)).padStart(2, "0")}`;

export async function readingSession(root, student, back, results) {
  const [processing, { items: passages }] = await Promise.all([api("/tenant/assessment-settings"), api("/tenant/passages")]);
  const processingDisabled = !processing.effectiveEnabled;
  root.replaceChildren();
  const workspace = el("section", undefined, { class: "reading-workspace" });
  root.append(workspace);
  const backButton = action("← All students", back);
  backButton.className = "quiet-button";
  workspace.append(backButton, el("p", "READING SESSION", { class: "section-kicker" }), el("h1", "Read, record, review"));
  const identity = el("div", undefined, { class: "reader-identity" });
  identity.append(el("span", student.name.slice(0, 1).toUpperCase(), { class: "reader-avatar", "aria-hidden": "true" }),
    dataEl("strong", student.name), el("span", `Class ${student.className} · ${student.schoolName || "School not recorded"}`));
  workspace.append(identity);
  if (!passages.length) { workspace.append(el("p", "No reading passages are available. Please contact your administrator.", { class: "notice" })); return; }
  if (student.consent?.status !== "GRANTED") { workspace.append(el("p", "Guardian authorization is required before recording. Ask your organization administrator to update it.", { class: "notice" })); return; }
  const grid = el("div", undefined, { class: "reading-grid" });
  const prompt = el("article", undefined, { class: "passage-panel" });
  const choices = el("div", undefined, { class: "reading-choices" });
  const languages = [...new Set(passages.map(p => p.language))];
  const language = select(choices, "readingLanguage", "Reading language", languages.map(l => [l, l === "hindi" ? "हिन्दी" : l === "english" ? "English" : l]));
  const preferredLanguage = getLanguage() === "hi" ? "hindi" : "english";
  if (languages.includes(preferredLanguage)) language.value = preferredLanguage;
  const passage = select(choices, "passage", "Reading passage", []);
  prompt.append(choices);
  const title = dataEl("h2"), text = dataEl("p", undefined, { class: "reading-text" }), metadata = el("p", undefined, { class: "passage-metadata" });
  prompt.append(title, text, metadata);
  const panel = el("aside", undefined, { class: "recorder-panel", "aria-label": "Audio recorder" });
  panel.append(el("p", "YOUR RECORDING", { class: "section-kicker" }), el("h2", "Let the student read aloud"), el("p", "Find a quiet spot. Keep the microphone nearby and read at a natural pace."));
  const status = el("p", "Ready to record", { class: "recording-status", role: "status", "aria-live": "polite" });
  const limit = Math.min(60, session.config.maxAudioSeconds || 60);
  const timer = el("div", "00:00", { class: "recording-timer", role: "timer", "aria-label": "Recording duration" });
  const meter = el("meter", undefined, { min: "0", max: "1", value: "0", "aria-label": "Microphone input level", class: "recording-meter" });
  const progress = el("progress", undefined, { max: String(limit), value: "0", "aria-label": "Recording time limit" });
  const audio = el("audio", undefined, { controls: "", "aria-label": "Review your recording" });
  audio.hidden = true;
  const primary = el("button", "Start recording", { type: "button", class: "record-button" });
  const again = el("button", "Record again", { type: "button", class: "quiet-button" });
  const submit = el("button", "Submit assessment", { type: "button" });
  const review = el("div", undefined, { class: "recording-review" });
  review.append(audio, again, submit);
  const feedback = el("div", undefined, { class: "assessment-feedback", "aria-live": "polite" });
  panel.append(status, timer, meter, progress, el("p", `Record 20–${limit} seconds. Stop within the limit. Longer recordings must be recorded again.`, { class: "recording-hint" }), primary, review, feedback);
  if (processingDisabled) panel.append(el("p", `You can record and listen here. ${processing.integrationsEnabled === false ? "Organization integrations are disabled. Contact your company administrator." : !processing.enabled ? "Your company administrator has disabled assessment processing for this organization." : "The assessment service is not configured or its worker is unavailable. Contact your service operator."} Unsaved audio is discarded when you leave.`, { class: "notice" }));
  grid.append(prompt, panel); workspace.append(grid);
  const mobileDock = el("div", undefined, { class: "mobile-recording-dock", "aria-label": "Recording controls" });
  const mobileStatus = el("span", "Ready to record", { class: "mobile-recording-status" });
  const mobileTime = el("span", "00:00", { "aria-label": "Recording duration" });
  const mobileRecord = el("button", "Start recording", { type: "button", class: "record-button" });
  const mobileSummary = el("div", undefined, { class: "mobile-dock-status" });
  mobileSummary.append(mobileStatus, mobileTime);
  const mobileAgain = el("button", "Record again", { type: "button", class: "quiet-button" });
  const mobileActions = el("div", undefined, { class: "mobile-dock-actions" });
  mobileActions.append(mobileAgain, mobileRecord);
  mobileDock.append(mobileSummary, mobileActions); workspace.append(mobileDock);
  const mobileViewport = matchMedia("(max-width: 720px)");
  const scrollTo = target => {
    if (mobileViewport.matches) target.scrollIntoView({ block: "start", behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth" });
  };
  const statusObserver = new MutationObserver(() => { mobileStatus.textContent = status.textContent; });
  statusObserver.observe(status, { childList: true, characterData: true, subtree: true });

  let state = "ready", recorder, stream, tick, cutoff, frame, audioContext, previewURL;
  let blob, duration = 0, selectedPassage, assessment, uploaded = false, disposed = false, started, pollTimer, limitExceeded = false;
  const current = () => passages.find(p => p.id === passage.value);
  function showPassage() {
    const p = current();
    title.textContent = p.title; text.textContent = p.text;
    text.lang = p.language === "hindi" ? "hi" : "en";
    metadata.textContent = `${p.text.trim().split(/\s+/u).length} words · Read at your natural pace`;
  }
  function chooseLanguage() {
    const options = passages.filter(p => p.language === language.value);
    passage.replaceChildren(...options.map(p => dataEl("option", p.title, { value: p.id })));
    const matching = options.find(p => String(p.className || p.class || "") === String(student.className));
    if (matching) passage.value = matching.id;
    showPassage();
  }
  language.onchange = chooseLanguage;
  passage.onchange = showPassage; chooseLanguage();
  function render() {
    const locked = state !== "ready";
    language.disabled = passage.disabled = locked;
    primary.hidden = ["invalid", "review", "submitting", "queued"].includes(state);
    primary.disabled = ["starting", "stopping"].includes(state);
    primary.textContent = state === "recording" ? "Stop recording" : state === "starting" ? "Opening microphone…" : state === "stopping" ? "Preparing audio…" : "Start recording";
    primary.setAttribute("aria-label", primary.textContent);
    primary.classList.toggle("is-recording", state === "recording");
    mobileDock.hidden = false;
    mobileAgain.hidden = !["review", "invalid"].includes(state);
    mobileRecord.disabled = state === "invalid" || primary.disabled || state === "submitting" || (state === "review" && processingDisabled);
    mobileRecord.textContent = ["invalid", "review"].includes(state) ? "Submit assessment" : state === "submitting" ? "Submitting…" : state === "queued" ? "View results" : primary.textContent;
    mobileRecord.setAttribute("aria-label", mobileRecord.textContent);
    mobileRecord.classList.toggle("is-recording", state === "recording");
    review.hidden = !["invalid", "review", "submitting", "queued"].includes(state);
    again.disabled = !["review", "invalid"].includes(state);
    submit.disabled = state !== "review" || processingDisabled;
    submit.textContent = state === "submitting" ? "Submitting…" : state === "queued" ? "Assessment submitted" : "Submit assessment";
    workspace.dataset.recording = String(state === "recording");
  }
  function stopCapture() {
    clearInterval(tick); clearTimeout(cutoff); cancelAnimationFrame(frame);
    stream?.getTracks().forEach(track => track.stop()); stream = null;
    if (audioContext) { audioContext.close().catch(() => {}); audioContext = null; }
    meter.value = 0;
  }
  function clearPreview() {
    audio.pause(); audio.removeAttribute("src"); audio.load(); audio.hidden = true;
    if (previewURL) URL.revokeObjectURL(previewURL);
    previewURL = null; blob = null; assessment = null; uploaded = false;
  }
  function cleanup() {
    if (disposed) return;
    disposed = true; clearTimeout(pollTimer);
    if (recorder) { recorder.onstop = null; recorder.ondataavailable = null; recorder.onerror = null; if (recorder.state !== "inactive") recorder.stop(); }
    stopCapture(); clearPreview(); observer.disconnect(); statusObserver.disconnect();
    window.removeEventListener("pagehide", cleanup);
  }
  const observer = new MutationObserver(() => { if (!workspace.isConnected) cleanup(); });
  observer.observe(document.body, { childList: true, subtree: true });
  window.addEventListener("pagehide", cleanup);
  function fail(message) { status.textContent = message; status.classList.add("error"); mobileStatus.textContent = message; }
  function stop(expired = false) {
    if (state !== "recording") return;
    duration = (performance.now() - started) / 1000;
    limitExceeded = expired || duration > limit;
    state = "stopping"; clearInterval(tick); clearTimeout(cutoff); render();
    recorder.stop();
  }
  async function start() {
    if (state !== "ready") return;
    status.classList.remove("error");
    if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) { fail("Recording is not supported here. Use a supported browser over HTTPS and allow microphone access."); return; }
    limitExceeded = false;
    state = "starting"; status.textContent = "Allow microphone access to begin."; render();
    try {
      const acquired = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (disposed) { acquired.getTracks().forEach(t => t.stop()); return; }
      stream = acquired;
      const mime = ["audio/webm;codecs=opus", "audio/mp4", "audio/ogg;codecs=opus", "audio/webm"].find(type => MediaRecorder.isTypeSupported(type));
      if (!mime) throw new Error("No supported audio format. Try another browser.");
      recorder = new MediaRecorder(stream, { mimeType: mime });
      const chunks = [];
      selectedPassage = current().id;
      recorder.ondataavailable = event => { if (event.data.size) chunks.push(event.data); };
      recorder.onerror = () => { cleanup(); fail("Recording was interrupted. Return to Students and start a new session."); primary.disabled = mobileRecord.disabled = true; };
      recorder.onstop = () => {
        stopCapture();
        if (disposed) return;
        if (limitExceeded) {
          clearPreview(); state = "invalid";
          timer.textContent = mobileTime.textContent = clock(duration); progress.value = limit;
          fail(`Recording exceeded ${limit} seconds and was discarded. Please record again. Submit assessment is disabled.`);
          render(); scrollTo(panel); return;
        }
        const type = (recorder.mimeType || mime).split(";")[0];
        blob = new Blob(chunks, { type });
        if (!blob.size || duration < 20 || duration > limit) { blob = null; state = "ready"; fail("Recording discarded. Record between 20 and 60 seconds of reading and try again."); render(); return; }
        if (blob.size > (session.config.maxAudioBytes || 10 * 1024 * 1024)) { blob = null; state = "ready"; fail("Recording is too large. Try a shorter reading."); render(); return; }
        previewURL = URL.createObjectURL(blob); audio.src = previewURL; audio.hidden = false;
        state = "review"; status.textContent = "Ready to review. Listen before submitting."; render();
        scrollTo(review);
      };
      recorder.start(); started = performance.now(); state = "recording";
      scrollTo(title);
      status.textContent = "Recording… Read the passage aloud, then stop.";
      tick = setInterval(() => { const elapsed = Math.min(limit, (performance.now() - started) / 1000); timer.textContent = mobileTime.textContent = clock(elapsed); progress.value = elapsed; }, 200);
      cutoff = setTimeout(() => stop(true), limit * 1000 + 1);
      try {
        const Context = window.AudioContext || window.webkitAudioContext;
        if (Context) {
          audioContext = new Context(); const analyser = audioContext.createAnalyser(); analyser.fftSize = 256;
          audioContext.createMediaStreamSource(stream).connect(analyser);
          const samples = new Uint8Array(analyser.fftSize);
          const draw = () => { if (disposed || state !== "recording") return; analyser.getByteTimeDomainData(samples); meter.value = Math.min(1, Math.sqrt(samples.reduce((sum, v) => sum + ((v - 128) / 128) ** 2, 0) / samples.length) * 4); frame = requestAnimationFrame(draw); }; draw();
        }
      } catch { /* Recording still works without a live level meter. */ }
      render();
    } catch (error) {
      stopCapture(); if (disposed) return;
      state = "ready";
      fail(["NotAllowedError", "NotFoundError", "NotReadableError"].includes(error.name) ? "Microphone unavailable. Allow access, check your microphone, and try again." : error.message);
      render();
    }
  }
  primary.onclick = () => { if (state === "recording") stop(); else start(); };
  mobileRecord.onclick = () => {
    if (state === "review") submit.click();
    else if (state === "queued") results();
    else primary.onclick();
  };
  mobileAgain.onclick = () => { again.click(); scrollTo(prompt); };
  again.onclick = () => { if (!["review", "invalid"].includes(state)) return; clearPreview(); state = "ready"; status.classList.remove("error"); status.textContent = "Ready for a new recording. Choose your passage, then start."; timer.textContent = mobileTime.textContent = "00:00"; progress.value = 0; render(); };
  async function poll() {
    if (disposed) return;
    try {
      const result = await api(`/tenant/assessments/${assessment.sk}`);
      if (disposed) return;
      if (result.state === "COMPLETED" || result.state === "FAILED") {
        status.textContent = result.state === "COMPLETED" ? "Assessment complete. Your results are ready." : "Processing failed. Open assessment history to review and retry.";
        feedback.replaceChildren(action("View results and history", results)); return;
      }
      status.textContent = result.state === "PROCESSING" ? "Analysing the reading… You can return to Students while this runs." : "Assessment queued. You can return to Students while this runs.";
      pollTimer = setTimeout(poll, 4000);
    } catch { if (!disposed) { status.textContent = "Unable to refresh progress. Check assessment history for the latest status."; feedback.replaceChildren(action("Check progress", poll), action("Assessment history", results)); } }
  }
  submit.onclick = async () => {
    if (state !== "review" || !blob || processingDisabled) return;
    state = "submitting"; status.classList.remove("error"); status.textContent = "Uploading your recording…"; render();
    try {
      if (!assessment) assessment = await api("/tenant/assessments", "POST", { studentId: student.sk, passageId: selectedPassage, type: blob.type, bytes: blob.size, duration });
      if (disposed) return;
      if (!uploaded) {
        try { await upload(assessment.upload, blob); uploaded = true; }
        catch (error) { assessment = null; throw error; }
      }
      if (disposed) return;
      await api(`/tenant/assessments/${assessment.sk}/complete`, "POST", {});
      if (disposed) return;
      state = "queued"; render(); feedback.replaceChildren(action("View progress and results", results)); poll();
    } catch (error) { if (!disposed) { state = "review"; fail(`${error.message} Your recording is still available here. Try submitting again.`); render(); } }
  };
  render();
}
