import type { ReactNode } from "react";
import {
  RESOURCES,
  UNIT_IDS,
  type BuildingId,
  type UnitId,
} from "../../engine/types";
import { t, useT } from "../../i18n/i18n";
import { buildingName } from "../../i18n/names";
import { ResourceIcon } from "../components/ResourceIcon";
import { UnitIcon } from "../components/UnitIcon";
import { formatNumber } from "../format";

/**
 * The pieces a battle report is drawn from, shared by the report itself and the simulator. A simulated battle has to
 * read exactly like a report of the same fight, so both render the same card out of the same parts; only where the
 * numbers come from differs.
 */

/** One side's army: a red section bar, the village it came from, and a table of fixed unit columns. */
export function BattleSidePanel({
  title,
  units,
  losses,
  placeLabel,
  village,
  hidden,
  hiddenText,
  columns,
  scroller,
}: {
  title: string;
  /** What the side brought. */
  units: Partial<Record<UnitId, number>>;
  /** What it lost. */
  losses: Partial<Record<UnitId, number>>;
  /** "Origin:" for the side that marched, "Destination:" for the side that was hit. */
  placeLabel: string;
  village: string;
  /** A side the reader was not told about shows no army: a wiped-out force has nobody left to describe what beat it. */
  hidden?: boolean;
  /** Words for a side the reader was not told about: a wiped-out army and a caught scouting party differ. */
  hiddenText?: string;
  /** The unit columns to draw, in order. Both sides get the same list so the tables stay aligned. */
  columns: readonly UnitId[];
  /** Told where the sideways scroller is, so the two sides' tables can be scrolled as one. */
  scroller?: (el: HTMLDivElement | null) => void;
}) {
  const t = useT();
  return (
    <div>
      <div className="report-bar">{title}</div>
      <div className="px-2 py-1 flex flex-wrap items-center gap-x-1.5">
        <span className="font-semibold">{placeLabel}</span>
        <span>{village}</span>
      </div>
      {hidden ? (
        <p className="px-2 pb-2 italic text-muted">
          {hiddenText ?? t("reports.armyUnknown")}
        </p>
      ) : (
        <div className="overflow-x-auto" ref={scroller}>
          <table
            className={`table-units ${columns.length < UNIT_IDS.length ? "table-units-compact" : ""}`}
          >
            <thead>
              <tr>
                <th />
                {columns.map((u) => (
                  <th key={u} className="text-center p-0">
                    <UnitIcon unit={u} />
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>{t("reports.quantity")}</td>
                {columns.map((u) => (
                  <td
                    key={u}
                    className={`text-center tabular-nums${(units[u] ?? 0) > 0 ? "" : " zero-figure"}`}
                  >
                    {formatNumber(units[u] ?? 0)}
                  </td>
                ))}
              </tr>
              <tr>
                <td>{t("reports.losses")}</td>
                {columns.map((u) => (
                  <td
                    key={u}
                    className={`text-center tabular-nums${(losses[u] ?? 0) > 0 ? " text-bad font-semibold" : " zero-figure"}`}
                  >
                    {formatNumber(losses[u] ?? 0)}
                  </td>
                ))}
              </tr>
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

/** Luck as a slider from −25 % to +25 %, with the value marked on it. */
export function LuckBar({ luck }: { luck: number }) {
  const pct = ((luck + 0.25) / 0.5) * 100;
  return (
    <div className="flex items-center gap-2 text-[12px]">
      <span className="text-muted">−25 %</span>
      <div className="relative h-3 w-48 border border-wood-border rounded-[2px] bg-gradient-to-r from-[#a33a2a] via-[#c8a54b] to-[#4c8f2f]">
        <div
          className="absolute top-[-3px] w-[2px] h-[16px] bg-ink"
          style={{ left: `calc(${pct}% - 1px)` }}
        />
      </div>
      <span className="text-muted">+25 %</span>
      <strong className={luck < 0 ? "text-bad" : "text-ok"}>
        {luck >= 0 ? "+" : ""}
        {(luck * 100).toFixed(1)} %
      </strong>
    </div>
  );
}

/**
 * The units a battle actually used, from the troops and losses of every side the reader can see. A kind nobody brought
 * gets no column, which is what lets the card fit a phone without scrolling sideways.
 */
export function usedUnits(sides: Partial<Record<UnitId, number>>[]): UnitId[] {
  return UNIT_IDS.filter((u) => sides.some((side) => (side[u] ?? 0) > 0));
}

/** A red label bar with its contents on the lines below, as the report's loot, loyalty and damage sections are. */
export function ReportSection({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="border-t border-wood-border/40">
      <div className="report-bar">{label}</div>
      <div className="px-2 py-1 space-y-0.5">{children}</div>
    </div>
  );
}

/** The loot line: one figure per resource, then how much of the load that is. */
export function LootLine({
  haul,
  capacity,
}: {
  haul: { wood: number; clay: number; iron: number };
  capacity: number;
}) {
  const sum = haul.wood + haul.clay + haul.iron;
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
      {RESOURCES.map((res) => (
        <span
          key={res}
          className="inline-flex items-center gap-0.5 tabular-nums"
        >
          <ResourceIcon r={res} size={14} />
          {formatNumber(haul[res])}
        </span>
      ))}
      <span className="text-muted">
        {formatNumber(sum)} / {formatNumber(capacity)}{" "}
        {capacity && sum >= capacity
          ? t("reports.fullLoad")
          : t("reports.partialLoot")}
      </span>
    </div>
  );
}

/** Wall and building damage: both are damage to what the defender built, so they share one bar. */
export function DamageLines({
  wallBefore,
  wallAfter,
  buildingDamage,
}: {
  wallBefore: number;
  wallAfter: number;
  buildingDamage?: { building: BuildingId; from: number; to: number };
}) {
  return (
    <>
      {wallBefore !== wallAfter && (
        <div>
          {t("reports.wall")}{" "}
          {t("reports.fromTo", {
            from: formatNumber(wallBefore),
            to: formatNumber(wallAfter),
          })}
        </div>
      )}
      {buildingDamage && (
        <div>
          {buildingName(buildingDamage.building)}:{" "}
          {t("reports.fromTo", {
            from: formatNumber(buildingDamage.from),
            to: formatNumber(buildingDamage.to),
          })}
        </div>
      )}
    </>
  );
}

/** The card a battle is drawn in: one bordered frame with red section bars inside it. */
export function ReportCard({ children }: { children: ReactNode }) {
  return <div className="report-card">{children}</div>;
}
