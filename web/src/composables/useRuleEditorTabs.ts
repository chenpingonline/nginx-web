import { nextTick, ref } from 'vue';

// Keep every panel mounted so tab changes preserve edits and native validation.
export function useRuleEditorTabs(initial = 'basic') {
  const activeTab = ref(initial);
  let revealing = false;
  let reporting = false;
  function revealInvalid(event: Event) {
    if (reporting) return;
    const input = event.target;
    if (!(input instanceof HTMLElement)) return;
    const panel = input.closest<HTMLElement>('[data-editor-tab]');
    if (!panel) return;
    event.preventDefault();
    // Native validation emits one event per invalid field; reveal the first only.
    if (revealing) return;
    revealing = true;
    activeTab.value = panel.dataset.editorTab || initial;
    for (let parent = input.parentElement; parent && parent !== panel; parent = parent.parentElement) {
      if (parent instanceof HTMLDetailsElement) parent.open = true;
    }
    void nextTick(() => {
      input.focus();
      input.scrollIntoView({ block: 'nearest' });
      revealing = false;
      if (input instanceof HTMLInputElement || input instanceof HTMLTextAreaElement || input instanceof HTMLSelectElement) {
        reporting = true;
        input.reportValidity();
        reporting = false;
      }
    });
  }
  return { activeTab, revealInvalid };
}
