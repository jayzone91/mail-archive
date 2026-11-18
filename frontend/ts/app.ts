import "./job-monitor";

const theme = document.querySelector<HTMLButtonElement>("#theme");
const systemTheme = matchMedia("(prefers-color-scheme: dark)");

function applyTheme(value: string | null) {
  if (value) document.documentElement.dataset.theme = value;
  const dark = value ? value === "dark" : systemTheme.matches;
  theme?.setAttribute("aria-pressed", String(dark));
}

applyTheme(localStorage.getItem("theme"));
systemTheme.addEventListener("change", () => applyTheme(localStorage.getItem("theme")));
theme?.addEventListener("click", () => {
  const isDark = document.documentElement.dataset.theme === "dark"
    || (!document.documentElement.dataset.theme && systemTheme.matches);
  const value = isDark ? "light" : "dark";
  localStorage.setItem("theme", value);
  applyTheme(value);
});

const appMenu = document.querySelector<HTMLDetailsElement>("[data-app-menu]");
if (appMenu) {
  const compactMenu = matchMedia("(max-width: 68rem)");
  const updateMenu = () => { appMenu.open = !compactMenu.matches; };
  updateMenu();
  compactMenu.addEventListener("change", updateMenu);
}

const accountSelect = document.querySelector<HTMLSelectElement>("[data-archive-account]");
if (accountSelect?.form) {
  accountSelect.form.classList.add("enhanced");
  accountSelect.addEventListener("change", () => accountSelect.form?.requestSubmit());
}

const scope = document.querySelector<HTMLSelectElement>("[data-archive-scope]");
const searchFolders = document.querySelector<HTMLDetailsElement>("[data-archive-search-folders]");
if (scope && searchFolders) {
  const updateFolderChoices = () => {
    searchFolders.hidden = scope.value !== "folders";
    if (!searchFolders.hidden) searchFolders.open = true;
  };
  updateFolderChoices();
  scope.addEventListener("change", updateFolderChoices);
}

const archiveFolders = document.querySelector<HTMLDetailsElement>("[data-archive-folders]");
if (archiveFolders) {
  const compactFolders = matchMedia("(max-width: 52rem)");
  const updateFolders = () => { archiveFolders.open = !compactFolders.matches; };
  updateFolders();
  compactFolders.addEventListener("change", updateFolders);
}

const archiveList = document.querySelector<HTMLElement>(".archive-workspace .archive-list");
if (archiveList) {
  keepArchivePosition(archiveList);
}

function archivePositionKey(url: URL): string {
  const query = new URLSearchParams(url.search);
  query.delete("message");
  query.delete("view");
  query.sort();
  return `mailarchive:position:${url.pathname}?${query}`;
}

function keepArchivePosition(list: HTMLElement) {
  const workspace = list.closest<HTMLElement>(".archive-workspace");
  if (!workspace) return;

  const key = archivePositionKey(new URL(list.dataset.archivePosition ?? location.href, location.href));
  const folders = workspace.querySelector<HTMLElement>(".archive-folders");

  workspace.addEventListener("click", (event) => {
    if (!(event instanceof MouseEvent) || event.button !== 0 ||
        event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    const link = event.target instanceof Element ? event.target.closest<HTMLAnchorElement>("a[href]") : null;
    if (!link || list.getClientRects().length === 0) return;
    const target = new URL(link.href);
    if (target.origin !== location.origin || archivePositionKey(target) !== key) return;
    try {
      sessionStorage.setItem(key, JSON.stringify({
        list: list.scrollTop,
        folders: folders?.scrollTop ?? 0,
        page: window.scrollY,
      }));
    } catch {
      // The selected row is still made visible when session storage is unavailable.
    }
  });

  requestAnimationFrame(() => {
    if (list.getClientRects().length === 0) return;
    try {
      const raw = sessionStorage.getItem(key);
      if (raw) {
        const previous = JSON.parse(raw);
        list.scrollTop = previous.list;
        if (folders) folders.scrollTop = previous.folders;
        window.scrollTo(0, previous.page);
      }
    } catch {
      // Continue with the selected row when storage cannot be read.
    }
    list.querySelector(".archive-row-selected")?.scrollIntoView({ block: "nearest" });
  });
}
