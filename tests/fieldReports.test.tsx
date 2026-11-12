// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { setLocale } from "../src/i18n/i18n";
import { reportVillageOf } from "../src/engine/reports";
import {
  UNIT_IDS,
  type GameState,
  type Report,
  type Village,
} from "../src/engine/types";
import { useGameStore } from "../src/state/gameStore";
import { useUiStore } from "../src/state/uiStore";
import { ReportsView } from "../src/ui/views/ReportsView";
import {
  activeVillage,
  makeBotVillage,
  newTestGame,
  stateFixture,
} from "./helpers";
import { stubServer } from "./serverStub";

const HOUR = 3_600_000;
let g: GameState;
let home: Village;
let bot: Village;
let cancels: unknown[];

function fieldReport(over: Partial<Report>): Report {
  return {
    id: `r${g.reports.length + 1}`,
    time: g.now,
    type: "field",
    originId: home.id,
    targetId: bot.id,
    origin: reportVillageOf(g, home.id),
    target: reportVillageOf(g, bot.id),
    attacker: { units: { axe: 10 }, losses: { axe: 3 } },
    defender: { units: { lightCav: 50 }, losses: { lightCav: 50 } },
    luck: 0.05,
    morale: 1,
    wallBefore: 0,
    wallAfter: 0,
    outcome: "attackerWon",
    read: false,
    field: { x: 12.4, y: 7.6, targetType: "attack", targetCommandId: "own" },
    ...over,
  };
}

async function open(r: Report) {
  g.reports.push(r);
  stubServer(g, {
    routes: {
      "POST /api/v1/worlds/1/orders/cancel-command": (b) => {
        cancels.push(b);
        return { status: 200, body: stateFixture(g) };
      },
    },
  });
  await useGameStore.getState().boot();
  render(<ReportsView />);
  fireEvent.click(await screen.findByText(/→/));
}

beforeEach(() => {
  setLocale("en");
  g = newTestGame();
  home = activeVillage(g);
  bot = makeBotVillage(g);
  cancels = [];
  useUiStore.setState({ view: "reports" });
});
afterEach(() => {
  cleanup();
  useGameStore.getState().shutdown();
  vi.unstubAllGlobals();
});

