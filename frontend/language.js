import { workspaceHindi, workspacePatterns } from "./workspace-hindi.js";
const hindi = {
  "A stronger start.<br>For every reader.": "बेहतर शुरुआत।<br>हर पाठक के लिए।",
  "YOUR SCHOOL’S READING WORKSPACE": "आपके विद्यालय का पठन कार्यक्षेत्र",
  "TEACHER & SCHOOL SIGN-IN": "शिक्षक और विद्यालय साइन इन",
  "Student records": "विद्यार्थी रिकॉर्ड",
  "Reading sessions": "पठन सत्र",
  "Learning insights": "सीखने की समझ",
  "Continue to secure sign-in, then return to your school workspace.": "सुरक्षित साइन इन पूरा करके अपने विद्यालय के कार्यक्षेत्र में लौटें।",
  "Sign in to Learning Setu": "लर्निंग सेतु में साइन इन करें",
  "New organization? Apply for registration without creating an account.": "नया संगठन? खाता बनाए बिना पंजीकरण के लिए आवेदन करें।",
  "Apply to register your organization": "अपने संगठन के पंजीकरण के लिए आवेदन करें",
  "A bridge to better reading": "बेहतर पठन की ओर एक सेतु",
  "Every reader has<br>a next chapter.": "हर पाठक के लिए<br>एक नई शुरुआत।",
  "Understand where your learners are today.<br class=\"desktop-break\"> Help them take their next step with confidence.": "जानें कि आपके विद्यार्थी आज कहाँ हैं।<br class=\"desktop-break\"> उन्हें आत्मविश्वास के साथ अगला कदम बढ़ाने में मदद करें।",
  "Reading assessments, student records, and your teaching team — connected in one workspace.": "पठन आकलन, विद्यार्थियों के रिकॉर्ड और आपकी शिक्षण टीम — सब एक ही कार्यक्षेत्र में।",
  "Explore how it works": "जानें यह कैसे काम करता है",
  "Built for educators.": "शिक्षकों के लिए बनाया गया।",
  "Centered on learners.": "विद्यार्थियों को केंद्र में रखकर।",
  "Your learning workspace": "आपका शिक्षण कार्यक्षेत्र",
  "Welcome to Learning Setu": "लर्निंग सेतु में आपका स्वागत है",
  "Sign in to access your organization and continue supporting your learners.": "अपने संगठन के कार्यक्षेत्र में जाने और विद्यार्थियों की मदद जारी रखने के लिए साइन इन करें।",
  "New to Learning Setu? Sign in to begin your organization’s registration.": "लर्निंग सेतु पर नए हैं? अपने संगठन का पंजीकरण शुरू करने के लिए साइन इन करें।",
  "From reading to understanding": "पढ़ने से समझने तक",
  "A clear path for every assessment.": "हर आकलन के लिए एक स्पष्ट प्रक्रिया।",
  "Less time managing the process.<br>More focus on the learner.": "प्रक्रिया संभालने में कम समय।<br>विद्यार्थियों पर अधिक ध्यान।",
  "01 / Prepare": "01 / तैयारी",
  "Bring your learners together": "अपने विद्यार्थियों को एक साथ जोड़ें",
  "Manage student records and give your team the access they need in your organization’s workspace.": "विद्यार्थियों के रिकॉर्ड व्यवस्थित रखें और अपने संगठन के कार्यक्षेत्र में टीम को आवश्यक पहुँच दें।",
  "02 / Listen": "02 / सुनें",
  "Make room for every voice": "हर आवाज़ को दें जगह",
  "Start a reading session and record a learner’s reading for transcription and assessment.": "पठन सत्र शुरू करें और विद्यार्थी के पढ़ने की रिकॉर्डिंग करें, ताकि उसका लिखित रूपांतरण और आकलन किया जा सके।",
  "03 / Understand": "03 / समझें",
  "Turn reading into insight": "पठन से सीखने की समझ पाएँ",
  "Review completed assessments and results to help inform your next conversation with each learner.": "पूरे हुए आकलन और परिणामों की समीक्षा करें, ताकि हर विद्यार्थी के साथ अगली बातचीत को बेहतर दिशा दे सकें।",
  "Learning Setu": "लर्निंग सेतु",
  "Connecting learners, educators, and possibility.": "विद्यार्थियों, शिक्षकों और संभावनाओं को जोड़ता सेतु।",
  "Reading assessment": "पठन आकलन",
  "Demo workspace": "डेमो कार्यक्षेत्र",
  "Choose an account": "खाता चुनें",
  "Sign in to workspace →": "कार्यक्षेत्र में साइन इन करें →",
  "Local demonstration · Synthetic accounts only": "स्थानीय डेमो · केवल परीक्षण खाते",
  "Organization sign-in →": "संगठन के लिए साइन इन करें →",
  "Company administrator sign-in": "कंपनी प्रशासक के लिए साइन इन करें",
  "Skip to main content": "मुख्य सामग्री पर जाएँ",
  "Learning Setu home": "लर्निंग सेतु का मुख्य पृष्ठ",
  "Start application": "आवेदन शुरू करें",
  "Learning Setu | Reading assessment": "लर्निंग सेतु | पठन आकलन",
  "Bring clarity to reading assessment. Learning Setu helps educators manage students, record reading sessions, and review results.": "पठन आकलन को स्पष्ट और सरल बनाएँ। लर्निंग सेतु शिक्षकों को विद्यार्थियों के रिकॉर्ड सँभालने, पठन सत्र रिकॉर्ड करने और परिणामों की समीक्षा करने में मदद करता है।"
};
let language = "en";
try {
  if (localStorage.getItem("setu-language") === "hi") language = "hi";
} catch { /* Language switching also works when storage is unavailable. */ }

