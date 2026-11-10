import { Fragment, useCallback, useEffect, useMemo, useState } from 'react';
import { BUILDING_IDS, UNIT_IDS, type BuildingId, type UnitId, type Village } from '../../engine/types';
import { COMBAT_BALANCE } from '../../data/units.data';
import { RESEARCH_BALANCE } from '../../data/research.data';
import { effectiveResearchLevel } from '../../engine/units';
import { buildingName, unitName } from '../../i18n/names';
import { t, useT } from '../../i18n/i18n';
import { selectActiveVillage, useGameStore } from '../../state/gameStore';
import { useUiStore } from '../../state/uiStore';
import { NumberInput } from '../../shared/NumberInput';
import { fullUnits, simulateBattle, type SimRequest, type SimResult } from '../../net/simulate';
import { formatNumber } from '../format';
import { UnitIcon } from '../components/UnitIcon';
import { PixelIcon } from '../pixelIcons';

const ZERO = (): Record<UnitId, number> => fullUnits({});

/** The levels the Smithy can reach, as the choices in a level field. */
const LEVELS = Array.from({ length: RESEARCH_BALANCE.maxLevel }, (_, i) => i + 1);

/** A fresh simulator starts at level 1: levels below that are the "not unlocked" floor and buy nothing. */
const ONE = (): Record<UnitId, number> => Object.fromEntries(UNIT_IDS.map((u) => [u, 1])) as Record<UnitId, number>;

/** A level record from anything partial, clamped to the Smithy's floor so a missing level reads as level 1, not 0. */
function levelsOf(levels: Partial<Record<UnitId, number>> | undefined): Record<UnitId, number> {
  const full = fullUnits(levels ?? {});
  return Object.fromEntries(UNIT_IDS.map((u) => [u, Math.max(1, full[u])])) as Record<UnitId, number>;
}

/**
 * The village's own levels. A unit that is still locked reads as level 1: the Smithy's floor, which multiplies it by
 * nothing either way, so the field shows the honest minimum instead of an option the list does not hold.
 */
function villageLevels(v: Village): Record<UnitId, number> {
  return Object.fromEntries(UNIT_IDS.map((u) => [u, Math.max(1, effectiveResearchLevel(v, u))])) as Record<UnitId, number>;
}

/** One simulation: what was asked and what came back, held together so the answer never mixes with later edits. */
interface Run {
  result: SimResult;
  attacker: Record<UnitId, number>;
  defender: Record<UnitId, number>;
  wall: number;
  hideLevel: number;
  catapultTarget: BuildingId | '';
}

/** The troops in an army, as a compact line: only the units that are actually there. */
function ArmyLine({ units }: { units: Partial<Record<UnitId, number>> }) {
  const present = UNIT_IDS.filter((u) => (units[u] ?? 0) > 0);
  if (present.length === 0) return <span className="text-muted">{t('sim.none')}</span>;
  return (
    <span className="inline-flex flex-wrap items-center gap-x-2 gap-y-1">
      {present.map((u) => (
        <span key={u} className="inline-flex items-center gap-1">
          <UnitIcon unit={u} size={14} />
          <span className="tabular-nums">{formatNumber(units[u] ?? 0)}</span>
        </span>
      ))}
    </span>
  );
}

/**
 * One unit's Smithy level. The noble is bought, not researched, so it has no level and gets an empty cell that keeps
 * the column under its heading.
 */
