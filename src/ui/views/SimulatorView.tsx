import {
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  UNIT_IDS,
  emptyBuildings,
  type BuildingId,
  type UnitId,
  type Village,
} from "../../engine/types";
import { COMBAT_BALANCE } from "../../data/units.data";
import { BUILDINGS, BUILDING_ORDER } from "../../data/buildings.data";
import { RESEARCH_BALANCE } from "../../data/research.data";
import { effectiveResearchLevel } from "../../engine/units";
import { buildingName, unitName, villageName } from "../../i18n/names";
import { t, useT } from "../../i18n/i18n";
import { selectActiveVillage, useGameStore } from "../../state/gameStore";
import { useUiStore } from "../../state/uiStore";
import { NumberInput } from "../../shared/NumberInput";
import {
  fullUnits,
  simulateBattle,
  type SimRequest,
  type SimResult,
} from "../../net/simulate";
import { formatNumber } from "../format";
import { UnitIcon } from "../components/UnitIcon";
import {
  BattleSidePanel,
  DamageLines,
  LuckBar,
  ReportCard,
  ReportSection,
  usedUnits,
} from "../report/battleReport";

const ZERO = (): Record<UnitId, number> => fullUnits({});

/** The levels the Smithy can reach, as the choices in a level field. */
const LEVELS = Array.from(
  { length: RESEARCH_BALANCE.maxLevel },
  (_, i) => i + 1,
);

/** A fresh simulator starts at level 1: levels below that are the "not unlocked" floor and buy nothing. */
const ONE = (): Record<UnitId, number> =>
  Object.fromEntries(UNIT_IDS.map((u) => [u, 1])) as Record<UnitId, number>;

/** The town hall a village the player has not described is assumed to have: the level the server fills in, so the
 * field shows the number the run will actually use. Level 1 is the floor of the rule and cannot be broken. */
const SIM_DEFAULT_MAIN_LEVEL = 5;

/** A level record from anything partial, clamped to the Smithy's floor so a missing level reads as level 1, not 0. */
function levelsOf(
  levels: Partial<Record<UnitId, number>> | undefined,
): Record<UnitId, number> {
  const full = fullUnits(levels ?? {});
  return Object.fromEntries(
    UNIT_IDS.map((u) => [u, Math.max(1, full[u])]),
  ) as Record<UnitId, number>;
}

/**
 * The village's own levels. A unit that is still locked reads as level 1: the Smithy's floor, which multiplies it by
 * nothing either way, so the field shows the honest minimum instead of an option the list does not hold.
 */
function villageLevels(v: Village): Record<UnitId, number> {
  return Object.fromEntries(
    UNIT_IDS.map((u) => [u, Math.max(1, effectiveResearchLevel(v, u))]),
  ) as Record<UnitId, number>;
}

/** One simulation: what was asked and what came back, held together so the answer never mixes with later edits. */
interface Run {
  result: SimResult;
  attacker: Record<UnitId, number>;
  defender: Record<UnitId, number>;
  wall: number;
  catapultTarget: BuildingId | "";
}

/**
 * One unit's Smithy level. The noble is bought, not researched, so it has no level and gets an empty cell that keeps
 * the column under its heading.
 */
function LevelField({
  side,
  unit,
  value,
  onSet,
}: {
  side: "attacker" | "defender";
  unit: UnitId;
  value: number;
  onSet: (n: number) => void;
}) {
  useT();
  if (unit === "noble") return <span />;
  return (
    <label
      htmlFor={`sim-${side === "attacker" ? "atk" : "def"}-lvl-${unit}`}
      className="flex items-center"
    >
      <span className="sr-only">
        {t(`sim.${side}`)} {unitName(unit)} {t("sim.level")}
      </span>
      <select
        id={`sim-${side === "attacker" ? "atk" : "def"}-lvl-${unit}`}
        className="field w-12 sm:w-14"
        value={value}
        onChange={(e) => onSet(Number(e.target.value))}
      >
        {LEVELS.map((n) => (
          <option key={n} value={n}>
            {n}
          </option>
        ))}
      </select>
    </label>
  );
}

/**
 * Both armies at once: one row per unit, the attacker's field against the defender's, so the two sides of a fight read
 * across a single line. The unit is an icon with its name kept for screen readers — twelve names twice over would be
 * the widest thing on the screen and say nothing the icon does not. Each army carries its own Smithy level per unit,
 * because the levels are what a siege turns on and they are per village.
 */
