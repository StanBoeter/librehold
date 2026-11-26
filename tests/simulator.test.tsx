// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { SimulatorTab } from "../src/ui/views/SimulatorView";
import { RallyPointView } from "../src/ui/views/RallyPointView";
import { ReportsView } from "../src/ui/views/ReportsView";
import { emptyBuildings, UNIT_IDS } from "../src/engine/types";
import { BUILDINGS, BUILDING_ORDER } from "../src/data/buildings.data";
import { useGameStore } from "../src/state/gameStore";
import { useUiStore } from "../src/state/uiStore";
import { setLocale } from "../src/i18n/i18n";
import { newDefaultGame } from "./helpers";
import { stubServer } from "./serverStub";

/** The battle the stub answers with, shaped like the server's own SimResult. */
const ANSWER = {
  kind: "battle",
  outcome: "attackerWon",
  attackPower: 4200,
  defensePower: 1800,
  luck: 0,
  morale: 1,
  attackerLosses: { sword: 120 },
  attackerSurvivors: { sword: 380 },
  defenderLosses: { spear: 200 },
  defenderSurvivors: {},
  wallBefore: 5,
  wallAfter: 3,
};

/** The answer to a scouting mission, shaped like the server's own resolveScout result. */
const SCOUT_ANSWER = {
  kind: "scout",
  outcome: "defenderWon",
  attackPower: 10,
  defensePower: 40,
  luck: 0,
  morale: 1,
  attackerLosses: { scout: 10 },
  attackerSurvivors: {},
  defenderLosses: {},
  defenderSurvivors: { scout: 20 },
  wallBefore: 10,
  wallAfter: 10,
};

async function boot(
  routes: Record<string, () => { status: number; body?: unknown }> = {},
  edit: (g: ReturnType<typeof newDefaultGame>) => void = () => {},
) {
  useGameStore.getState().shutdown();
  const g = newDefaultGame();
  edit(g);
  const paths = stubServer(g, { routes });
  await useGameStore.getState().boot();
  return { g, paths };
}

beforeEach(() => {
  setLocale("en");
  useUiStore.setState({ simPrefill: null, rallyPrefill: null });
});
afterEach(() => {
  cleanup();
  useGameStore.getState().shutdown();
  vi.unstubAllGlobals();
  setLocale("pl");
});

