// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  within,
} from "@testing-library/react";
import { setLocale } from "../src/i18n/i18n";
import { MapView } from "../src/ui/views/MapView";
import { useGameStore } from "../src/state/gameStore";
import { useUiStore } from "../src/state/uiStore";
import { MARK_COLORS, nextMarkColor } from "../src/ui/map/playerMarks";
import { activeVillage, makeBotVillage, newTestGame } from "./helpers";
import { stubServer } from "./serverStub";

/** The map with a bot village next door, booted so the map cache holds the region the minimap draws from. */
async function showMap() {
  const g = newTestGame();
  const home = activeVillage(g);
  const bot = makeBotVillage(g);
  stubServer(g);
  await useGameStore.getState().boot();
  const view = render(<MapView />);
  // The map loads its chunks after the first paint; the minimap draws from them.
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  return { g, home, bot, container: view.container };
}

const panel = (c: HTMLElement) =>
  c.querySelector<HTMLElement>("[data-player-marks]")!;

/** The ring a marked village is drawn with, or null for an unmarked one. */
const ring = (c: HTMLElement, x: number, y: number) =>
  c.querySelector<HTMLElement>(`[data-tile="${x}|${y}"] [data-mark]`);

const mark = (c: HTMLElement, name: string, color?: string) => {
  const form = within(panel(c));
  fireEvent.change(form.getByLabelText("Player name"), {
    target: { value: name },
  });
  if (color)
    fireEvent.change(form.getByLabelText("Colour"), {
      target: { value: color },
    });
  fireEvent.click(form.getByRole("button", { name: "Mark" }));
};

beforeEach(() => {
  setLocale("en");
  useUiStore.setState({
    view: "map",
    mapCenter: null,
    prefs: { ...useUiStore.getState().prefs, mapPlayerMarks: [] },
  });
});
afterEach(() => {
  cleanup();
  useGameStore.getState().shutdown();
  vi.unstubAllGlobals();
});

describe("marking players on the map", () => {
  it("the panel sits under the map, above the hint about how the map works", async () => {
    const { container } = await showMap();
    const marks = panel(container);
    expect(marks).toBeTruthy();
    // Under the map's frame, not in the toolbar over it.
    expect(marks.closest("[data-above]")).toBeNull();
    expect(
      marks.compareDocumentPosition(
        container.querySelector("[data-map-surface]")!,
      ) & Node.DOCUMENT_POSITION_PRECEDING,
    ).toBeTruthy();
    const help = [...container.querySelectorAll("p")].find((p) =>
      p.textContent?.includes("Click a village to open its menu"),
    )!;
    expect(
      marks.compareDocumentPosition(help) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("a marked player's villages are ringed in the colour and their minimap dot takes it", async () => {
    const { container, bot } = await showMap();
    expect(ring(container, bot.x, bot.y)).toBeNull();

    mark(container, "Test bot", "#12ab34");

    const ringed = ring(container, bot.x, bot.y)!;
    expect(ringed.getAttribute("data-mark")).toBe("#12ab34");
    expect(ringed.style.border).toContain("2px solid");
    const dot = container.querySelector(
      `[data-minimap-dot="#12ab34"]`,
    ) as SVGRectElement | null;
    expect(dot).toBeTruthy();
    expect(dot!.getAttribute("x")).toBeTruthy();
    // The bot's village is the only one of that player, so the dot is the bot's field.
    expect(useUiStore.getState().prefs.mapPlayerMarks).toEqual([
      { name: "Test bot", color: "#12ab34" },
    ]);
  });

  it("the name is matched the way a player types it: any case, spaces around it", async () => {
    const { container, bot } = await showMap();
    mark(container, "  tEsT bOt ");
    expect(ring(container, bot.x, bot.y)).toBeTruthy();
    expect(useUiStore.getState().prefs.mapPlayerMarks[0]!.name).toBe(
      "tEsT bOt",
    );
  });

  it("unmarking takes the ring and the minimap dot away again", async () => {
    const { container, bot } = await showMap();
    mark(container, "Test bot", "#12ab34");
    expect(ring(container, bot.x, bot.y)).toBeTruthy();

    fireEvent.click(
      within(panel(container)).getByRole("button", {
        name: "Unmark Test bot",
      }),
    );

    expect(ring(container, bot.x, bot.y)).toBeNull();
    expect(container.querySelector("[data-minimap-dot]")).toBeNull();
    expect(useUiStore.getState().prefs.mapPlayerMarks).toEqual([]);
  });

  it("a name no player in the world has is called out, and can still be marked", async () => {
    const { container } = await showMap();
    const form = within(panel(container));
    fireEvent.change(form.getByLabelText("Player name"), {
      target: { value: "Nobody here" },
    });
    expect(panel(container).querySelector("[data-mark-unknown]")).toBeTruthy();
    // A mark made now is there for when that player joins or the name is right.
    fireEvent.click(form.getByRole("button", { name: "Mark" }));
    expect(useUiStore.getState().prefs.mapPlayerMarks).toEqual([
      { name: "Nobody here", color: nextMarkColor([]) },
    ]);
  });

  it("the colour can be changed on a mark already made", async () => {
    const { container, bot } = await showMap();
    mark(container, "Test bot", "#12ab34");
    fireEvent.change(
      within(panel(container)).getByLabelText("Colour for Test bot"),
      { target: { value: "#abcdef" } },
    );
    expect(ring(container, bot.x, bot.y)!.getAttribute("data-mark")).toBe(
      "#abcdef",
    );
    expect(
      container.querySelector('[data-minimap-dot="#abcdef"]'),
    ).toBeTruthy();
  });

  it("a mark outlives the player colours switch: it is the player's own choice, not the owner's colour", async () => {
    const { container, bot } = await showMap();
    mark(container, "Test bot", "#12ab34");
    expect(container.querySelector("[data-roof]")).toBeTruthy();

    fireEvent.click(
      within(container.querySelector("[data-map-display]")!).getByRole(
        "checkbox",
        { name: "Player colours" },
      ),
    );

    // Turning the owners' colours off is a general tidying; a marked player stays marked, or the mark would be
    // useless exactly when the map is at its busiest.
    const roof = container.querySelector<HTMLElement>("[data-roof]")!;
    expect(roof.style.backgroundColor).toMatch(/#12ab34|rgb\(18, 171, 52\)/);
    expect(ring(container, bot.x, bot.y)!.getAttribute("data-mark")).toBe(
      "#12ab34",
    );
  });

  it("marks are remembered in the browser", async () => {
    const { container } = await showMap();
    mark(container, "Test bot", "#12ab34");
    const raw = JSON.parse(localStorage.getItem("lh-ui-prefs")!) as {
      mapPlayerMarks: unknown;
    };
    expect(raw.mapPlayerMarks).toEqual([
      { name: "Test bot", color: "#12ab34" },
    ]);
  });
});

describe("the colour a new mark starts with", () => {
  it("is one no other mark uses, and after the palette runs out, the next in turn", () => {
    const first = nextMarkColor([]);
    expect(MARK_COLORS).toContain(first);
    const second = nextMarkColor([{ name: "a", color: first }]);
    expect(second).not.toBe(first);
    expect(MARK_COLORS).toContain(second);

    // Every colour of the palette taken: it comes back round to the first rather than repeating one out of order.
    const all = MARK_COLORS.map((color, i) => ({ name: `p${i}`, color }));
    expect(nextMarkColor(all)).toBe(MARK_COLORS[0]);
  });
});