function ArmyFields({
  attacker,
  defender,
  attackerResearch,
  defenderResearch,
  onSet,
  onSetLevel,
  hideDefender,
}: {
  attacker: Record<UnitId, number>;
  defender: Record<UnitId, number>;
  attackerResearch: Record<UnitId, number>;
  defenderResearch: Record<UnitId, number>;
  onSet: (side: "attacker" | "defender", u: UnitId, n: number) => void;
  onSetLevel: (side: "attacker" | "defender", u: UnitId, n: number) => void;
  /** The defender's column is gone when the sim is used to size one army on its own. */
  hideDefender?: boolean;
}) {
  useT();
  // The icon column is a fixed width, not `auto`: an `auto` column is the one that gives way when the row runs out of
  // space, and on a 320 px phone it collapsed to 0 px, hiding every unit icon — leaving the player typing numbers
  // against rows they could not tell apart.
  const cols = hideDefender
    ? "grid-cols-[18px_auto_auto]"
    : "grid-cols-[18px_auto_auto_auto_auto]";
  return (
    <div
      className={`grid ${cols} gap-x-1 sm:gap-x-2 gap-y-1 items-center w-fit`}
    >
      {/* The heading is the first rows of the same grid as the fields, so its labels sit over the columns they name.
          As a grid of its own it could not: an empty cell collapses to 0 px while the icon column does not, and the
          two headings drift right of the fields under them. Each side spans its two columns — the troops and the
          level that multiplies them. */}
      <span />
      <span className="text-center text-[12px] font-semibold col-span-2">
        {t("sim.attacker")}
      </span>
      {!hideDefender && (
        <span className="text-center text-[12px] font-semibold col-span-2">
          {t("sim.defender")}
        </span>
      )}
      <span />
      <span className="text-center text-[11px] text-muted">
        {t("sim.troops")}
      </span>
      <span className="text-center text-[11px] text-muted">
        {t("sim.level")}
      </span>
      {!hideDefender && (
        <span className="text-center text-[11px] text-muted">
          {t("sim.troops")}
        </span>
      )}
      {!hideDefender && (
        <span className="text-center text-[11px] text-muted">
          {t("sim.level")}
        </span>
      )}
      {/* One grid item per cell: the icon, then the attacker's troops and level, then the defender's. A wrapper here
          would emit more items per unit and the columns would drift a cell per row. */}
      {UNIT_IDS.map((u) => (
        <Fragment key={u}>
          <span
            className="flex items-center justify-center"
            title={unitName(u)}
          >
            <UnitIcon unit={u} size={18} />
          </span>
          <label htmlFor={`sim-atk-${u}`} className="flex items-center">
            <span className="sr-only">
              {t("sim.attacker")} {unitName(u)}
            </span>
            <NumberInput
              id={`sim-atk-${u}`}
              className="field w-14 sm:w-16 text-right"
              value={attacker[u]}
              onValue={(n) => onSet("attacker", u, n)}
              min={0}
              step={1}
            />
          </label>
          <LevelField
            side="attacker"
            unit={u}
            value={attackerResearch[u]}
            onSet={(n) => onSetLevel("attacker", u, n)}
          />
          {!hideDefender && (
            <>
              <label htmlFor={`sim-def-${u}`} className="flex items-center">
                <span className="sr-only">
                  {t("sim.defender")} {unitName(u)}
                </span>
                <NumberInput
                  id={`sim-def-${u}`}
                  className="field w-14 sm:w-16 text-right"
                  value={defender[u]}
                  onValue={(n) => onSet("defender", u, n)}
                  min={0}
                  step={1}
                />
              </label>
              <LevelField
                side="defender"
                unit={u}
                value={defenderResearch[u]}
                onSet={(n) => onSetLevel("defender", u, n)}
              />
            </>
          )}
        </Fragment>
      ))}
    </div>
  );
}

/**
 * The battle simulator: type two armies and the defence they meet, and read what a real battle would do with them.
 * The result comes from the server's own battle code, so it cannot drift from a raid actually sent.
 */