export function getLanguage() { return language; }
export function setLanguage(value) {
  language = value === "hi" ? "hi" : "en";
  try { localStorage.setItem("setu-language", language); } catch { /* Use this visit's preference. */ }
  window.dispatchEvent(new CustomEvent("setu-language-change", { detail: language }));
}
export function translate(text, locale = language) {
  if (locale !== "hi" || typeof text !== "string") return text;
  if (hindi[text] || workspaceHindi[text]) return hindi[text] || workspaceHindi[text];
  const t = value => translate(value, locale);
  for (const [pattern, render] of workspacePatterns) {
    const match = text.match(pattern);
    if (match) return render(match, t);
  }
  const months = { Jan: 'जनवरी', Feb: 'फ़रवरी', Mar: 'मार्च', Apr: 'अप्रैल', May: 'मई', Jun: 'जून', Jul: 'जुलाई', Aug: 'अगस्त', Sep: 'सितंबर', Sept: 'सितंबर', Oct: 'अक्टूबर', Nov: 'नवंबर', Dec: 'दिसंबर' };
  if (/^(?:\d{1,2} [A-Z][a-z]{2,3}|[A-Z][a-z]{2,3} \d{1,2}),?/.test(text) && !text.includes(' · ')) {
    return text.replace(/\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sept?|Oct|Nov|Dec|AM|PM)\b/g, word => months[word] || (word === 'AM' ? 'पूर्वाह्न' : 'अपराह्न'));
  }
  // Composed UI summaries contain independently translated labels and counts.
  for (const separator of [' · ', ', ']) {
    if (text.includes(separator)) return text.split(separator).map(t).join(separator);
  }
  return text;
}
export function updatePageLanguage(locale) {
  const t = (text) => translate(text, locale);
  document.documentElement.lang = locale;
  document.title = t("Learning Setu | Reading assessment");
  document.querySelector('meta[name="description"]').content = t("Bring clarity to reading assessment. Learning Setu helps educators manage students, record reading sessions, and review results.");
  document.querySelector(".brand strong").textContent = t("Learning Setu");
  document.querySelector(".brand").setAttribute("aria-label", t("Learning Setu home"));
  document.querySelector(".brand-description").textContent = t("Reading assessment");
  document.querySelector(".skip-link").textContent = t("Skip to main content");
  localizeTree(document.body);
  for (const button of document.querySelectorAll("[data-language]")) {
    button.setAttribute("aria-pressed", String(button.dataset.language === locale));
  }
}

// Translate display copy in place: switching locale must not recreate a form,
// reload assessment data, stop audio, or discard a live recording.
const originals = new WeakMap();
const attributes = ['aria-label', 'aria-valuetext', 'placeholder', 'title'];
const excluded = 'script, style, textarea, [translate="no"], [data-language]';
let observer;
function applyValue(node, key, read, write) {
  let values = originals.get(node);
  if (!values) { values = new Map(); originals.set(node, values); }
  const current = read();
  let entry = values.get(key);
  if (!entry || current !== entry.display) entry = { source: current };
  const trimmed = entry.source.trim();
  const translated = translate(trimmed);
  entry.display = trimmed ? entry.source.replace(trimmed, () => translated) : entry.source;
  values.set(key, entry);
  if (current !== entry.display) write(entry.display);
}
export function localizeTree(root) {
  if (!root) return;
  const element = root.nodeType === Node.TEXT_NODE ? root.parentElement : root;
  if (element?.closest?.(excluded)) return;
  const visit = node => {
    if (node.nodeType === Node.TEXT_NODE) {
      if (!node.parentElement?.closest(excluded)) applyValue(node, 'text', () => node.data, value => { node.data = value; });
    } else if (node.nodeType === Node.ELEMENT_NODE && !node.closest(excluded)) {
      for (const key of attributes) if (node.hasAttribute(key)) applyValue(node, key, () => node.getAttribute(key), value => node.setAttribute(key, value));
    }
  };
  visit(root);
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) visit(walker.currentNode);
}
export function startInterfaceTranslation() {
  if (observer) return;
  // Never observes form values, source records, or API payloads.
  observer = new MutationObserver(records => {
    for (const record of records) {
      if (record.type === 'childList') for (const node of record.addedNodes) localizeTree(node);
      else localizeTree(record.target);
    }
  });
  observer.observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: attributes });
  localizeTree(document.body);
}