describe("the battle simulator", () => {
  it("is reachable as a tab of the rally point", async () => {
    await boot();
    render(<RallyPointView />);
    expect(screen.getByRole("button", { name: "Simulator" })).toBeTruthy();
  });

  it("asks the server and shows who won and what each side lost", async () => {
    const { paths } = await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    render(<SimulatorTab />);
    // An army to send: two swordsmen is enough for the stub to be asked.
    fireInput(atk("sword"), "500");
    screen.getByRole("button", { name: "Simulate" }).click();

    expect(await screen.findByText("The attacker wins")).toBeTruthy();
    expect(paths.some((p) => p.includes("/simulate"))).toBe(true);
    // Drawn as a battle report: the same two army tables, a row of what each side brought and a row of its losses.
    expect(screen.getAllByText("Losses").length).toBeGreaterThan(0);
    expect(screen.getByText("120")).toBeTruthy();
    // The card is a report and nothing more: every bar in it is one a real report has, and there is no extra
    // section a report never shows.
    const card = document.querySelector(".report-card")!;
    const bars = [...card.querySelectorAll(".report-bar")].map((b) =>
      (b.textContent ?? "").trim(),
    );
    for (const bar of bars) {
      expect(bar).not.toMatch(/Worth lost|Value lost|cost/i);
    }
  });

  it("says a scouting mission was a scouting mission, not a battle", async () => {
    await boot({
      "POST /api/v1/worlds/1/simulate": () => ({
        status: 200,
        body: SCOUT_ANSWER,
      }),
    });
    render(<SimulatorTab />);
    // Scouts alone are a scouting mission, which the server resolves by a rule of its own.
    fireInput(atk("scout"), "10");
    screen.getByRole("button", { name: "Simulate" }).click();
    expect(await screen.findByText("Scouting failed")).toBeTruthy();
    expect(screen.getByText("Simulated scouting")).toBeTruthy();
    // The battle's own numbers would describe a fight that never happened: scouts alone ignore luck and the wall.
    expect(screen.queryByText(/Attack power/)).toBe(null);
    expect(screen.queryByRole("slider")).toBe(null);
  });

  it("does not offer the wall, luck or catapults when only scouts are sent", async () => {
    await boot();
    render(<SimulatorTab />);
    fireInput(atk("scout"), "10");
    expect(document.getElementById("sim-wall")).toBe(null);
    expect(document.getElementById("sim-luck")).toBe(null);
    expect(document.getElementById("sim-catapult")).toBe(null);
    // Catapults would not be a scout mission at all, so they are the wrong thing to reach for here.
    // One swordsman makes it an ordinary attack again, where the wall and luck matter.
    fireInput(atk("sword"), "1");
    expect(document.getElementById("sim-wall")).not.toBe(null);
    expect(document.getElementById("sim-luck")).not.toBe(null);
    // Catapults are a separate question: without them there is still nothing to aim at.
    expect(document.getElementById("sim-catapult")).toBe(null);
    fireInput(atk("catapult"), "5");
    expect(document.getElementById("sim-catapult")).not.toBe(null);
  });

  it("draws every unit column unless the reports were asked to narrow them", async () => {
    await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    // Off, as it is on a wide screen: a report then shows every unit, so the simulator has to as well.
    useUiStore.setState({
      prefs: { ...useUiStore.getState().prefs, reportsCompactUnits: false },
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    screen.getByRole("button", { name: "Simulate" }).click();
    await screen.findByText("The attacker wins");
    expect(document.querySelector("table.table-units-compact")).toBe(null);
    // Two tables, one per side, each with a spacer column and all twelve units.
    const tables = document.querySelectorAll("table.table-units");
    expect(tables.length).toBe(2);
    for (const t of tables) {
      expect(t.querySelectorAll("thead th").length).toBe(13);
    }

    // On, and the card narrows to the units the battle used — the report's own rule, from the same pref.
    act(() => {
      useUiStore.setState({
        prefs: { ...useUiStore.getState().prefs, reportsCompactUnits: true },
      });
    });
    const compact = document.querySelector("table.table-units-compact");
    expect(compact).not.toBe(null);
    // A swordsman and a spearman, and nobody else.
    expect(compact!.querySelectorAll("thead th").length).toBe(3);
  });

  it("keeps the two sides' tables scrolled together, as a report does", async () => {
    await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    screen.getByRole("button", { name: "Simulate" }).click();
    await screen.findByText("The attacker wins");
    // Each side scrolls sideways on a phone, so scrolling one has to scroll the other: the same unit type has to stay
    // under the same finger on both.
    const scrollers = [
      ...document.querySelectorAll("[data-sim-result] .overflow-x-auto"),
    ] as HTMLDivElement[];
    expect(scrollers.length).toBe(2);
    const [first, second] = scrollers;
    expect(first && second).toBeTruthy();
    first!.scrollLeft = 120;
    first!.dispatchEvent(new Event("scroll"));
    expect(second!.scrollLeft).toBe(120);
  });

  it("shows a wall that fell", async () => {
    await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    screen.getByRole("button", { name: "Simulate" }).click();
    // The wall and the building it hit share the report's "Buildings" bar.
    expect(await screen.findByText("Buildings:")).toBeTruthy();
    expect(screen.getByText(/from 5 to 3/)).toBeTruthy();
  });

  it("does not ask the server before there are troops", async () => {
    const { paths } = await boot();
    render(<SimulatorTab />);
    const run = screen.getByRole("button", {
      name: "Simulate",
    }) as HTMLButtonElement;
    expect(run.disabled).toBe(true);
    expect(paths.filter((p) => p.includes("/simulate")).length).toBe(0);
  });

  it("fills the attacker in from the village the player is looking at", async () => {
    await boot({}, (g) => {
      g.villages[g.activeVillageId]!.units.sword = 77;
    });
    render(<SimulatorTab />);
    screen.getByRole("button", { name: "Fill in my troops" }).click();
    // The village is either side of the battle, so the button asks which.
    fireEvent.click(await screen.findByRole("button", { name: "As attacker" }));
    await waitFor(() => {
      expect((atk("sword") as HTMLInputElement).value).toBe("77");
    });
  });

  it("fills the defender in, with its Smithy levels and its wall, when that is the side chosen", async () => {
    const { paths } = await boot(
      {
        "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
      },
      (g) => {
        const v = g.villages[g.activeVillageId]!;
        v.units = { ...v.units, spear: 240, archer: 60 };
        v.research = { ...v.research, spear: 2, archer: 3 };
        v.buildings = { ...v.buildings, wall: 11 };
      },
    );
    render(<SimulatorTab />);
    screen.getByRole("button", { name: "Fill in my troops" }).click();
    fireEvent.click(await screen.findByRole("button", { name: "As defender" }));
    await waitFor(() => {
      expect(
        (document.getElementById("sim-def-spear") as HTMLInputElement).value,
      ).toBe("240");
    });
    expect(
      (document.getElementById("sim-def-archer") as HTMLInputElement).value,
    ).toBe("60");
    // The Smithy levels of this village, on the side it now holds.
    expect(
      (document.getElementById("sim-def-lvl-spear") as HTMLSelectElement).value,
    ).toBe("2");
    expect(
      (document.getElementById("sim-def-lvl-archer") as HTMLSelectElement)
        .value,
    ).toBe("3");
    // And the wall it would be defended behind.
    expect(
      (document.getElementById("sim-wall") as HTMLInputElement).value,
    ).toBe("11");
    // The attacker is left alone: it is somebody else's army, not this village's.
    expect((atk("sword") as HTMLInputElement).value).toBe("0");

    screen.getByRole("button", { name: "Simulate" }).click();
    await waitFor(() =>
      expect(paths.some((p) => p.includes("/simulate"))).toBe(true),
    );
    const sent = JSON.parse(
      String(
        (
          fetch as unknown as { mock: { calls: [string, RequestInit][] } }
        ).mock.calls.find(([url]) => url.includes("/simulate"))![1].body,
      ),
    ) as {
      defender: {
        units: Record<string, number>;
        research: Record<string, number>;
        wall: number;
      };
      attacker: { units: Record<string, number> };
    };
    expect(sent.defender.units.spear).toBe(240);
    expect(sent.defender.research.spear).toBe(2);
    expect(sent.defender.wall).toBe(11);
    expect(sent.attacker.units.sword ?? 0).toBe(0);
  });

  it("fills the attacker's Smithy levels in from the same village", async () => {
    const { paths } = await boot(
      {
        "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
      },
      (g) => {
        const v = g.villages[g.activeVillageId]!;
        v.units = { ...v.units, sword: 77, axe: 10 };
        // Level 3 in the Smithy, and a locked unit beside it: the levels are per village and per unit.
        v.research = { ...v.research, sword: 3, axe: 0 };
      },
    );
    render(<SimulatorTab />);
    screen.getByRole("button", { name: "Fill in my troops" }).click();
    fireEvent.click(await screen.findByRole("button", { name: "As attacker" }));
    await waitFor(() => {
      expect(
        (document.getElementById("sim-atk-lvl-sword") as HTMLSelectElement)
          .value,
      ).toBe("3");
    });
    // The defender is somebody else's village: nothing about the player's own levels belongs there.
    expect(
      (document.getElementById("sim-def-lvl-sword") as HTMLSelectElement).value,
    ).toBe("1");

    screen.getByRole("button", { name: "Simulate" }).click();
    await waitFor(() =>
      expect(paths.some((p) => p.includes("/simulate"))).toBe(true),
    );
    const sent = JSON.parse(
      String(
        (
          fetch as unknown as { mock: { calls: [string, RequestInit][] } }
        ).mock.calls.find(([url]) => url.includes("/simulate"))![1].body,
      ),
    ) as {
      attacker: { research: Record<string, number> };
      defender: { research: Record<string, number> };
    };
    expect(sent.attacker.research.sword).toBe(3);
    // A unit the village never unlocked is sent as level 1 — the Smithy's floor — never as the locked 0.
    expect(sent.attacker.research.axe).toBe(1);
    expect(sent.defender.research.sword).toBe(1);
  });

  it("fills the defender in from the report as level 3 as well", async () => {
    await boot();
    useUiStore.getState().openSimulator({
      attacker: { sword: 400 },
      defender: { spear: 200 },
      attackerResearch: { sword: 2 },
      defenderResearch: { spear: 3 },
      wall: 6,
    });
    render(<SimulatorTab />);
    await waitFor(() => {
      expect(
        (document.getElementById("sim-atk-lvl-sword") as HTMLSelectElement)
          .value,
      ).toBe("2");
    });
    expect(
      (document.getElementById("sim-def-lvl-spear") as HTMLSelectElement).value,
    ).toBe("3");
  });

  it("aims the catapults at the building a report says was hit", async () => {
    await boot();
    useUiStore.getState().openSimulator({
      attacker: { sword: 500, catapult: 30 },
      defender: { spear: 100 },
      wall: 12,
      catapultTarget: "barracks",
      defenderBuildings: { barracks: 4 },
    });
    render(<SimulatorTab />);
    // The report carried catapults, so the target it names is offered.
    await waitFor(() => {
      expect(
        (document.getElementById("sim-catapult") as HTMLSelectElement).value,
      ).toBe("barracks");
    });
    // The level the building stood at comes with the report, so the replay shows the damage it really took.
    expect(
      (document.getElementById("sim-catapult-level") as HTMLInputElement).value,
    ).toBe("4");
  });

  it("takes a battle handed to it from a report, once", async () => {
    await boot();
    useUiStore.getState().openSimulator({
      attacker: { axe: 300 },
      defender: { spear: 150 },
      wall: 7,
    });
    render(<SimulatorTab />);
    await waitFor(() => {
      expect((atk("axe") as HTMLInputElement).value).toBe("300");
    });
    expect(
      (document.getElementById("sim-def-spear") as HTMLInputElement).value,
    ).toBe("150");
    expect(
      (screen.getByLabelText("Wall level") as HTMLInputElement).value,
    ).toBe("7");
    // One-off: the tab does not reset itself on the next render.
    expect(useUiStore.getState().simPrefill).toBe(null);
  });

  it("says so when the server refuses", async () => {
    await boot({
      "POST /api/v1/worlds/1/simulate": () => ({
        status: 500,
        body: { code: "boom" },
      }),
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "10");
    screen.getByRole("button", { name: "Simulate" }).click();
    expect(
      await screen.findByText(/The simulation did not answer/),
    ).toBeTruthy();
  });
});

/** The attacker's field for a unit — each army has one, so the id picks the side. */
function atk(unit: string): HTMLElement {
  return document.getElementById(`sim-atk-${unit}`)!;
}

/** Typing into a NumberInput: it reads the value off the event, so a plain change does it. */
function fireInput(el: HTMLElement, value: string) {
  const input = el as HTMLInputElement;
  input.focus();
  const setter = Object.getOwnPropertyDescriptor(
    window.HTMLInputElement.prototype,
    "value",
  )!.set!;
  setter.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("the button on a battle report", () => {
  it("hands the battle to the simulator, both armies and the wall it was fought at", async () => {
    // The report has to be in the game the store loads, not in a copy made afterwards.
    await boot({}, (g) => {
      const home = g.villages[g.activeVillageId]!;
      g.reports.push({
        id: "r1",
        time: g.now,
        type: "attack",
        originId: home.id,
        targetId: "v:9:9",
        origin: {
          id: home.id,
          name: "Home",
          x: home.x,
          y: home.y,
          ownerId: home.ownerId ?? undefined,
        },
        target: { id: "v:9:9", name: "Barbarians", x: 9, y: 9 },
        attacker: {
          units: { axe: 40 },
          losses: { axe: 5 },
          research: { axe: 3 },
        },
        defender: {
          units: { spear: 20 },
          losses: { spear: 20 },
          research: { spear: 2 },
        },
        luck: 0.1,
        morale: 1,
        wallBefore: 6,
        wallAfter: 4,
        buildingDamage: { building: "wall", from: 6, to: 4 },
        outcome: "attackerWon",
        read: false,
      });
    });
    render(<ReportsView />);
    fireEvent.click(await screen.findByText(/→/));
    fireEvent.click(screen.getByRole("button", { name: "Fill in simulator" }));

    const handed = useUiStore.getState().simPrefill!;
    expect(handed.attacker).toEqual({ axe: 40 });
    expect(handed.defender).toEqual({ spear: 20 });
    // The levels the battle was really fought at, from both sides: replaying it at level 1 is a different battle.
    expect(handed.attackerResearch).toEqual({ axe: 3 });
    expect(handed.defenderResearch).toEqual({ spear: 2 });
    expect(handed.wall).toBe(6);
    expect(handed.luck).toBeCloseTo(0.1);
    // A battle picks its catapult target at random, so the replay has to be told which building was hit; guessing
    // sieges a building the report never mentions.
    expect(handed.catapultTarget).toBe("wall");
    expect(useUiStore.getState().view).toBe("rally");
  });

  it("offers a scout report, and hands over what the scouts saw", async () => {
    await boot({}, (g) => {
      const home = g.villages[g.activeVillageId]!;
      g.reports.push({
        id: "r2",
        time: g.now,
        type: "scout",
        originId: home.id,
        targetId: "v:9:9",
        origin: {
          id: home.id,
          name: "Home",
          x: home.x,
          y: home.y,
          ownerId: home.ownerId ?? undefined,
        },
        target: { id: "v:9:9", name: "Barbarians", x: 9, y: 9 },
        attacker: { units: { scout: 5 }, losses: {} },
        defender: { units: {}, losses: {} },
        scout: {
          resources: { wood: 100, clay: 200, iron: 300 },
          buildings: { ...emptyBuildings(), main: 7, wall: 8, barracks: 4 },
          units: { spear: 90, archer: 40 },
          loyalty: 80,
        },
        luck: 0,
        morale: 1,
        wallBefore: 8,
        wallAfter: 8,
        outcome: "attackerWon",
        read: false,
      });
    });
    render(<ReportsView />);
    fireEvent.click(await screen.findByText(/→/));
    fireEvent.click(screen.getByRole("button", { name: "Fill in simulator" }));

    const handed = useUiStore.getState().simPrefill!;
    // The defenders the scouts saw, and the wall they stand behind.
    expect(handed.defender).toEqual({ spear: 90, archer: 40 });
    expect(handed.wall).toBe(8);
    // The buildings a catapult could break: without them the server guesses a town hall and every run shows the same.
    expect(handed.defenderBuildings).toMatchObject({
      main: 7,
      wall: 8,
      barracks: 4,
    });
    expect(handed.attacker).toBe(undefined);
    expect(useUiStore.getState().view).toBe("rally");
  });

  it("offers nothing on a field report, which has no village to defend", async () => {
    await boot({}, (g) => {
      const home = g.villages[g.activeVillageId]!;
      g.reports.push({
        id: "r3",
        time: g.now,
        type: "field",
        originId: home.id,
        targetId: "v:9:9",
        origin: {
          id: home.id,
          name: "Home",
          x: home.x,
          y: home.y,
          ownerId: home.ownerId ?? undefined,
        },
        target: { id: "v:9:9", name: "Barbarians", x: 9, y: 9 },
        attacker: { units: { axe: 10 }, losses: {} },
        defender: { units: { spear: 4 }, losses: { spear: 4 } },
        luck: 0,
        morale: 1,
        wallBefore: 0,
        wallAfter: 0,
        outcome: "attackerWon",
        read: false,
        field: { x: 9, y: 9, targetType: "attack", targetCommandId: "c1" },
      });
    });
    render(<ReportsView />);
    fireEvent.click(await screen.findByText(/→/));
    expect(screen.queryByRole("button", { name: "Fill in simulator" })).toBe(
      null,
    );
  });
});

describe("a report the reader was blinded in", () => {
  it("offers no button, because there is no army to fill in", async () => {
    await boot({}, (g) => {
      const home = g.villages[g.activeVillageId]!;
      g.reports.push({
        id: "r-blind",
        time: g.now,
        type: "attack",
        originId: home.id,
        targetId: "v:9:9",
        origin: {
          id: home.id,
          name: "Home",
          x: home.x,
          y: home.y,
          ownerId: home.ownerId ?? undefined,
        },
        target: { id: "v:9:9", name: "Barbarians", x: 9, y: 9 },
        // The attacker was wiped out, so his copy holds no defender: blindSide zeroed the whole side.
        attacker: { units: { sword: 1 }, losses: { sword: 1 } },
        defender: { units: {}, losses: {} },
        defenderHidden: true,
        luck: 0,
        morale: 1,
        wallBefore: 20,
        wallAfter: 20,
        outcome: "defenderWon",
        read: false,
      });
    });
    render(<ReportsView />);
    fireEvent.click(await screen.findByText(/→/));
    // Replaying this would fight an empty village and answer a battle the report never described.
    expect(screen.queryByRole("button", { name: "Fill in simulator" })).toBe(
      null,
    );
  });
});

describe("the buildings a prefill brings", () => {
  it("sends them, so a catapult aims at a building the village really has", async () => {
    const { paths } = await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    useUiStore.getState().openSimulator({
      attacker: { sword: 500, catapult: 20 },
      defender: { spear: 50 },
      defenderBuildings: { ...emptyBuildings(), main: 7, wall: 8, barracks: 4 },
      wall: 8,
    });
    render(<SimulatorTab />);
    await waitFor(() =>
      expect((atk("sword") as HTMLInputElement).value).toBe("500"),
    );
    fireEvent.change(document.getElementById("sim-catapult")!, {
      target: { value: "barracks" },
    });
    screen.getByRole("button", { name: "Simulate" }).click();
    await waitFor(() =>
      expect(paths.some((p) => p.includes("/simulate"))).toBe(true),
    );
    const sent = JSON.parse(
      String(
        (
          fetch as unknown as { mock: { calls: [string, RequestInit][] } }
        ).mock.calls.find(([url]) => url.includes("/simulate"))![1].body,
      ),
    ) as {
      defender: { buildings: Record<string, number>; wall: number };
    };
    expect(sent.defender.buildings.barracks).toBe(4);
    expect(sent.defender.buildings.main).toBe(7);
    expect(sent.defender.wall).toBe(8);
  });

  it("sends no buildings at all when nothing told it what the village has", async () => {
    const { paths } = await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    screen.getByRole("button", { name: "Simulate" }).click();
    await waitFor(() =>
      expect(paths.some((p) => p.includes("/simulate"))).toBe(true),
    );
    const sent = JSON.parse(
      String(
        (
          fetch as unknown as { mock: { calls: [string, RequestInit][] } }
        ).mock.calls.find(([url]) => url.includes("/simulate"))![1].body,
      ),
    ) as { defender: Record<string, unknown> };
    // The server's own default (a town hall and the wall) is the honest answer when the player knows nothing.
    expect(sent.defender.buildings).toBe(undefined);
  });

  it("does not ask for the wall's level twice", async () => {
    await boot();
    render(<SimulatorTab />);
    fireInput(atk("catapult"), "30");
    fireEvent.change(document.getElementById("sim-wall")!, {
      target: { value: "9" },
    });
    fireEvent.change(document.getElementById("sim-catapult")!, {
      target: { value: "wall" },
    });
    // The wall already has a field, so that field is the level the catapults break it from — not a second one.
    expect(document.getElementById("sim-catapult-level")).toBe(null);
    expect(screen.getAllByText(/Wall level/).length).toBe(1);
    expect(
      (document.getElementById("sim-wall") as HTMLInputElement).value,
    ).toBe("9");
  });

  it("names a building rather than offering a draw it cannot show", async () => {
    await boot();
    render(<SimulatorTab />);
    fireInput(atk("catapult"), "30");
    const select = document.getElementById("sim-catapult") as HTMLSelectElement;
    // No "a random building": the simulator answers one question, and a draw would make the answer differ each press.
    // What the server would do with no target is take the first candidate, which is always the town hall.
    expect([...select.options].map((o) => o.value)).toEqual([
      ...BUILDING_ORDER,
    ]);
    expect(select.value).toBe("main");
    // Every building has its own ceiling, so the level field has to follow the one that is chosen.
    const level = () =>
      document.getElementById("sim-catapult-level") as HTMLInputElement;
    expect(level().max).toBe(String(BUILDINGS.main.maxLevel));
    fireEvent.change(select, { target: { value: "hide" } });
    expect(level().max).toBe(String(BUILDINGS.hide.maxLevel));
    fireEvent.change(select, { target: { value: "wall" } });
    // The wall keeps its own field, which carries its own ceiling.
    expect(document.getElementById("sim-catapult-level")).toBe(null);
    expect((document.getElementById("sim-wall") as HTMLInputElement).max).toBe(
      "20",
    );
  });

  it("offers no catapult target when the attacker brings no catapults", async () => {
    await boot();
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    // Catapults are the only thing that breaks a building, so the field would be a choice that changes nothing.
    expect(document.getElementById("sim-catapult")).toBe(null);
    expect(document.getElementById("sim-catapult-level")).toBe(null);
    fireEvent.change(atk("catapult"), { target: { value: "1" } });
    expect(document.getElementById("sim-catapult")).not.toBe(null);
    // Taking the last one away takes the field with it, and the target it held.
    fireEvent.change(document.getElementById("sim-catapult")!, {
      target: { value: "barracks" },
    });
    fireEvent.change(atk("catapult"), { target: { value: "0" } });
    expect(document.getElementById("sim-catapult")).toBe(null);
    fireEvent.change(atk("catapult"), { target: { value: "1" } });
    // The field is back, on the town hall rather than the target that went with the catapults.
    expect(
      (document.getElementById("sim-catapult") as HTMLSelectElement).value,
    ).toBe("main");
  });

  it("asks for the level of the building the catapults are aimed at", async () => {
    const { paths } = await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    // Without catapults there is nothing to aim, so neither the target nor its level is offered.
    expect(document.getElementById("sim-catapult")).toBe(null);
    expect(document.getElementById("sim-catapult-level")).toBe(null);
    fireInput(atk("catapult"), "30");
    // With them, a target and its level, starting on the town hall.
    expect(document.getElementById("sim-catapult-level")).not.toBe(null);
    expect(screen.getByText("Town Hall level:")).toBeTruthy();
    fireEvent.change(document.getElementById("sim-catapult")!, {
      target: { value: "barracks" },
    });
    // A building nothing is known about starts at the lowest a village can have, so the choice is never dropped.
    const level = () =>
      document.getElementById("sim-catapult-level") as HTMLInputElement;
    expect(level().value).toBe("1");
    expect(level().max).toBe(String(BUILDINGS.barracks.maxLevel));
    // Named for the building it belongs to, so it is clear what the number is the level of.
    expect(screen.getByText("Barracks level:")).toBeTruthy();
    fireEvent.change(document.getElementById("sim-catapult")!, {
      target: { value: "storage" },
    });
    expect(screen.getByText("Storehouse level:")).toBeTruthy();
    expect(screen.queryByText("Barracks level:")).toBe(null);
    fireEvent.change(document.getElementById("sim-catapult")!, {
      target: { value: "barracks" },
    });

    fireInput(level(), "9");
    screen.getByRole("button", { name: "Simulate" }).click();
    await waitFor(() =>
      expect(paths.some((p) => p.includes("/simulate"))).toBe(true),
    );
    const sent = JSON.parse(
      String(
        (
          fetch as unknown as { mock: { calls: [string, RequestInit][] } }
        ).mock.calls.find(([url]) => url.includes("/simulate"))![1].body,
      ),
    ) as { defender: { buildings: Record<string, number> } };
    // The level travels with the target: a catapult can only break a building it is told the level of, so sending
    // the name alone made the choice do nothing at all.
    expect(sent.defender.buildings.barracks).toBe(9);
  });

  it("forgets the target and its level when cleared", async () => {
    await boot();
    render(<SimulatorTab />);
    fireInput(atk("catapult"), "30");
    fireEvent.change(document.getElementById("sim-catapult")!, {
      target: { value: "barracks" },
    });
    fireInput(document.getElementById("sim-catapult-level")!, "7");
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    // Clearing empties the army, so there are no catapults left to aim and the target goes with them.
    expect(document.getElementById("sim-catapult")).toBe(null);
    expect(document.getElementById("sim-catapult-level")).toBe(null);
    // Bringing catapults back starts from no target rather than the one that was cleared.
    fireInput(atk("catapult"), "30");
    expect(
      (document.getElementById("sim-catapult") as HTMLSelectElement).value,
    ).toBe("main");
    expect(screen.getByText("Town Hall level:")).toBeTruthy();
  });
});

describe("the army fields", () => {
  it("keeps the icon column a real width, so the icons never vanish on a narrow screen", async () => {
    await boot();
    const { container } = render(<SimulatorTab />);
    // An `auto` icon column is the one that gives way when the row runs out of space, and on a 320 px phone it
    // collapsed to 0 px: every unit icon disappeared, leaving rows of numbers nobody could tell apart.
    const grid = container.querySelector("div.grid")!;
    expect(grid.className).toContain("grid-cols-[18px_auto_auto_auto_auto]");
    const icons = [...grid.querySelectorAll("img")];
    expect(icons.length).toBe(UNIT_IDS.length);
  });

  it("shows the unit as an icon only, with the name kept for screen readers", async () => {
    await boot();
    const { container } = render(<SimulatorTab />);
    // The icon is there, and no visible text names the unit — twelve names twice over was the whole width.
    const icon = container.querySelector('img[alt="Swordsman"]');
    expect(icon).not.toBeNull();
    // Every bit of text in the fields is inside `.sr-only`, so nothing names the unit on screen. (textContent
    // would still contain it, which is the point of sr-only, so it is the wrapper that has to be checked.)
    // Only the army fields: the settings on the right name themselves on screen, as they should.
    for (const el of container.querySelectorAll(
      'label[for^="sim-atk-"] > span, label[for^="sim-def-"] > span',
    )) {
      const named = (el.textContent ?? "").trim() !== "";
      if (named) expect(el.classList.contains("sr-only")).toBe(true);
    }
    // The name still reaches the accessible label, and it says which side it belongs to.
    expect(screen.getByLabelText("Attacker Swordsman")).toBeTruthy();
    expect(screen.getByLabelText("Defender Swordsman")).toBeTruthy();
  });

  it("puts each unit on one row: its icon, then the attacker's troops and level, then the defender's", async () => {
    await boot();
    render(<SimulatorTab />);
    // happy-dom reports every rect as 0x0, so measuring positions proves nothing here. What carries the layout is
    // the grid order: every unit must own five consecutive cells. A wrapper element inside the grid emits a
    // different number of items per unit and the columns drift a cell per row — which is exactly what went wrong.
    const grid = document.getElementById("sim-atk-sword")!.closest(".grid")!;
    // Eight heading cells, then five per unit.
    expect(grid.children.length).toBe(68);
    // The headings are the first rows of the same grid, so they sit over the columns they name. In a grid of their
    // own they could not: an empty cell collapses to 0 px while the icon column does not, and the headings drifted
    // right of the fields under them. Each side's name spans the troops-and-level pair it heads.
    expect(grid.children[0]!.textContent).toBe("");
    expect(grid.children[1]!.textContent).toBe("Attacker");
    expect(grid.children[2]!.textContent).toBe("Defender");
    expect(grid.children[3]!.textContent).toBe("");
    expect(grid.children[4]!.textContent).toBe("Troops");
    expect(grid.children[5]!.textContent).toBe("Level");
    expect(grid.children[6]!.textContent).toBe("Troops");
    expect(grid.children[7]!.textContent).toBe("Level");
    // The headings name four columns and the cells under them are those four, in that order.
    expect(grid.children[9]!.querySelector("input")!.id).toBe("sim-atk-spear");
    expect(grid.children[10]!.querySelector("select")!.id).toBe(
      "sim-atk-lvl-spear",
    );
    expect(grid.children[11]!.querySelector("input")!.id).toBe("sim-def-spear");
    expect(grid.children[12]!.querySelector("select")!.id).toBe(
      "sim-def-lvl-spear",
    );
    const rows: [string, string][] = [
      ["Spearman", "spear"],
      ["Swordsman", "sword"],
      ["Axeman", "axe"],
      ["Archer", "archer"],
      ["Scout", "scout"],
      ["Light cavalryman", "lightCav"],
      ["Mounted archer", "mountedArcher"],
      ["Heavy cavalryman", "heavyCav"],
      ["Ram", "ram"],
      ["Catapult", "catapult"],
      ["Knight", "paladin"],
      ["Hetman", "noble"],
    ];
    rows.forEach(([name, unit], i) => {
      const at = (n: number) => grid.children[i * 5 + 8 + n]!;
      expect(at(0).querySelector("img")!.getAttribute("alt")).toBe(name);
      expect(at(1).querySelector("input")!.id).toBe(`sim-atk-${unit}`);
      expect(at(3).querySelector("input")!.id).toBe(`sim-def-${unit}`);
      // A noble is bought, not researched, so its level cells are empty but still hold the column open.
      const levelId = (side: "atk" | "def") => `sim-${side}-lvl-${unit}`;
      if (unit === "noble") {
        expect(at(2).querySelector("select")).toBeNull();
        expect(at(4).querySelector("select")).toBeNull();
      } else {
        expect(at(2).querySelector("select")!.id).toBe(levelId("atk"));
        expect(at(4).querySelector("select")!.id).toBe(levelId("def"));
      }
    });
  });

  it("offers the Smithy's levels, not the locked level 0", async () => {
    await boot();
    render(<SimulatorTab />);
    for (const side of ["atk", "def"]) {
      const select = document.getElementById(
        `sim-${side}-lvl-sword`,
      ) as HTMLSelectElement;
      // Level 0 is "not unlocked" and buys nothing: a field offering it would let a player pick a state no village
      // has. (The browser has no settled value to read here either — a controlled select whose value is not in its
      // list falls back to the first option — so what is checked is the list itself, and the send below.)
      expect([...select.options].map((o) => o.value)).toEqual(["1", "2", "3"]);
      expect([...select.options].map((o) => o.textContent)).toEqual([
        "1",
        "2",
        "3",
      ]);
    }
  });

  it("sends the levels of both armies with the battle, so they change the answer", async () => {
    const { paths } = await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    fireInput(document.getElementById("sim-def-spear")!, "200");
    fireEvent.change(document.getElementById("sim-atk-lvl-sword")!, {
      target: { value: "3" },
    });
    fireEvent.change(document.getElementById("sim-def-lvl-spear")!, {
      target: { value: "2" },
    });
    screen.getByRole("button", { name: "Simulate" }).click();
    await waitFor(() =>
      expect(paths.some((p) => p.includes("/simulate"))).toBe(true),
    );
    const sent = JSON.parse(
      String(
        (
          fetch as unknown as { mock: { calls: [string, RequestInit][] } }
        ).mock.calls.find(([url]) => url.includes("/simulate"))![1].body,
      ),
    ) as {
      attacker: { research: Record<string, number> };
      defender: { research: Record<string, number> };
    };
    expect(sent.attacker.research.sword).toBe(3);
    expect(sent.defender.research.spear).toBe(2);
    // Every unit is named, so nothing falls back to a level the server would read as locked.
    expect(sent.attacker.research.spear).toBe(1);
  });
});

describe("the simulator layout", () => {
  it("keeps the army fields narrow enough to leave the settings room beside them", async () => {
    await boot();
    render(<SimulatorTab />);
    // A stretched input was the whole problem: the two columns must not claim the row.
    const widths = [
      ...document.querySelectorAll<HTMLInputElement>(
        'input[id^="sim-atk-"], input[id^="sim-def-"]',
      ),
    ].map((el) => el.className);
    expect(widths.length).toBe(24);
    expect(
      widths.every((c) => /\bw-16\b/.test(c) && !/\bw-full\b/.test(c)),
    ).toBe(true);
  });

  it("stacks the settings under the armies, with the answer beside them on a wide screen", async () => {
    await boot();
    render(<SimulatorTab />);
    // Catapults in the army, so the target field that belongs to this block exists at all.
    fireInput(atk("catapult"), "30");
    const armies = document
      .querySelector('input[id="sim-atk-sword"]')!
      .closest("div[class*='p-3']")!;
    const settings = document
      .getElementById("sim-wall")!
      .closest("div[class*='p-3']")!;
    // Neither is a framed panel: they are fields on the page.
    expect(armies.className).not.toContain("panel");
    expect(settings.className).not.toContain("panel");

    // Both sit in the one left-hand column, the settings under the armies, so the eye runs down a single column.
    const left = armies.parentElement!;
    expect(left).toBe(settings.parentElement);
    expect(left.className).toContain("flex-col");
    expect(left.className).not.toContain("lg:flex-row");

    // And that column is the left half of the two-column arrangement, the answer on the right.
    const row = left.parentElement!;
    expect(row.className).toContain("lg:flex-row");
    expect(row.className).toContain("flex-col");

    // The button closes the column of fields it acts on: it sits under the last setting, not at the top of the
    // answer column where it floated beside the army headers.
    const simulate = screen.getByRole("button", { name: "Simulate" });
    expect(left.contains(simulate)).toBe(true);
    const lastSetting = document.getElementById("sim-luck")!;
    expect(
      lastSetting.compareDocumentPosition(simulate) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    // Every setting lives in that one block, not scattered across a four-column strip.
    for (const id of ["sim-morale", "sim-luck", "sim-catapult"]) {
      expect(document.getElementById(id)!.closest("div[class*='p-3']")).toBe(
        settings,
      );
    }
  });
});

describe("the settings panel width", () => {
  it("never stretches across the page, stacked or beside the armies", async () => {
    await boot();
    render(<SimulatorTab />);
    const settings = document
      .getElementById("sim-wall")!
      .closest("div[class*='p-3']") as HTMLElement;
    // The fields inside are 64px, so the panel has no reason to grow: `w-full` alone let it run the whole width
    // when the settings drop under the armies.
    expect(settings.className).toContain("max-w-[280px]");
    expect(/\bw-full\b/.test(settings.className)).toBe(true);
    // And the same cap holds beside the armies on a wide screen.
    expect(settings.className).toContain("shrink-0");
  });
});

describe("the result summary lines", () => {
  it("says nothing about a haul: the simulator answers the fight, not the loot", async () => {
    await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    screen.getByRole("button", { name: "Simulate" }).click();
    expect(await screen.findByText("The attacker wins")).toBeTruthy();
    expect(screen.queryByText(/Haul back/)).toBe(null);
  });
});

describe("the result does not follow the next edit", () => {
  it("keeps showing the armies the run was made with after the fields are changed", async () => {
    await boot({
      "POST /api/v1/worlds/1/simulate": () => ({ status: 200, body: ANSWER }),
    });
    render(<SimulatorTab />);
    fireInput(atk("sword"), "500");
    screen.getByRole("button", { name: "Simulate" }).click();
    expect(await screen.findByText("The attacker wins")).toBeTruthy();

    const sentOf = (side: "atk" | "def") => {
      // The first row of that side's army table in the result card — the troops it brought, not its losses. Both
      // tables are inside the card, attacker first.
      const block = document.querySelector("[data-sim-result]")!;
      const table = block.querySelectorAll("table")[side === "atk" ? 0 : 1];
      const row = table?.querySelectorAll("tbody tr")[0];
      return (row as HTMLElement | undefined)?.innerText ?? "";
    };
    expect(sentOf("atk")).toContain("500");

    // Now start typing the next fight: the last answer must not rewrite itself.
    fireInput(atk("sword"), "9");
    expect((atk("sword") as HTMLInputElement).value).toBe("9");
    expect(sentOf("atk")).toContain("500");
  });
});