export function SimulatorTab() {
  useT();
  const worldId = useGameStore((s) => s.worldId);
  const village = useGameStore(selectActiveVillage);
  const clearPrefill = useUiStore((s) => s.consumeSimPrefill);

  const [attacker, setAttacker] = useState<Record<UnitId, number>>(ZERO);
  const [defender, setDefender] = useState<Record<UnitId, number>>(ZERO);
  // Levels start at 1: the floor of the Smithy, where every unit already is before anything is researched.
  const [attackerResearch, setAttackerResearch] =
    useState<Record<UnitId, number>>(ONE);
  const [defenderResearch, setDefenderResearch] =
    useState<Record<UnitId, number>>(ONE);
  const [wall, setWall] = useState(0);
  const [luck, setLuck] = useState(0);
  const [morale, setMorale] = useState(1);
  // The defender's buildings, when a report or the map knows them: catapults need something to aim at, and the
  // server otherwise guesses a town hall. Null means "an ordinary grown village".
  const [buildings, setBuildings] = useState<Record<BuildingId, number> | null>(
    null,
  );
  // Always a building: there is no "let the server decide" here, because what it would decide is a draw nobody can
  // read and the answer has to name the building it broke.
  const [catapultTarget, setCatapultTarget] = useState<BuildingId>("main");
  // The level the target stands at. A catapult can only break a building it can see the level of, so without this
  // the choice above would be quietly ignored and the run would hit whatever the server guessed instead. It starts
  // at the town hall's, matching the target a fresh simulator already has.
  const [catapultLevel, setCatapultLevel] = useState(SIM_DEFAULT_MAIN_LEVEL);
  // True while the fields still hold a report's armies and nothing the player typed, so a prefill's own target is
  // not thrown away before the player has touched anything.
  const prefilled = useRef(false);
  // The run's own snapshot: the result reads its armies from here, so typing the next fight does not rewrite the
  // last answer's "sent" column while the numbers beside it still describe the old battle.
  const [lastRun, setLastRun] = useState<Run | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const luckPct = useMemo(() => Math.round(COMBAT_BALANCE.luckRange * 100), []);

  // Catapults are the only thing that breaks a building, so without them there is no target to choose.
  const hasCatapults = attacker.catapult > 0;

  // The one building the catapults were told to aim at, at the level the player typed for it. The wall is left out:
  // it already has a field of its own above, and a second one would let a run fight over one wall and break another.
  const targetBuilding = useMemo(
    () =>
      hasCatapults && catapultTarget !== "wall" && catapultLevel > 0
        ? ({ [catapultTarget]: catapultLevel } as Partial<
            Record<BuildingId, number>
          >)
        : null,
    [hasCatapults, catapultTarget, catapultLevel],
  );

  // Picking a building shows the level it already has, so the field never starts at nothing to break. A building
  // nothing is known about starts at level 1: the lowest a village can have, and one a catapult can break, so the
  // choice is never silently dropped.
  const chooseTarget = useCallback(
    (id: BuildingId) => {
      setCatapultTarget(id);
      setCatapultLevel(
        buildings?.[id] ??
          (id === "main" ? SIM_DEFAULT_MAIN_LEVEL : id === "wall" ? wall : 1),
      );
    },
    [buildings, wall],
  );

  const request = useMemo<SimRequest>(
    () => ({
      attacker: { units: attacker, research: attackerResearch },
      defender: {
        units: defender,
        research: defenderResearch,
        wall,
        ...(buildings
          ? { buildings: { ...buildings, ...targetBuilding } }
          : targetBuilding
            ? { buildings: targetBuilding }
            : {}),
      },
      luck,
      morale,
      catapultTarget,
    }),
    [
      attacker,
      defender,
      attackerResearch,
      defenderResearch,
      wall,
      buildings,
      targetBuilding,
      luck,
      morale,
      catapultTarget,
    ],
  );

  // Asked for on a press rather than on every keystroke: half-typed armies are not a question anybody is asking.
  const run = useCallback(async () => {
    if (worldId === null) return;
    setBusy(true);
    setError(null);
    try {
      setLastRun({
        result: await simulateBattle(worldId, request),
        attacker,
        defender,
        wall,
        catapultTarget,
      });
    } catch {
      setError(t("sim.failed"));
      setLastRun(null);
    } finally {
      setBusy(false);
    }
  }, [worldId, request]);

  // The village can be either side: what is in it is what it would send, and also what would stand there if someone
  // came at it. So the button asks which, rather than deciding for the player.
  const [fillSide, setFillSide] = useState<"attacker" | "defender" | null>(
    null,
  );
  const fillFromVillage = useCallback(
    (side: "attacker" | "defender") => {
      if (!village) return;
      const units = fullUnits(village.units);
      const levels = villageLevels(village);
      if (side === "attacker") {
        setAttacker(units);
        setAttackerResearch(levels);
      } else {
        setDefender(units);
        setDefenderResearch(levels);
        // Its wall is part of what an attacker would meet, so it comes with the garrison.
        setWall(village.buildings.wall ?? 0);
      }
      prefilled.current = false;
      setFillSide(null);
    },
    [village],
  );

  // A prefill from the Map or a report arrives once, then is cleared so the tab keeps the player's own edits.
  useEffect(() => {
    const p = clearPrefill();
    if (!p) return;
    prefilled.current = true;
    setAttacker(fullUnits(p.attacker ?? {}));
    setDefender(fullUnits(p.defender ?? {}));
    if (p.attackerResearch) setAttackerResearch(levelsOf(p.attackerResearch));
    if (p.defenderResearch) setDefenderResearch(levelsOf(p.defenderResearch));
    if (p.wall !== undefined) setWall(p.wall);
    if (p.defenderBuildings)
      setBuildings({ ...emptyBuildings(), ...p.defenderBuildings });
    if (p.catapultTarget) {
      setCatapultTarget(p.catapultTarget);
      // The report knows the level the building stood at, so the replay shows it instead of asking again. A report
      // that broke nothing names no building, and the town hall's own default is the honest guess there.
      setCatapultLevel(
        p.defenderBuildings?.[p.catapultTarget] ?? SIM_DEFAULT_MAIN_LEVEL,
      );
    }
    if (p.morale !== undefined) setMorale(p.morale);
    if (p.luck !== undefined) setLuck(p.luck);
  }, [clearPrefill]);

  const hasTroops =
    Object.values(attacker).some((n) => n > 0) ||
    Object.values(defender).some((n) => n > 0);

  // Scouts sent on their own are a scouting mission, which the server resolves by a rule of its own: no luck, no
  // wall, no catapults. The settings for those would be fields that change nothing, so they are not offered.
  const scoutingOnly =
    attacker.scout > 0 &&
    UNIT_IDS.every((u) => u === "scout" || attacker[u] === 0);
  // A target chosen while there were catapults means nothing once the last one is taken away, and leaving it set would
  // keep sending it in a run that cannot use it. A prefill is the one time a target arrives without catapults of its
  // own, so it is allowed to stand until the player touches the army.
  useEffect(() => {
    if (hasCatapults) return;
    setCatapultTarget((t) => (prefilled.current ? t : "main"));
    if (!prefilled.current) setCatapultLevel(SIM_DEFAULT_MAIN_LEVEL);
  }, [hasCatapults]);

  return (
    <div className="max-w-[900px]">
      <p className="text-[12px] text-muted mb-3">{t("sim.hint")}</p>

      <div className="flex flex-wrap items-end gap-2 mb-3">
        <button
          type="button"
          className="btn"
          onClick={() => setFillSide("attacker")}
          disabled={!village}
        >
          {t("sim.fillVillage")}
        </button>
        <button
          type="button"
          className="btn"
          onClick={() => {
            setAttacker(ZERO());
            setDefender(ZERO());
            setAttackerResearch(ONE());
            setDefenderResearch(ONE());
            setCatapultTarget("main");
            setCatapultLevel(SIM_DEFAULT_MAIN_LEVEL);
            setLastRun(null);
          }}
        >
          {t("sim.clear")}
        </button>
      </div>

      {/* A desktop has two columns: the battle you describe on the left, its answer on the right. The armies come
          first and what the defence meets sits under them, so the eye runs down one column. A phone puts everything
          in one column, the answer under the question. Neither column is a framed panel: they are fields on the
          page, and boxing them only added borders to look past. */}
      <div className="flex flex-col lg:flex-row lg:items-start gap-3">
        <div className="flex flex-col gap-3 min-w-0">
          <div className="p-3 w-fit shrink-0">
            <ArmyFields
              attacker={attacker}
              defender={defender}
              attackerResearch={attackerResearch}
              defenderResearch={defenderResearch}
              onSet={(side, u, n) => {
                prefilled.current = false;
                (side === "attacker" ? setAttacker : setDefender)((army) => ({
                  ...army,
                  [u]: n,
                }));
              }}
              onSetLevel={(side, u, n) =>
                (side === "attacker"
                  ? setAttackerResearch
                  : setDefenderResearch)((lv) => ({ ...lv, [u]: n }))
              }
            />
          </div>
          <div className="p-3 flex flex-col gap-2 w-full max-w-[280px] shrink-0">
            {scoutingOnly && (
              <p className="text-[12px] text-muted">{t("sim.scoutRule")}</p>
            )}
            {!scoutingOnly && (
              <>
                <label
                  htmlFor="sim-wall"
                  className="text-[12px] flex items-center gap-2"
                >
                  {/* When the wall is the catapult target, this field is also the level they break it from, so it
                      says so rather than leaving a second "Wall level" to appear beside it. */}
                  <span className="flex-1">
                    {catapultTarget === "wall"
                      ? t("sim.catapultLevel", {
                          building: buildingName("wall"),
                        })
                      : t("sim.wall")}
                  </span>
                  <NumberInput
                    id="sim-wall"
                    className="field w-16 text-right"
                    value={wall}
                    onValue={(n) => setWall(n)}
                    min={0}
                    max={20}
                    step={1}
                  />
                </label>
                <label
                  htmlFor="sim-morale"
                  className="text-[12px] flex items-center gap-2"
                >
                  <span className="flex-1">{t("sim.morale")}</span>
                  <NumberInput
                    id="sim-morale"
                    className="field w-16 text-right"
                    value={morale}
                    onValue={(n) => setMorale(n)}
                    min={0}
                    max={4}
                    step={0.01}
                  />
                </label>
                <label
                  htmlFor="sim-luck"
                  className="text-[12px] flex flex-col gap-1"
                >
                  <span>{t("sim.luck", { range: luckPct })}</span>
                  <div className="flex items-center gap-2">
                    <input
                      id="sim-luck"
                      type="range"
                      className="flex-1"
                      min={-COMBAT_BALANCE.luckRange}
                      max={COMBAT_BALANCE.luckRange}
                      step={0.01}
                      value={luck}
                      onChange={(e) => setLuck(Number(e.target.value))}
                    />
                    <span className="tabular-nums w-[46px] text-right">
                      {Math.round(luck * 100)}%
                    </span>
                  </div>
                </label>
                {hasCatapults && (
                  <>
                    {/* The send screen's own control, but without its "a random building": a real battle draws that at
                        random, while the simulator answers one question. Leaving the choice out would make the server
                        take the first candidate, which is always the town hall — a label that says "random" over a
                        result that never varies. A building has to be named, and it is the one whose level is asked
                        for next. */}
                    <label htmlFor="sim-catapult" className="text-[12px] block">
                      {t("rally.catTarget")}{" "}
                      <select
                        id="sim-catapult"
                        className="px-1 border border-wood-border rounded bg-white/50"
                        value={catapultTarget}
                        onChange={(e) =>
                          chooseTarget(e.target.value as BuildingId)
                        }
                      >
                        {BUILDING_ORDER.map((b) => (
                          <option key={b} value={b}>
                            {buildingName(b)}
                          </option>
                        ))}
                      </select>
                    </label>
                    {catapultTarget !== "wall" && (
                      <label
                        htmlFor="sim-catapult-level"
                        className="text-[12px] flex items-center gap-2"
                      >
                        <span className="flex-1">
                          {t("sim.catapultLevel", {
                            building: buildingName(catapultTarget),
                          })}
                        </span>
                        <NumberInput
                          id="sim-catapult-level"
                          className="field w-16 text-right"
                          value={catapultLevel}
                          onValue={(n) => setCatapultLevel(n)}
                          min={1}
                          max={BUILDINGS[catapultTarget].maxLevel}
                          step={1}
                        />
                      </label>
                    )}
                  </>
                )}
              </>
            )}
          </div>
        </div>

        {/* The answer sits beside the question, so a change and its effect can be read without scrolling. */}
        <div className="flex flex-col gap-2 min-w-0 lg:flex-1">
          <div>
            <button
              type="button"
              className="btn btn-primary"
              onClick={run}
              disabled={busy || !hasTroops}
            >
              {busy ? t("sim.running") : t("sim.run")}
            </button>
          </div>
          {error && <p className="text-bad text-[12px]">{error}</p>}
          {lastRun && <SimulatorResult run={lastRun} />}
        </div>
      </div>

      {fillSide !== null && village && (
        <FillSideDialog
          village={village}
          onPick={fillFromVillage}
          onClose={() => setFillSide(null)}
        />
      )}
    </div>
  );
}