describe("field battle reports", () => {
  it("a field battle shows both sides, the meeting place and Turn back for a hit army still marching", async () => {
    g.commands.push({
      id: "own",
      type: "attack",
      originId: home.id,
      targetId: bot.id,
      units: { axe: 7 },
      fieldHit: true,
      departedAt: g.now - HOUR,
      arrivesAt: g.now + HOUR,
    });
    await open(fieldReport({}));
    expect(screen.getByText(/Field battle at 12.4\|7.6/)).toBeTruthy();
    expect(screen.getByText("Attacker")).toBeTruthy();
    expect(screen.getByText("Defender")).toBeTruthy();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Turn back" }));
    });
    expect(cancels).toEqual([{ commandId: "own" }]);
  });

  it("no Turn back once the army is no longer marching", async () => {
    await open(fieldReport({}));
    expect(screen.queryByRole("button", { name: "Turn back" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Repeat the attack (same troops)" }),
    ).toBeNull();
  });

  it("a field scouting report lists the scouted units", async () => {
    await open(
      fieldReport({
        type: "fieldScout",
        attacker: { units: { scout: 3 }, losses: {} },
        defender: { units: { axe: 120 }, losses: {} },
        scout: { units: { axe: 120 } } as Report["scout"],
      }),
    );
    expect(screen.getByText(/Scouted army/)).toBeTruthy();
    expect(document.body.textContent).toContain("120");
  });

  it("shows the army with icons across the top, and Quantity and Losses rows", async () => {
    await open(fieldReport({}));
    // One table per side: a header row of unit icons, then a Quantity row and a Losses row.
    const tables = document.querySelectorAll("table.table-units");
    expect(tables).toHaveLength(2);
    for (const table of tables) {
      // The blank label column comes first, then one column per unit — all twelve by default, so the
      // attacker's and defender's tables line up column for column.
      expect(table.querySelectorAll("thead th")).toHaveLength(
        UNIT_IDS.length + 1,
      );
      expect(table.querySelectorAll("tbody tr")).toHaveLength(2);
    }
    // The attacker brought axes only, and the defender light cavalry only, yet every unit still has a column.
    expect(screen.getAllByRole("img", { name: "Spearman" })).toHaveLength(2);
    expect(screen.getAllByRole("img", { name: "Hetman" })).toHaveLength(2);
    // The row labels appear once per side.
    expect(screen.getAllByText("Quantity:")).toHaveLength(2);
    expect(screen.getAllByText("Losses")).toHaveLength(2);
    expect(screen.getAllByText("Origin:").length).toBeGreaterThanOrEqual(1);
  });

  it("narrows to the units the battle used, so a phone need not scroll sideways", async () => {
    await open(fieldReport({}));
    const box = screen.getByRole("checkbox", {
      name: "Only units in this battle",
    });
    expect((box as HTMLInputElement).checked).toBe(false);
    // The attacker used axes, the defender light cavalry: two columns plus the label column.
    fireEvent.click(box);

    const tables = document.querySelectorAll("table.table-units");
    expect(tables).toHaveLength(2);
    for (const table of tables) {
      expect(table.querySelectorAll("thead th")).toHaveLength(3);
      // Both sides show the same two units, so they still line up column for column.
      expect(screen.getAllByRole("img", { name: "Axeman" })).toHaveLength(2);
      expect(
        screen.getAllByRole("img", { name: "Light cavalryman" }),
      ).toHaveLength(2);
      expect(table.classList.contains("table-units-compact")).toBe(true);
    }
    // A unit nobody brought is gone entirely.
    expect(screen.queryByRole("img", { name: "Spearman" })).toBeNull();
    // Unchecking brings all twelve back.
    fireEvent.click(box);
    expect(
      document.querySelectorAll("table.table-units thead th"),
    ).toHaveLength((UNIT_IDS.length + 1) * 2);
  });

  it("dims the zeroes, so the numbers that matter stand out", async () => {
    await open(fieldReport({}));
    const tables = [...document.querySelectorAll("table.table-units")];
    expect(tables).toHaveLength(2);
    for (const table of tables) {
      const [quantity, losses] = [...table.querySelectorAll("tbody tr")].map(
        (tr) => [...tr.querySelectorAll("td")].slice(1),
      );
      // Every figure still reads its number; only the zeroes are dimmed.
      for (const cell of [...quantity!, ...losses!])
        expect(cell.textContent).toMatch(/^\d[\d,]*$/);
      expect(
        quantity!.filter((c) => c.classList.contains("zero-figure")),
      ).toHaveLength(UNIT_IDS.length - 1);
      expect(
        losses!.filter((c) => c.classList.contains("zero-figure")),
      ).toHaveLength(UNIT_IDS.length - 1);
    }
    // The attacker's ten axes and the defender's fifty light cavalry stay in full ink.
    const axe = UNIT_IDS.indexOf("axe");
    const attackerQuantity = [
      ...tables[0]!.querySelectorAll("tbody tr")[0]!.querySelectorAll("td"),
    ].slice(1);
    expect(attackerQuantity[axe]!.textContent).toBe("10");
    expect(attackerQuantity[axe]!.classList.contains("zero-figure")).toBe(
      false,
    );
  });

  it("offers no narrowing when the battle used every unit type", async () => {
    const all = Object.fromEntries(
      UNIT_IDS.map((u) => [u, 1]),
    ) as Report["attacker"]["units"];
    await open(
      fieldReport({
        attacker: { units: all, losses: {} },
        defender: { units: all, losses: {} },
      }),
    );
    expect(
      screen.queryByRole("checkbox", { name: "Only units in this battle" }),
    ).toBeNull();
  });

  it("scrolls both sides together, so the same unit stays under the same column", async () => {
    await open(fieldReport({}));
    const sides = [...document.querySelectorAll("table.table-units")].map(
      (t) => t.parentElement as HTMLDivElement,
    );
    const [attacker, defender] = sides;
    expect(attacker).toBeDefined();
    expect(defender).toBeDefined();
    // Scroll the attacker's table sideways, as a thumb on a phone would.
    attacker!.scrollLeft = 120;
    fireEvent.scroll(attacker!);
    // The defender's table follows, so both sides still show the same unit types.
    expect(defender!.scrollLeft).toBe(120);
    // And the other way round.
    defender!.scrollLeft = 40;
    fireEvent.scroll(defender!);
    expect(attacker!.scrollLeft).toBe(40);
  });

  it("a wiped-out side is not told what killed it", async () => {
    // The defender lost every man, so his report carries no attacker numbers.
    await open(
      fieldReport({
        attackerHidden: true,
        defender: { units: { lightCav: 10 }, losses: { lightCav: 10 } },
      }),
    );
    expect(
      screen.getByText(/nobody came back to count the enemy/),
    ).toBeTruthy();
    // The enemy's numbers must not leak anywhere in the report body.
    expect(document.body.textContent).not.toContain("50");
  });

  it("the surviving side still sees the whole battle", async () => {
    await open(fieldReport({}));
    expect(
      screen.queryByText(/nobody came back to count the enemy/),
    ).toBeNull();
    expect(document.body.textContent).toContain("50");
  });

  it("a missed interception says the army was gone", async () => {
    await open(
      fieldReport({
        outcome: "defenderWon",
        target: { id: "", name: "", x: 0, y: 0 },
        targetId: "",
        defender: { units: {}, losses: {} },
        field: {
          x: 12.4,
          y: 7.6,
          targetType: "attack",
          targetCommandId: "gone",
          missed: true,
        },
      }),
    );
    expect(screen.getByText(/army was gone/)).toBeTruthy();
    expect(document.body.textContent).not.toContain("(0|0)");
  });

  it("a hidden side does not name its units in the narrowed columns", async () => {
    // The attacker brought catapults, but the reader was wiped out and may not see them. The narrowed table must
    // only offer the units of the side the reader can actually see.
    await open(
      fieldReport({
        attackerHidden: true,
        attacker: { units: { catapult: 2 }, losses: { catapult: 2 } },
        defender: { units: { axe: 5 }, losses: {} },
      }),
    );
    fireEvent.click(
      screen.getByRole("checkbox", { name: "Only units in this battle" }),
    );
    expect(screen.queryByRole("img", { name: "Catapult" })).toBeNull();
    expect(screen.getAllByRole("img", { name: "Axeman" })).toHaveLength(1);
  });

  it('keeps the "only units in this battle" choice in the saved preferences', async () => {
    // The choice lives in UiPrefs, so it survives a reload instead of resetting to the phone default.
    useUiStore.getState().setPrefs({ reportsCompactUnits: true });
    await open(fieldReport({}));
    const box = screen.getByRole("checkbox", {
      name: "Only units in this battle",
    }) as HTMLInputElement;
    expect(box.checked).toBe(true);
    expect(
      document.querySelectorAll("table.table-units thead th"),
    ).toHaveLength(3 * 2);
    useUiStore.getState().setPrefs({ reportsCompactUnits: false });
  });

  it("colours the result bar by the reader: a defender who lost sees the loss colour", async () => {
    // Same report read by the defender: the attacker won, which is a green bar for the attacker and a red one here.
    await open(
      fieldReport({
        outcome: "attackerWon",
        target: { ...reportVillageOf(g, bot.id), ownerId: "human" },
      }),
    );
    const bar = document.querySelector(".report-bar")!;
    expect(bar.classList.contains("panel-header-loss")).toBe(true);
    expect(bar.classList.contains("report-bar-ok")).toBe(false);
  });

  it("keeps the space between the loot total and the full-load note", async () => {
    await open(
      fieldReport({
        haul: { wood: 1000, clay: 0, iron: 0 },
        haulCapacity: 1000,
      }),
    );
    expect(document.body.textContent).toContain("1,000 / 1,000 — full load");
  });

  it("caught scouts are told nothing of the garrison, in words that fit scouting", async () => {
    // A scouting mission that was caught is not an army wiped out on a battlefield, so it must not talk about one.
    await open(
      fieldReport({
        type: "scout",
        outcome: "defenderWon",
        defenderHidden: true,
        attacker: { units: { scout: 3 }, losses: { scout: 3 } },
        defender: { units: {}, losses: {} },
        scout: undefined,
        field: undefined,
      }),
    );
    expect(screen.getByText(/Your scouts were caught/)).toBeTruthy();
    expect(document.body.textContent).not.toContain(
      "nobody came back to count the enemy",
    );
    // The sender still reads what he lost.
    expect(document.body.textContent).toContain("Scouting failed");
  });

  it("a scouting mission that got through shows the garrison", async () => {
    await open(
      fieldReport({
        type: "scout",
        outcome: "attackerWon",
        attacker: { units: { scout: 400 }, losses: {} },
        defender: { units: { spear: 120 }, losses: {} },
        scout: {
          units: { spear: 120 },
          resources: { wood: 100, clay: 100, iron: 100 },
          buildings: {},
          loyalty: 100,
        } as Report["scout"],
        field: undefined,
      }),
    );
    expect(screen.getByText(/Scouting succeeded/)).toBeTruthy();
    expect(document.body.textContent).toContain("120");
    expect(screen.queryByText(/Your scouts were caught/)).toBeNull();
  });
});
