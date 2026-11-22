import { useMemo, useState } from "react";
import { t as tr } from "../../i18n/i18n";
import { useGameStore } from "../../state/gameStore";
import { markKey, useUiStore } from "../../state/uiStore";
import { nextMarkColor } from "./playerMarks";
import { HUMAN_ID } from "../../engine/types";

/**
 * Other players picked out by name, each in a colour the player chooses: their villages take that colour on the map and
 * their dots on the minimap. The marks are a display choice kept in the browser, so the panel sits under the map next
 * to the hint about how the map works.
 */
export function PlayerMarksPanel() {
  const marks = useUiStore((s) => s.prefs.mapPlayerMarks);
  const setPrefs = useUiStore((s) => s.setPrefs);
  const [name, setName] = useState("");
  const [picked, setPicked] = useState<string | null>(null);
  const color = picked ?? nextMarkColor(marks);
  // Every player in the world is in the state, so the names can be offered as suggestions even where the map has not
  // loaded their villages yet. Marking yourself is pointless: your own villages already have their red roofs.
  const known = useGameStore((s) => s.game?.players);
  const names = useMemo(
    () =>
      Object.values(known ?? {})
        .filter((p) => p.id !== HUMAN_ID && p.status !== "pending")
        .map((p) => p.name)
        .sort((a, b) => a.localeCompare(b)),
    [known],
  );
  const typed = name.trim();
  const unknown =
    typed.length > 0 && !names.some((n) => markKey(n) === markKey(typed));

  const add = () => {
    if (!typed) return;
    const key = markKey(typed);
    const rest = marks.filter((m) => markKey(m.name) !== key);
    setPrefs({ mapPlayerMarks: [...rest, { name: typed, color }] });
    setName("");
    setPicked(null);
  };
  const remove = (key: string) =>
    setPrefs({ mapPlayerMarks: marks.filter((m) => markKey(m.name) !== key) });
  const recolor = (key: string, c: string) =>
    setPrefs({
      mapPlayerMarks: marks.map((m) =>
        markKey(m.name) === key ? { ...m, color: c } : m,
      ),
    });

  return (
    <fieldset className="panel px-2 py-1.5 text-[12px] mt-3" data-player-marks>
      <legend className="sr-only">{tr("map.marks.title")}</legend>
      <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
        <span className="text-muted" title={tr("map.marks.hint")}>
          {tr("map.marks.title")}
        </span>
        <form
          className="inline-flex flex-wrap items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault();
            add();
          }}
        >
          <input
            className="w-36 px-1 border border-wood-border rounded bg-white/50"
            placeholder={tr("map.marks.name")}
            aria-label={tr("map.marks.name")}
            list="map-player-marks-names"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <datalist id="map-player-marks-names">
            {names.map((n) => (
              <option key={n} value={n} />
            ))}
          </datalist>
          <input
            type="color"
            className="h-6 w-8 border border-wood-border rounded bg-white/50 p-0"
            aria-label={tr("map.marks.pick")}
            title={tr("map.marks.pick")}
            value={color}
            onChange={(e) => setPicked(e.target.value)}
          />
          <button className="btn" type="submit" disabled={!typed}>
            {tr("map.marks.add")}
          </button>
        </form>
        {marks.length > 0 && (
          <ul className="inline-flex flex-wrap items-center gap-x-2 gap-y-1 ml-1">
            {marks.map((m) => (
              <li
                key={markKey(m.name)}
                data-mark-row={m.name}
                className="inline-flex items-center gap-1"
              >
                <input
                  type="color"
                  className="h-5 w-6 border border-wood-border rounded bg-white/50 p-0"
                  aria-label={tr("map.marks.color", { name: m.name })}
                  title={tr("map.marks.color", { name: m.name })}
                  value={m.color}
                  onChange={(e) => recolor(markKey(m.name), e.target.value)}
                />
                <span>{m.name}</span>
                <button
                  className="btn !px-1 !py-0 !text-[11px] leading-4"
                  aria-label={tr("map.marks.remove", { name: m.name })}
                  title={tr("map.marks.remove", { name: m.name })}
                  onClick={() => remove(markKey(m.name))}
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
      {unknown && (
        <p className="text-muted mt-1" data-mark-unknown>
          {tr("map.marks.unknown", { name: typed })}
        </p>
      )}
    </fieldset>
  );
}
