// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { setLocale } from "../src/i18n/i18n";
import { reportVillageOf, unreadReports } from "../src/engine/reports";
import type { GameState, Report, Village } from "../src/engine/types";
import { useGameStore } from "../src/state/gameStore";
import { useUiStore } from "../src/state/uiStore";
import { ReportsView } from "../src/ui/views/ReportsView";
import {
  activeVillage,
  makeBotVillage,
  newTestGame,
  stateFixture,
} from "./helpers";
import { stubServer, type StubRoute } from "./serverStub";

let g: GameState;
let home: Village;
let bot: Village;
let bodies: Record<string, unknown[]>;

function attackReport(over: Partial<Report>): Report {
  return {
    id: `r${g.reports.length + 1}`,
    time: g.now,
    type: "attack",
    originId: home.id,
    targetId: bot.id,
    origin: reportVillageOf(g, home.id),
    target: reportVillageOf(g, bot.id),
    attacker: { units: { lightCav: 5 }, losses: {} },
    defender: { units: {}, losses: {} },
    luck: 0,
    morale: 1,
    wallBefore: 0,
    wallAfter: 0,
    outcome: "attackerWon",
    read: false,
    ...over,
  };
}

const record =
  (name: string): StubRoute =>
  (body) => {
    (bodies[name] ??= []).push(body);
    return { status: 200, body: stateFixture(g) };
  };

const rows = () => screen.getAllByRole("row").slice(1);
const tab = (name: string) => screen.getByRole("tab", { name });

beforeEach(async () => {
  setLocale("en");
  g = newTestGame();
  home = activeVillage(g);
  bot = makeBotVillage(g);
  g.reports.push(attackReport({}));
  g.reports.push(attackReport({ autoFarm: true }));
  bodies = {};
  stubServer(g, {
    routes: {
      "POST /api/v1/worlds/1/orders/mark-all-reports-read": record("markAll"),
      "POST /api/v1/worlds/1/orders/delete-read-reports": record("deleteRead"),
    },
  });
  useUiStore.setState({
    view: "reports",
    reportsTab: "manual",
    reportsVillageFilter: null,
  });
  await useGameStore.getState().boot();
});
afterEach(() => {
  cleanup();
  useGameStore.getState().shutdown();
  vi.unstubAllGlobals();
});

describe("auto-farm reports", () => {
  it("the Manual tab opens first and holds only the reports a player sent", () => {
    render(<ReportsView />);
    expect(tab("Manual").getAttribute("aria-selected")).toBe("true");
    expect(rows()).toHaveLength(1);
    expect(screen.queryByText("auto-farm")).toBeNull();
  });

  it("the Auto-farm tab holds the raids, each tagged, and the report itself carries the tag", () => {
    render(<ReportsView />);
    fireEvent.click(tab("Auto-farm"));
    expect(rows()).toHaveLength(1);
    const tagged = rows().filter((r) => r.textContent!.includes("auto-farm"));
    expect(tagged).toHaveLength(1);
    fireEvent.click(tagged[0]!);
    expect(screen.getByRole("heading", { level: 3 }).textContent).toContain(
      "auto-farm",
    );
  });

  it("each tab counts against its own budget", () => {
    render(<ReportsView />);
    expect(screen.getByRole("heading", { level: 2 }).textContent).toContain(
      "(1/500)",
    );
    fireEvent.click(tab("Auto-farm"));
    expect(screen.getByRole("heading", { level: 2 }).textContent).toContain(
      "(1/250)",
    );
  });

  it("the tab holds when the list opens again, while the village filter is cleared", () => {
    useUiStore.setState({ reportsVillageFilter: home.id });
    const { unmount } = render(<ReportsView />);
    fireEvent.click(tab("Auto-farm"));
    unmount();
    expect(useUiStore.getState().reportsVillageFilter).toBeNull();
    render(<ReportsView />);
    expect(tab("Auto-farm").getAttribute("aria-selected")).toBe("true");
    expect(rows()).toHaveLength(1);
  });

  it("they arrive unread but stay out of the unread count", () => {
    const game = useGameStore.getState().game!;
    expect(game.reports.every((r) => !r.read)).toBe(true);
    expect(unreadReports(game)).toBe(1);
  });

  it("when the filters hide every report, the list says so", () => {
    // A read manual report: the tab holds one, but the unread filter hides it.
    act(() => {
      const game = useGameStore.getState().game!;
      useGameStore.setState({
        game: {
          ...game,
          reports: game.reports.map((r) => ({ ...r, read: true })),
        },
      });
    });
    render(<ReportsView />);
    fireEvent.change(screen.getByRole("combobox"), {
      target: { value: "unread" },
    });
    screen.getByText("No reports match the filters.");
  });

  it("a tab with nothing in it says so, even when the other tab has reports", () => {
    act(() => {
      const game = useGameStore.getState().game!;
      useGameStore.setState({
        game: { ...game, reports: game.reports.filter((r) => r.autoFarm) },
      });
    });
    render(<ReportsView />);
    screen.getByText(
      "No reports. Send an attack from the map or the Rally Point.",
    );
  });

  it("each tab's bulk actions reach only that tab", async () => {
    render(<ReportsView />);
    fireEvent.click(screen.getByText("Mark all as read"));
    await waitFor(() => expect(bodies.markAll).toEqual([{ scope: "manual" }]));
    fireEvent.click(tab("Auto-farm"));
    fireEvent.click(screen.getByText("Mark all as read"));
    fireEvent.click(screen.getByText("Delete read"));
    await waitFor(() =>
      expect(bodies.markAll).toEqual([
        { scope: "manual" },
        { scope: "autoFarm" },
      ]),
    );
    await waitFor(() =>
      expect(bodies.deleteRead).toEqual([{ scope: "autoFarm" }]),
    );
  });
});
