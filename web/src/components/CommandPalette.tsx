import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { Ban, CornerDownLeft, FileText, Globe2, Loader2, MonitorSmartphone, Replace, Route as RouteIcon, Search } from "lucide-react";
import { searchResources, type SearchKind, type SearchResult } from "../api-search";
import { useI18n } from "../i18n-context";

export interface PaletteItem {
  id: string;
  label: string;
  hint?: string;
  icon: ReactNode;
  /** Words that also match, e.g. the group name. */
  keywords?: string;
  run: () => void;
}

interface Option extends PaletteItem {
  group: string;
}

const KIND_ICONS: Record<SearchKind, ReactNode> = {
  zone: <Globe2 size={16} />,
  record: <FileText size={16} />,
  client: <MonitorSmartphone size={16} />,
  rewrite: <Replace size={16} />,
  forwarding: <RouteIcon size={16} />,
  blocklist: <Ban size={16} />,
};

const DEBOUNCE_MS = 200;

function matches(query: string, ...values: (string | undefined)[]) {
  const haystack = values.filter(Boolean).join(" ").toLowerCase();
  return query.toLowerCase().split(/\s+/).filter(Boolean).every((word) => haystack.includes(word));
}

/**
 * Ctrl/Cmd+K command palette: a modal dialog with a combobox input and a
 * grouped listbox. Pages and actions are filtered locally; resources come
 * from a bounded server search.
 */
export function CommandPalette({ open, onClose, pages, actions }: { open: boolean; onClose: () => void; pages: PaletteItem[]; actions: PaletteItem[] }) {
  if (!open) return null;
  return <PaletteDialog onClose={onClose} pages={pages} actions={actions} />;
}

function PaletteDialog({ onClose, pages, actions }: { onClose: () => void; pages: PaletteItem[]; actions: PaletteItem[] }) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const listId = useId();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const [remote, setRemote] = useState<{ query: string; results: SearchResult[] } | null>(null);
  const [loading, setLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const returnFocus = useRef<Element | null>(document.activeElement);

  useEffect(() => {
    inputRef.current?.focus();
    const previous = returnFocus.current;
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);

  const trimmed = query.trim();
  useEffect(() => {
    if (trimmed.length < 2) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setLoading(true);
      searchResources(trimmed, controller.signal)
        .then((results) => setRemote({ query: trimmed, results }))
        .catch(() => {
          if (!controller.signal.aborted) setRemote({ query: trimmed, results: [] });
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    }, DEBOUNCE_MS);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [trimmed]);

  const options = useMemo<Option[]>(() => {
    const out: Option[] = [];
    for (const page of pages) if (!trimmed || matches(trimmed, page.label, page.keywords)) out.push({ ...page, group: t("palette.pages") });
    for (const action of actions) if (!trimmed || matches(trimmed, action.label, action.keywords)) out.push({ ...action, group: t("palette.actions") });
    if (trimmed.length >= 2 && remote?.query === trimmed) {
      remote.results.forEach((result, i) => {
        out.push({
          id: `result-${i}`,
          group: t(`palette.kind_${result.kind}`),
          label: result.title,
          hint: result.subtitle,
          icon: KIND_ICONS[result.kind] ?? <Search size={16} />,
          run: () => navigate(result.link),
        });
      });
    }
    return out;
  }, [pages, actions, trimmed, remote, t, navigate]);

  const current = Math.min(active, Math.max(options.length - 1, 0));
  const choose = (option: Option | undefined) => {
    if (!option) return;
    onClose();
    option.run();
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        setActive((current + 1) % Math.max(options.length, 1));
        break;
      case "ArrowUp":
        event.preventDefault();
        setActive((current - 1 + options.length) % Math.max(options.length, 1));
        break;
      case "Home":
        event.preventDefault();
        setActive(0);
        break;
      case "End":
        event.preventDefault();
        setActive(Math.max(options.length - 1, 0));
        break;
      case "Enter":
        event.preventDefault();
        choose(options[current]);
        break;
      case "Escape":
        event.preventDefault();
        event.stopPropagation();
        onClose();
        break;
      case "Tab":
        // The input is the only tab stop; keep focus inside the dialog.
        event.preventDefault();
        break;
    }
  };

  useEffect(() => {
    document.getElementById(`${listId}-${current}`)?.scrollIntoView?.({ block: "nearest" });
  }, [current, listId]);

  let lastGroup = "";
  const searching = loading || (trimmed.length >= 2 && remote?.query !== trimmed);
  return (
    <div className="palette-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <div className="palette" role="dialog" aria-modal="true" aria-label={t("palette.title")} onKeyDown={onKeyDown}>
        <div className="palette-input">
          <Search size={18} aria-hidden="true" />
          <input
            ref={inputRef}
            role="combobox"
            aria-expanded="true"
            aria-controls={listId}
            aria-autocomplete="list"
            aria-activedescendant={options.length ? `${listId}-${current}` : undefined}
            aria-label={t("palette.placeholder")}
            placeholder={t("palette.placeholder")}
            value={query}
            onChange={(event) => { setQuery(event.target.value); setActive(0); }}
            spellCheck={false}
            autoComplete="off"
          />
          {searching && <Loader2 size={16} className="spin" aria-hidden="true" />}
          <kbd>Esc</kbd>
        </div>
        <ul className="palette-list" id={listId} role="listbox" aria-label={t("palette.results")}>
          {options.map((option, index) => {
            const heading = option.group !== lastGroup ? option.group : "";
            lastGroup = option.group;
            return (
              <li key={`${option.group}-${option.id}`} role="presentation">
                {heading && <div className="palette-group" role="presentation">{heading}</div>}
                <div
                  id={`${listId}-${index}`}
                  role="option"
                  aria-selected={index === current}
                  className="palette-option"
                  onMouseMove={() => setActive(index)}
                  onClick={() => choose(option)}
                >
                  <span className="palette-icon" aria-hidden="true">{option.icon}</span>
                  <span className="palette-text">
                    <span>{option.label}</span>
                    {option.hint && <small>{option.hint}</small>}
                  </span>
                  {index === current && <CornerDownLeft size={14} className="palette-enter" aria-hidden="true" />}
                </div>
              </li>
            );
          })}
        </ul>
        {options.length === 0 && !searching && <p className="palette-empty" role="status">{t("palette.no_results")}</p>}
        <p className="palette-footer" aria-hidden="true">
          <span><kbd>↑</kbd><kbd>↓</kbd> {t("palette.navigate")}</span>
          <span><kbd>Enter</kbd> {t("palette.open")}</span>
          <span><kbd>Ctrl</kbd>+<kbd>K</kbd> {t("palette.toggle")}</span>
        </p>
      </div>
    </div>
  );
}
