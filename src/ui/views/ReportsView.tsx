import { buildingName, unitName } from "../../i18n/names";
import { PixelIcon } from "../pixelIcons";
import { useEffect, useState } from "react";
import { BUILDINGS, BUILDING_ORDER } from "../../data/buildings.data";
import { COMBAT_BALANCE, UNITS } from "../../data/units.data";
import { reportIcon, reportVillageOf } from "../../engine/reports";
import { reportVillageLabel } from "../../i18n/names";
import { t, useT } from "../../i18n/i18n";
import { AutoFarmTag, BotTag } from "../components/BotTag";
import {
  RESOURCES,
  UNIT_IDS,
  HUMAN_ID,
  type Report,
  type ReportType,
  type UnitId,
} from "../../engine/types";
import { useGameStore } from "../../state/gameStore";
import { useUiStore } from "../../state/uiStore";
import { ResourceIcon } from "../components/ResourceIcon";
import { UnitIcon } from "../components/UnitIcon";
import { formatNumber } from "../format";
import { useClock } from "../useClock";
import { humanVillageIds, isHumanVillage } from "../../engine/players";
import { canCancelCommand } from "../../engine/commands";
import {
  BattleSidePanel,
  DamageLines,
  LootLine,
  LuckBar,
  ReportCard,
  ReportSection,
  usedUnits,
} from "../report/battleReport";

type Filter = "all" | ReportType | "unread";

const fieldPlace = (r: Report) =>
  `${r.field!.x.toFixed(1)}|${r.field!.y.toFixed(1)}`;

/** Where a report's troops went: a village, or a place in the field when they found no army there. */
function targetLabel(r: Report): string {
  return r.field && !r.targetId
    ? t("reports.field.place", { at: fieldPlace(r) })
    : reportVillageLabel(r.target);
}

function resultText(r: Report): string {
  if (r.field?.missed) return t("reports.field.missed");
  if (r.type === "conquest") return t("reports.conquered");
  if (r.type === "scout" || r.type === "fieldScout")
    return r.outcome === "attackerWon"
      ? t("reports.scoutOk")
      : t("reports.scoutFail");
  return r.outcome === "attackerWon"
    ? t("reports.attackerWon")
    : t("reports.defenderWon");
}