/**
 * Which side the village's troops should go on. The same garrison is both what the village would send and what would
 * stand in it, so the button cannot know which the player means; the Smithy levels and the wall follow the choice.
 */
function FillSideDialog({
  village,
  onPick,
  onClose,
}: {
  village: Village;
  onPick: (side: "attacker" | "defender") => void;
  onClose: () => void;
}) {
  useT();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div
      className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-2"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-label={t("sim.fillVillage")}
        className="panel w-full max-w-[320px]"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="panel-header flex items-center gap-2">
          <span>{t("sim.fillWhichSide")}</span>
          <button className="btn !px-1.5 !py-0 ml-auto" onClick={onClose}>
            ✕
          </button>
        </div>
        <div className="p-3 space-y-2">
          <p className="text-[12px] text-muted">
            {t("sim.fillHint", { village: villageName(village) })}
          </p>
          <div className="flex flex-wrap gap-2">
            <button className="btn" onClick={() => onPick("attacker")}>
              {t("sim.asAttacker")}
            </button>
            <button className="btn" onClick={() => onPick("defender")}>
              {t("sim.asDefender")}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

/**
 * What the last run did, drawn as a battle report: the same card, the same section bars, the same two army tables. A
 * player who has read a report already knows how to read this, and the two can be compared side by side without
 * translating between two layouts.
 */
function SimulatorResult({ run }: { run: Run }) {
  useT();
  const { result, attacker, defender } = run;
  const won = result.outcome === "attackerWon";
  // Scouts sent alone are a scouting mission, not a battle, and the server resolves them by a rule of their own.
  const scouting = result.kind === "scout";
  const compactUnits = useUiStore((s) => s.prefs.reportsCompactUnits);
  // The two sides' tables are wider than a phone, so each scrolls. Scrolling one scrolls the other, so the same unit
  // stays under the same finger — the same behaviour a real report has, which is what this card is meant to be.
  const [scrollers] = useState(() => new Set<HTMLDivElement>());
  const keepScrollInStep = (el: HTMLDivElement | null) => {
    if (!el || el.dataset.scrollWired) return;
    el.dataset.scrollWired = "1";
    scrollers.add(el);
    el.addEventListener("scroll", () => {
      for (const other of scrollers) {
        if (other !== el && other.scrollLeft !== el.scrollLeft)
          other.scrollLeft = el.scrollLeft;
      }
    });
  };
  // The columns, by the report's own rule: every unit, unless the player asked the reports to narrow to the ones the
  // battle used. Sharing the rule is what keeps the two cards identical.
  const used = usedUnits([
    attacker,
    defender,
    result.attackerLosses,
    result.defenderLosses,
  ]);
  const canNarrow = used.length > 0 && used.length < UNIT_IDS.length;
  const columns = compactUnits && canNarrow ? used : UNIT_IDS;
  return (
    <div className="mt-4 max-w-[900px] text-[12px]" data-sim-result>
      <ReportCard>
        <div
          className={`report-bar ${won ? "report-bar-ok" : "panel-header-loss"}`}
        >
          {scouting
            ? won
              ? t("reports.scoutOk")
              : t("reports.scoutFail")
            : won
              ? t("sim.attackerWon")
              : t("sim.defenderWon")}
        </div>
        <div className="px-2 py-1.5 space-y-1">
          <div>{scouting ? t("sim.atScout") : t("sim.atSimulated")}</div>
          {/* A scout mission is its own calculation: the scouts alone against the scouts kept at home, with no luck
              and no wall. A report leaves the luck bar out of a scouting for the same reason. */}
          {scouting ? (
            <div className="text-muted">{t("sim.scoutRule")}</div>
          ) : (
            <>
              <LuckBar luck={result.luck} />
              {result.morale !== 1 && (
                <div>
                  {t("reports.morale", { n: Math.round(result.morale * 100) })}
                </div>
              )}
            </>
          )}
        </div>
        <BattleSidePanel
          title={t("reports.attacker")}
          units={attacker}
          losses={result.attackerLosses}
          placeLabel={t("reports.origin")}
          village={t("sim.noVillage")}
          columns={columns}
          scroller={keepScrollInStep}
        />
        <BattleSidePanel
          title={t("reports.defender")}
          units={defender}
          losses={result.defenderLosses}
          placeLabel={t("reports.destination")}
          village={t("sim.noVillage")}
          columns={columns}
          scroller={keepScrollInStep}
        />
        {/* What the attack did to the village, in the report's own bars and its own wrapper. The simulator asks about
            the fight, not the loot, so there is no loot section; wall and building damage share one bar, as they do
            in a report. */}
        {(result.wallBefore !== result.wallAfter || result.buildingDamage) && (
          <div className="pt-1.5">
            <ReportSection label={t("reports.buildings")}>
              <DamageLines
                wallBefore={result.wallBefore}
                wallAfter={result.wallAfter}
                buildingDamage={result.buildingDamage}
              />
            </ReportSection>
          </div>
        )}
      </ReportCard>
    </div>
  );
}
