document.querySelectorAll<HTMLElement>("[data-job-monitor]").forEach(monitorJobs);

function monitorJobs(section: HTMLElement) {
  const endpoint = section.dataset.jobMonitor;
  const list = section.querySelector<HTMLElement>("[data-job-list]");
  const note = section.querySelector<HTMLElement>("[data-job-monitor-note]");
  if (!endpoint || !list || !note) return;

  let timer: number | undefined;
  let pending = false;
  let stopped = false;

  function schedule(delay: number) {
    window.clearTimeout(timer);
    if (!document.hidden && !stopped) timer = window.setTimeout(refresh, delay);
  }

  const refresh = async () => {
    if (document.hidden || stopped || pending) return;
    pending = true;
    let delay = 10_000;
    try {
      const response = await fetch(endpoint, { redirect: "manual", signal: AbortSignal.timeout(8_000) });
      if (response.type === "opaqueredirect" || response.status === 401 || response.status === 403) {
        stopped = true;
        note.textContent = "Die Sitzung ist abgelaufen. Melde dich erneut an, um den Status zu sehen.";
        note.hidden = false;
        return;
      }
      if (!response.ok) throw new Error("Status unavailable");
      const fragment = document.createElement("template");
      fragment.innerHTML = await response.text();
      const jobs = fragment.content.firstElementChild;
      if (!jobs?.hasAttribute("data-active-jobs")) throw new Error("Invalid job list");

      delay = jobs.getAttribute("data-active-jobs") === "true" ? 2_000 : 10_000;
      if (!list.contains(document.activeElement) && !jobs.isEqualNode(list.firstElementChild)) {
        list.replaceChildren(fragment.content);
      }
      note.hidden = true;
    } catch {
      note.textContent = "Der aktuelle Status ist vorübergehend nicht erreichbar. MailArchive versucht es erneut.";
      note.hidden = false;
    } finally {
      pending = false;
      schedule(delay);
    }
  };

  document.addEventListener("visibilitychange", () => schedule(0));
  list.addEventListener("focusout", () => schedule(0));
  schedule(2_000);
}