function ReportDetail({ id, onBack }: { id: string; onBack: () => void }) {
  const clock = useClock();
  const json = useGameStore((s) =>
    JSON.stringify(s.game!.reports.find((r) => r.id === id) ?? null),
  );
  const r = JSON.parse(json) as Report | null;
  const openRally = useUiStore((s) => s.openRally);
  const openSimulator = useUiStore((s) => s.openSimulator);
  const compactUnits = useUiStore((s) => s.prefs.reportsCompactUnits);
  const setPrefs = useUiStore((s) => s.setPrefs);
  const sendTemplate = useGameStore((s) => s.sendTemplate);
  const deleteReport = useGameStore((s) => s.deleteReport);
  const villages = useGameStore((s) => s.game!.villages);
  const cancel = useGameStore((s) => s.cancelCmd);
  // The army this field report is about, when it is the player's own, was hit and may still turn back.
  const turnBackId = useGameStore((s) => {
    const g = s.game!;
    const rep = g.reports.find((x) => x.id === id);
    const c = rep?.field
      ? g.commands.find((x) => x.id === rep.field!.targetCommandId)
      : undefined;
    return c &&
      c.fieldHit &&
      humanVillageIds(g).includes(c.originId) &&
      canCancelCommand(g, c)
      ? c.id
      : null;
  });
  const t = useT();
  // The two sides' tables are wider than a phone, so each scrolls. Scrolling one scrolls the other, so the same unit
  // type stays under the same finger: the column you are looking at is the column you are looking at on both sides.
  const [scrollers] = useState(() => new Set<HTMLDivElement>());
  const keepScrollInStep = (el: HTMLDivElement) => {
    if (el.dataset.scrollWired) return;
    el.dataset.scrollWired = "1";
    el.addEventListener("scroll", () => {
      for (const other of scrollers) {
        if (other !== el && other.scrollLeft !== el.scrollLeft)
          other.scrollLeft = el.scrollLeft;
      }
    });
  };
  if (!r) return <p className="text-muted italic">{t("reports.missing")}</p>;
  const isField = r.type === "field" || r.type === "fieldScout";
  const target = villages[r.targetId];
  const tx = r.target.x;
  const ty = r.target.y;
  const canRetry =
    !isField &&
    tx !== undefined &&
    ty !== undefined &&
    !(target && isHumanVillage(target));
  // A battle report holds both armies, the Smithy levels they fought at and the wall it happened at, so the
  // simulator can replay it. A scout report holds what the scouts saw: the troops inside, the wall and the
  // buildings a catapult could aim at. A field report is a meeting in the open with no village to defend.
  //
  // A blinded side is the exception. Whoever was wiped out cannot say what beat him, so his copy of the report holds
  // no enemy army at all — replaying it would fight an empty village and answer a different battle, one the report
  // never described. There is nothing honest to fill in, so there is no button.
  const blinded = !!(r.attackerHidden || r.defenderHidden);
  const canSimulate = !isField && !blinded && (r.type !== "scout" || !!r.scout);
  const simulate = () => {
    if (r.type === "scout" && r.scout) {
      openSimulator({
        defender: r.scout.units,
        defenderBuildings: r.scout.buildings,
        wall: r.scout.buildings.wall,
      });
      return;
    }
    openSimulator({
      attacker: r.attacker.units,
      defender: r.defender.units,
      attackerResearch: r.attacker.research,
      defenderResearch: r.defender.research,
      wall: r.wallBefore,
      luck: r.luck,
      morale: r.morale,
      // A battle picks its catapult target at random, so a replay that guessed would hit another building. The
      // report names the one that was actually hit, and the level it stood at. Naming the building is not enough
      // on its own: a catapult only aims at a building the defender has, so without its level the simulator would
      // skip the named target and guess again.
      defenderBuildings: r.buildingDamage
        ? { [r.buildingDamage.building]: r.buildingDamage.from }
        : undefined,
      catapultTarget: r.buildingDamage?.building,
    });
  };
  const haulSum = r.haul ? r.haul.wood + r.haul.clay + r.haul.iron : 0;
  // The bar reads as won or lost for whoever is reading: a defender who watched his village fall must not see the
  // winner's green. A report always arrives on one side, so the reader is the side whose village is in it.
  const readAsDefender = r.target.ownerId === HUMAN_ID;
  const lost = readAsDefender
    ? r.outcome === "attackerWon"
    : r.outcome === "defenderWon";

  // The units this battle actually used, from the visible sides' troops and losses. A hidden side contributes
  // nothing — otherwise its column would still hint at what the enemy brought, which is the whole point of hiding it.
  const visibleSides = [
    r.attackerHidden ? null : r.attacker,
    r.defenderHidden ? null : r.defender,
  ].filter((side): side is Report["attacker"] => side !== null);
  const usedUnits = UNIT_IDS.filter((u) =>
    visibleSides.some(
      (side) => (side.units[u] ?? 0) > 0 || (side.losses[u] ?? 0) > 0,
    ),
  );
  const canNarrow = usedUnits.length > 0 && usedUnits.length < UNIT_IDS.length;
  const columns = compactUnits && canNarrow ? usedUnits : UNIT_IDS;

  return (
    <div className="space-y-3 text-[12px]">
      <div className="flex items-center gap-2 flex-wrap">
        <button className="btn" onClick={onBack}>
          {t("reports.back")}
        </button>
        <h3 className="text-[15px]">
          <PixelIcon name={reportIcon(r)} size={18} />{" "}
          {reportVillageLabel(r.origin)}
          {r.origin.ownerIsBot && <BotTag />} → {targetLabel(r)}
          {r.target.ownerIsBot && <BotTag />}
          {r.autoFarm && <AutoFarmTag />}
        </h3>
        <span className="text-muted ml-auto">{clock.at(r.time)}</span>
      </div>
      <div className="flex gap-2 flex-wrap">
        {canRetry && (
          <button
            className="btn"
            onClick={() =>
              openRally({
                x: tx!,
                y: ty!,
                units: r.attacker.units,
                mode: "attack",
              })
            }
          >
            {t("reports.retry")}
          </button>
        )}
        {canSimulate && (
          <button className="btn" onClick={simulate}>
            {t("sim.fromReport")}
          </button>
        )}
        {canRetry && (
          <button className="btn" onClick={() => sendTemplate("A", tx!, ty!)}>
            {t("reports.sendA")}
          </button>
        )}
        {turnBackId && (
          <button
            className="btn"
            onClick={() => {
              void cancel(turnBackId);
            }}
          >
            {t("armyCard.turnBack")}
          </button>
        )}
        {canNarrow && (
          <label className="inline-flex items-center gap-1">
            <input
              type="checkbox"
              checked={compactUnits}
              onChange={(e) =>
                setPrefs({ reportsCompactUnits: e.target.checked })
              }
            />
            {t("reports.onlyUsedUnits")}
          </label>
        )}
        <button
          className="btn ml-auto"
          onClick={() => {
            deleteReport(r.id);
            onBack();
          }}
        >
          {t("reports.delete")}
        </button>
      </div>
      {/* One card: red section bars inside a single frame. */}
      <ReportCard>
        <div
          className={`report-bar ${lost ? "panel-header-loss" : "report-bar-ok"}`}
        >
          {resultText(r)}
        </div>
        <div className="px-2 py-1.5 space-y-1">
          {r.field ? (
            <div>{t("reports.field.at", { at: fieldPlace(r) })}</div>
          ) : (
            <div>
              {t("reports.battleAt", { at: reportVillageLabel(r.target) })}
            </div>
          )}
          {r.type !== "scout" &&
            r.type !== "fieldScout" &&
            !r.field?.missed && <LuckBar luck={r.luck} />}
          {r.morale !== 1 && (
            <div>{t("reports.morale", { n: Math.round(r.morale * 100) })}</div>
          )}
        </div>
        <BattleSidePanel
          title={t("reports.attacker")}
          units={r.attacker.units}
          losses={r.attacker.losses}
          placeLabel={t("reports.origin")}
          village={reportVillageLabel(r.origin)}
          hidden={r.attackerHidden}
          scroller={(el) => {
            if (el) {
              scrollers.add(el);
              keepScrollInStep(el);
            }
          }}
          columns={columns}
        />
        {!r.field?.missed && (
          <BattleSidePanel
            title={t("reports.defender")}
            units={r.defender.units}
            losses={r.defender.losses}
            placeLabel={
              r.field ? t("reports.origin") : t("reports.destination")
            }
            village={reportVillageLabel(r.target)}
            hidden={r.defenderHidden}
            hiddenText={
              r.type === "scout" || r.type === "fieldScout"
                ? t("reports.scoutUnknown")
                : undefined
            }
            scroller={(el) => {
              if (el) {
                scrollers.add(el);
                keepScrollInStep(el);
              }
            }}
            columns={columns}
          />
        )}
        {/* What the attack actually did: loot, loyalty, and the damage done inside the village. Each is a full-width
            red label bar with its contents on the lines below, and wraps freely — no clipping, no sideways scrolling.
            Wall and building damage share one bar: both are damage to what the defender built. */}
        {(r.haul ||
          r.noHome ||
          r.loyaltyBefore !== undefined ||
          r.wallBefore !== r.wallAfter ||
          r.buildingDamage) && (
          <div className="pt-1.5">
            {r.haul && (
              <div className="border-t border-wood-border/40">
                <div className="report-bar">{t("reports.loot")}</div>
                <div className="px-2 py-1 flex flex-wrap items-center gap-x-2 gap-y-1">
                  {RESOURCES.map((res) => (
                    <span
                      key={res}
                      className="inline-flex items-center gap-0.5 tabular-nums"
                    >
                      <ResourceIcon r={res} size={14} />
                      {formatNumber(r.haul![res])}
                    </span>
                  ))}
                  <span className="text-muted">
                    {formatNumber(haulSum)} /{" "}
                    {formatNumber(r.haulCapacity ?? 0)}{" "}
                    {r.haulCapacity && haulSum >= r.haulCapacity
                      ? t("reports.fullLoad")
                      : t("reports.partialLoot")}
                  </span>
                </div>
              </div>
            )}
            {!r.haul && r.noHome && (
              <div className="border-t border-wood-border/40 px-2 py-1">
                {t("reports.noHome")}
              </div>
            )}
            {r.loyaltyBefore !== undefined && (
              <div className="border-t border-wood-border/40">
                <div className="report-bar">{t("reports.loyalty")}</div>
                <div className="px-2 py-1">
                  {t("reports.fromTo", {
                    from: formatNumber(r.loyaltyBefore),
                    to: formatNumber(r.loyaltyAfter ?? 0),
                  })}
                  {r.conquered && (
                    <strong className="text-ok">
                      {" "}
                      {t("reports.villageTaken")}
                    </strong>
                  )}
                </div>
              </div>
            )}
            {(r.wallBefore !== r.wallAfter || r.buildingDamage) && (
              <div className="border-t border-wood-border/40">
                <div className="report-bar">{t("reports.buildings")}</div>
                <div className="px-2 py-1 space-y-0.5">
                  {r.wallBefore !== r.wallAfter && (
                    <div>
                      {t("reports.wall")}{" "}
                      {t("reports.fromTo", {
                        from: formatNumber(r.wallBefore),
                        to: formatNumber(r.wallAfter),
                      })}
                    </div>
                  )}
                  {r.buildingDamage && (
                    <div>
                      {buildingName(r.buildingDamage.building)}:{" "}
                      {t("reports.fromTo", {
                        from: formatNumber(r.buildingDamage.from),
                        to: formatNumber(r.buildingDamage.to),
                      })}
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        )}
      </ReportCard>
      {r.scout && r.type === "fieldScout" && (
        <div className="panel p-2 flex flex-wrap gap-2">
          <span className="font-semibold">{t("reports.field.scouted")}</span>
          {UNIT_IDS.filter((u) => (r.scout!.units[u] ?? 0) > 0).map((u) => (
            <span key={u} className="inline-flex items-center gap-0.5">
              <UnitIcon unit={u} size={14} />
              {formatNumber(r.scout!.units[u] ?? 0)}
            </span>
          ))}
        </div>
      )}
      {r.scout && r.type !== "fieldScout" && (
        <div className="panel p-2 space-y-2">
          <div className="flex items-center gap-3 flex-wrap">
            <span className="font-semibold">{t("reports.resources")}</span>
            {RESOURCES.map((res) => (
              <span
                key={res}
                className="inline-flex items-center gap-0.5 tabular-nums"
              >
                <ResourceIcon r={res} size={14} />
                {formatNumber(r.scout!.resources[res])}
              </span>
            ))}
            <span className="ml-auto">
              {t("reports.loyalty")} <strong>{r.scout.loyalty}</strong>
            </span>
          </div>
          <div className="flex flex-wrap gap-x-3 gap-y-0.5">
            {BUILDING_ORDER.filter((b) => (r.scout!.buildings[b] ?? 0) > 0).map(
              (b) => (
                <span key={b}>
                  {buildingName(b)} <strong>{r.scout!.buildings[b]}</strong>
                </span>
              ),
            )}
          </div>
          <div className="flex flex-wrap gap-2">
            <span className="font-semibold">{t("reports.troops")}</span>
            {UNIT_IDS.filter((u) => (r.scout!.units[u] ?? 0) > 0).map((u) => (
              <span key={u} className="inline-flex items-center gap-0.5">
                <UnitIcon unit={u} size={14} />
                {formatNumber(r.scout!.units[u] ?? 0)}
              </span>
            ))}
            {UNIT_IDS.every((u) => !(r.scout!.units[u] ?? 0)) && (
              <span className="text-muted">{t("reports.none")}</span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

export function ReportsView() {
  const clock = useClock();
  const [filter, setFilter] = useState<Filter>("all");
  const [selected, setSelected] = useState<string | null>(null);
  const villageFilter = useUiStore((s) => s.reportsVillageFilter);
  const setVillageFilter = useUiStore((s) => s.setReportsVillageFilter);
  const hideAutoFarm = useUiStore((s) => s.reportsHideAutoFarm);
  const setHideAutoFarm = useUiStore((s) => s.setReportsHideAutoFarm);
  const villageFilterLabel = useGameStore((s) =>
    villageFilter
      ? reportVillageLabel(reportVillageOf(s.game!, villageFilter))
      : "",
  );
  const reportToOpen = useUiStore((s) => s.reportToOpen);
  const consumeReportToOpen = useUiStore((s) => s.consumeReportToOpen);
  const list = useGameStore((s) =>
    JSON.stringify(
      s
        .game!.reports.filter(
          (r) =>
            !villageFilter ||
            r.targetId === villageFilter ||
            r.originId === villageFilter,
        )
        .filter(
          (r) =>
            filter === "all" ||
            (filter === "unread" ? !r.read : r.type === filter),
        )
        .filter((r) => !hideAutoFarm || !r.autoFarm)
        .map((r) => ({
          id: r.id,
          time: r.time,
          type: r.type,
          read: r.read,
          autoFarm: !!r.autoFarm,
          icon: reportIcon(r),
          o: reportVillageLabel(r.origin),
          t: targetLabel(r),
          haul: r.haul ? r.haul.wood + r.haul.clay + r.haul.iron : null,
          cap: r.haulCapacity ?? null,
        }))
        .reverse(),
    ),
  );
  const rows = JSON.parse(list) as {
    id: string;
    time: number;
    type: ReportType;
    read: boolean;
    autoFarm: boolean;
    icon: string;
    o: string;
    t: string;
    haul: number | null;
    cap: number | null;
  }[];
  const markRead = useGameStore((s) => s.markReportRead);
  const markAll = useGameStore((s) => s.markAllReportsRead);
  const deleteRead = useGameStore((s) => s.deleteReadReports);
  const total = useGameStore((s) => s.game!.reports.length);
  const t = useT();

  useEffect(() => {
    const id = consumeReportToOpen();
    if (id) {
      markRead(id);
      setSelected(id);
    }
  }, [reportToOpen, consumeReportToOpen, markRead]);
  useEffect(() => () => setVillageFilter(null), [setVillageFilter]);

  if (selected)
    return <ReportDetail id={selected} onBack={() => setSelected(null)} />;

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-2 text-[12px]">
        <h2 className="text-lg mr-2">
          {t("reports.title")}{" "}
          <span className="text-muted font-body text-[12px]">
            ({total}/{COMBAT_BALANCE.maxReports})
          </span>
        </h2>
        <select
          className="px-1 border border-wood-border rounded bg-white/50"
          value={filter}
          onChange={(e) => setFilter(e.target.value as Filter)}
        >
          {(
            ["all", "unread", "attack", "scout", "conquest", "support"] as const
          ).map((f) => (
            <option key={f} value={f}>
              {t(`reports.filter.${f}`)}
            </option>
          ))}
        </select>
        <label className="inline-flex items-center gap-1">
          <input
            type="checkbox"
            checked={hideAutoFarm}
            onChange={(e) => setHideAutoFarm(e.target.checked)}
          />{" "}
          {t("reports.hideAutoFarm")}
        </label>
        {villageFilter && (
          <span className="inline-flex items-center gap-1 px-1.5 py-0.5 border border-wood-border rounded bg-white/40">
            {t("reports.villageFilter")} <strong>{villageFilterLabel}</strong>
            <button
              className="ml-1 font-bold"
              title={t("reports.allVillages")}
              onClick={() => setVillageFilter(null)}
            >
              ✕
            </button>
          </span>
        )}
        <button className="btn ml-auto" onClick={() => markAll(hideAutoFarm)}>
          {t("reports.markAll")}
        </button>
        <button className="btn" onClick={() => deleteRead(hideAutoFarm)}>
          {t("reports.deleteRead")}
        </button>
      </div>
      {rows.length === 0 ? (
        <p className="text-muted italic text-[12px]">
          {t(total > 0 ? "reports.noneMatch" : "reports.empty")}
        </p>
      ) : (
        <table className="table-game table-stack">
          <thead>
            <tr>
              <th></th>
              <th>{t("reports.colReport")}</th>
              <th>{t("reports.colLoot")}</th>
              <th>{t("reports.colTime")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr
                key={r.id}
                className={`cursor-pointer ${r.read ? "text-muted" : "font-semibold"}`}
                onClick={() => {
                  markRead(r.id);
                  setSelected(r.id);
                }}
              >
                <td className="text-center w-8 stack-inline">
                  <PixelIcon name={r.icon} size={16} />
                </td>
                <td className="stack-inline">
                  <button
                    type="button"
                    className="cursor-pointer text-left hover:underline focus:underline"
                  >
                    {r.o} → {r.t}
                  </button>
                  {r.autoFarm && <AutoFarmTag />}
                </td>
                <td className="tabular-nums" data-label={t("reports.colLoot")}>
                  {r.haul !== null
                    ? `${formatNumber(r.haul)}${r.cap ? ` / ${formatNumber(r.cap)}` : ""}`
                    : "—"}
                </td>
                <td
                  className="tabular-nums whitespace-nowrap"
                  data-label={t("reports.colTime")}
                >
                  {clock.at(r.time)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