function LevelField({ side, unit, value, onSet }: {
  side: 'attacker' | 'defender';
  unit: UnitId;
  value: number;
  onSet: (n: number) => void;
}) {
  useT();
  if (unit === 'noble') return <span />;
  return (
    <label htmlFor={`sim-${side === 'attacker' ? 'atk' : 'def'}-lvl-${unit}`} className="flex items-center">
      <span className="sr-only">{t(`sim.${side}`)} {unitName(unit)} {t('sim.level')}</span>
      <select
        id={`sim-${side === 'attacker' ? 'atk' : 'def'}-lvl-${unit}`}
        className="field w-14"
        value={value}
        onChange={(e) => onSet(Number(e.target.value))}
      >
        {LEVELS.map((n) => <option key={n} value={n}>{n}</option>)}
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
function ArmyFields({ attacker, defender, attackerResearch, defenderResearch, onSet, onSetLevel, hideDefender }: {
  attacker: Record<UnitId, number>;
  defender: Record<UnitId, number>;
  attackerResearch: Record<UnitId, number>;
  defenderResearch: Record<UnitId, number>;
  onSet: (side: 'attacker' | 'defender', u: UnitId, n: number) => void;
  onSetLevel: (side: 'attacker' | 'defender', u: UnitId, n: number) => void;
  /** The defender's column is gone when the sim is used to size one army on its own. */
  hideDefender?: boolean;
}) {
  useT();
  const cols = hideDefender ? 'grid-cols-[auto_auto_auto]' : 'grid-cols-[auto_auto_auto_auto_auto]';
  return (
    <div className={`grid ${cols} gap-x-2 gap-y-1 items-center w-fit`}>
      {/* The heading is the first rows of the same grid as the fields, so its labels sit over the columns they name.
          As a grid of its own it could not: an empty cell collapses to 0 px while the icon column does not, and the
          two headings drift right of the fields under them. Each side spans its two columns — the troops and the
          level that multiplies them. */}
      <span />
      <span className="text-center text-[12px] font-semibold col-span-2">{t('sim.attacker')}</span>
      {!hideDefender && <span className="text-center text-[12px] font-semibold col-span-2">{t('sim.defender')}</span>}
      <span />
      <span className="text-center text-[11px] text-muted">{t('sim.troops')}</span>
      <span className="text-center text-[11px] text-muted">{t('sim.level')}</span>
      {!hideDefender && <span className="text-center text-[11px] text-muted">{t('sim.troops')}</span>}
      {!hideDefender && <span className="text-center text-[11px] text-muted">{t('sim.level')}</span>}
      {/* One grid item per cell: the icon, then the attacker's troops and level, then the defender's. A wrapper here
          would emit more items per unit and the columns would drift a cell per row. */}
      {UNIT_IDS.map((u) => (
        <Fragment key={u}>
          <span className="flex items-center justify-center" title={unitName(u)}>
            <UnitIcon unit={u} size={18} />
          </span>
          <label htmlFor={`sim-atk-${u}`} className="flex items-center">
            <span className="sr-only">{t('sim.attacker')} {unitName(u)}</span>
            <NumberInput
              id={`sim-atk-${u}`}
              className="field w-16 text-right"
              value={attacker[u]}
              onValue={(n) => onSet('attacker', u, n)}
              min={0}
              step={1}
            />
          </label>
          <LevelField side="attacker" unit={u} value={attackerResearch[u]} onSet={(n) => onSetLevel('attacker', u, n)} />
          {!hideDefender && (
            <>
              <label htmlFor={`sim-def-${u}`} className="flex items-center">
                <span className="sr-only">{t('sim.defender')} {unitName(u)}</span>
                <NumberInput
                  id={`sim-def-${u}`}
                  className="field w-16 text-right"
                  value={defender[u]}
                  onValue={(n) => onSet('defender', u, n)}
                  min={0}
                  step={1}
                />
              </label>
              <LevelField side="defender" unit={u} value={defenderResearch[u]} onSet={(n) => onSetLevel('defender', u, n)} />
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
  const [attackerResearch, setAttackerResearch] = useState<Record<UnitId, number>>(ONE);
  const [defenderResearch, setDefenderResearch] = useState<Record<UnitId, number>>(ONE);
  const [wall, setWall] = useState(0);
  const [luck, setLuck] = useState(0);
  const [morale, setMorale] = useState(1);
  const [hideLevel, setHideLevel] = useState(0);
  const [catapultTarget, setCatapultTarget] = useState<BuildingId | ''>('');
  // The run's own snapshot: the result reads its armies from here, so typing the next fight does not rewrite the
  // last answer's "sent" column while the numbers beside it still describe the old battle.
  const [lastRun, setLastRun] = useState<Run | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const luckPct = useMemo(() => Math.round(COMBAT_BALANCE.luckRange * 100), []);

  const request = useMemo<SimRequest>(() => ({
    attacker: { units: attacker, research: attackerResearch },
    defender: { units: defender, research: defenderResearch, wall, hideLevel },
    luck,
    morale,
    ...(catapultTarget ? { catapultTarget } : {}),
  }), [attacker, defender, attackerResearch, defenderResearch, wall, hideLevel, luck, morale, catapultTarget]);

  // Asked for on a press rather than on every keystroke: half-typed armies are not a question anybody is asking.
  const run = useCallback(async () => {
    if (worldId === null) return;
    setBusy(true);
    setError(null);
    try {
      setLastRun({ result: await simulateBattle(worldId, request), attacker, defender, wall, hideLevel, catapultTarget });
    } catch {
      setError(t('sim.failed'));
      setLastRun(null);
    } finally {
      setBusy(false);
    }
  }, [worldId, request]);

  // An empty battle is not worth asking about, so the fields start with the village's own troops and Smithy levels
  // filled in when the tab is opened from there.
  const fillFromVillage = useCallback(() => {
    if (!village) return;
    setAttacker(fullUnits(village.units));
    setAttackerResearch(villageLevels(village));
  }, [village]);

  // A prefill from the Map or a report arrives once, then is cleared so the tab keeps the player's own edits.
  useEffect(() => {
    const p = clearPrefill();
    if (!p) return;
    setAttacker(fullUnits(p.attacker ?? {}));
    setDefender(fullUnits(p.defender ?? {}));
    if (p.attackerResearch) setAttackerResearch(levelsOf(p.attackerResearch));
    if (p.defenderResearch) setDefenderResearch(levelsOf(p.defenderResearch));
    if (p.wall !== undefined) setWall(p.wall);
    if (p.hideLevel !== undefined) setHideLevel(p.hideLevel);
    if (p.morale !== undefined) setMorale(p.morale);
    if (p.luck !== undefined) setLuck(p.luck);
  }, [clearPrefill]);

  const hasTroops = Object.values(attacker).some((n) => n > 0) || Object.values(defender).some((n) => n > 0);

  return (
    <div className="max-w-[900px]">
      <p className="text-[12px] text-muted mb-3">{t('sim.hint')}</p>

      <div className="flex flex-wrap items-end gap-2 mb-3">
        <button type="button" className="btn" onClick={fillFromVillage} disabled={!village}>
          {t('sim.fillVillage')}
        </button>
        <button
          type="button"
          className="btn"
          onClick={() => {
            setAttacker(ZERO());
            setDefender(ZERO());
            setAttackerResearch(ONE());
            setDefenderResearch(ONE());
            setLastRun(null);
          }}
        >
          {t('sim.clear')}
        </button>
      </div>

      {/* A desktop has room either side: the armies on the left, what the battle meets on the right. A phone puts
          them in one column, the settings under the armies. */}
      <div className="flex flex-col lg:flex-row lg:items-start gap-3">
        <div className="panel p-3 w-fit shrink-0">
          <ArmyFields
            attacker={attacker}
            defender={defender}
            attackerResearch={attackerResearch}
            defenderResearch={defenderResearch}
            onSet={(side, u, n) => (side === 'attacker' ? setAttacker : setDefender)((army) => ({ ...army, [u]: n }))}
            onSetLevel={(side, u, n) => (side === 'attacker' ? setAttackerResearch : setDefenderResearch)((lv) => ({ ...lv, [u]: n }))}
          />
        </div>

        <div className="panel p-3 flex flex-col gap-2 w-full max-w-[280px] shrink-0">
          <label htmlFor="sim-wall" className="text-[12px] flex items-center gap-2">
            <span className="flex-1">{t('sim.wall')}</span>
            <NumberInput id="sim-wall" className="field w-16 text-right" value={wall} onValue={(n) => setWall(n)} min={0} max={20} step={1} />
          </label>
          <label htmlFor="sim-hide" className="text-[12px] flex items-center gap-2">
            <span className="flex-1">{t('sim.hide')}</span>
            <NumberInput id="sim-hide" className="field w-16 text-right" value={hideLevel} onValue={(n) => setHideLevel(n)} min={0} max={20} step={1} />
          </label>
          <label htmlFor="sim-morale" className="text-[12px] flex items-center gap-2">
            <span className="flex-1">{t('sim.morale')}</span>
            <NumberInput id="sim-morale" className="field w-16 text-right" value={morale} onValue={(n) => setMorale(n)} min={0} max={4} step={0.01} />
          </label>
          <label htmlFor="sim-luck" className="text-[12px] flex flex-col gap-1">
            <span>{t('sim.luck', { range: luckPct })}</span>
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
              <span className="tabular-nums w-[46px] text-right">{Math.round(luck * 100)}%</span>
            </div>
          </label>
          <label htmlFor="sim-catapult" className="text-[12px] flex flex-col gap-1">
            <span>{t('sim.catapultTarget')}</span>
            <select
              id="sim-catapult"
              className="field"
              value={catapultTarget}
              onChange={(e) => setCatapultTarget(e.target.value as BuildingId | '')}
            >
              <option value="">{t('sim.randomBuilding')}</option>
              {BUILDING_IDS.map((b) => <option key={b} value={b}>{buildingName(b)}</option>)}
            </select>
          </label>
        </div>
      </div>

      <div className="mt-3">
        <button type="button" className="btn btn-primary" onClick={run} disabled={busy || !hasTroops}>
          {busy ? t('sim.running') : t('sim.run')}
        </button>
      </div>
      {error && <p className="text-bad text-[12px] mt-2">{error}</p>}

      {lastRun && <SimulatorResult run={lastRun} />}
    </div>
  );
}

/** What the last run did: who won, what each side lost, and what the loot and wall look like afterwards. */
function SimulatorResult({ run }: { run: Run }) {
  useT();
  const { result, attacker, defender } = run;
  const won = result.outcome === 'attackerWon';
  const haulTotal = result.haul.wood + result.haul.clay + result.haul.iron;
  return (
    <div className="mt-4" data-sim-result>
      <h3 className={`text-[15px] mb-1 ${won ? 'text-ok' : 'text-bad'}`}>
        {won ? t('sim.attackerWon') : t('sim.defenderWon')}
      </h3>
      <p className="text-[12px] text-muted mb-2">
        {t('sim.powers', {
          attack: formatNumber(Math.round(result.attackPower)),
          defense: formatNumber(Math.round(result.defensePower)),
        })}
      </p>
      <div className="overflow-x-auto">
        <table className="table-game w-full text-[12px]">
          <thead>
            <tr>
              <th>{t('sim.side')}</th>
              <th>{t('sim.sent')}</th>
              <th>{t('sim.lost')}</th>
              <th>{t('sim.left')}</th>
              <th>{t('sim.cost')}</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td data-label={t('sim.side')}>{t('sim.attacker')}</td>
              <td data-label={t('sim.sent')}><ArmyLine units={attacker} /></td>
              <td data-label={t('sim.lost')}><ArmyLine units={result.attackerLosses} /></td>
              <td data-label={t('sim.left')}><ArmyLine units={result.attackerSurvivors} /></td>
              <td data-label={t('sim.cost')} className="tabular-nums">{formatNumber(Math.round(result.attackerCost))}</td>
            </tr>
            <tr>
              <td data-label={t('sim.side')}>{t('sim.defender')}</td>
              <td data-label={t('sim.sent')}><ArmyLine units={defender} /></td>
              <td data-label={t('sim.lost')}><ArmyLine units={result.defenderLosses} /></td>
              <td data-label={t('sim.left')}><ArmyLine units={result.defenderSurvivors} /></td>
              <td data-label={t('sim.cost')} className="tabular-nums">{formatNumber(Math.round(result.defenderCost))}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <ul className="mt-2 text-[12px] space-y-0.5">
        {result.wallAfter !== result.wallBefore && (
          <li>{t('sim.wallFell', { from: result.wallBefore, to: result.wallAfter })}</li>
        )}
        {result.buildingDamage && (
          <li>
            {t('sim.buildingHit', {
              building: buildingName(result.buildingDamage.building),
              from: result.buildingDamage.from,
              to: result.buildingDamage.to,
            })}
          </li>
        )}
        {/* A raid that brings nothing home is not worth a line: the same rule the wall and building lines follow. */}
        {won && haulTotal > 0 && (
          <li>{t('sim.haul', { loot: formatNumber(Math.round(haulTotal)) })}</li>
        )}
      </ul>
    </div>
  );
}
