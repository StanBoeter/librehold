// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { setLocale } from "../src/i18n/i18n";
import { MapView } from "../src/ui/views/MapView";
import { useGameStore } from "../src/state/gameStore";
import { useUiStore } from "../src/state/uiStore";
import type { Command, GameState, Village } from "../src/engine/types";
import { activeVillage, makeBotVillage, newTestGame } from "./helpers";

const HOUR = 3_600_000;

function addCmd(
  g: GameState,
  c: Partial<Command> & Pick<Command, "id" | "type" | "originId" | "targetId">,
): Command {
  const full: Command = {
    units: {},
    departedAt: g.now - HOUR,
    arrivesAt: g.now + 3 * HOUR,
    ...c,
  };
  g.commands.push(full);
  return full;
}

/** A game by the bot villages next door, with a passing army in sight of home. */
function setup(build: (g: GameState, home: Village) => void): void {
  const g = newTestGame();
  const home = activeVillage(g);
  g.config.sightRadius = 3;
  build(g, home);
  useGameStore.setState({
    game: g,
    phase: "ready",
    devMode: false,
    notice: null,
    lastError: null,
  });
}

beforeEach(() => {
  setLocale("en");
  useUiStore.setState({
    view: "map",
    mapCenter: null,
    prefs: {
      ...useUiStore.getState().prefs,
      mapIcons: true,
      mapNearbyAttacks: true,
      mapOwnAttacks: true,
      mapSupport: true,
      mapMerchants: true,
      mapToolbarOpen: true,
    },
  });
});
afterEach(cleanup);

describe("map overlays", () => {
  it("the display panel sets the icons off and keeps the lines and arrows", () => {
    setup((g, home) => {
      const bot = makeBotVillage(g);
      addCmd(g, {
        id: "own",
        type: "attack",
        originId: home.id,
        targetId: bot.id,
        units: { axe: 10 },
      });
    });
    const { container } = render(<MapView />);
    expect(container.querySelector("[data-pixel-village]")).toBeTruthy();
    expect(container.querySelector('[data-army="own"] line')).toBeTruthy();

    fireEvent.click(screen.getByRole("checkbox", { name: "Icons" }));

    expect(useUiStore.getState().prefs.mapIcons).toBe(false);
    // The villages are not icons of an army and stay on the map.
    expect(container.querySelector("[data-pixel-village]")).toBeTruthy();
    expect(container.querySelector("[data-crowd]")).toBeNull();
    const army = container.querySelector<SVGGElement>('[data-army="own"]')!;
    expect(army.querySelector("[data-marker]")).toBeNull();
    expect(army.querySelector("line")!.getAttribute("marker-end")).toBe(
      "url(#army-arrow-ownAttack)",
    );
  });

  it('"Nearby armies" drops passing armies but never an incoming attack', () => {
    setup((g, home) => {
      const bot = makeBotVillage(g);
      const bot2 = makeBotVillage(g);
      addCmd(g, {
        id: "passing",
        type: "attack",
        originId: bot.id,
        targetId: bot2.id,
      });
      addCmd(g, {
        id: "inc",
        type: "attack",
        originId: bot.id,
        targetId: home.id,
      });
    });
    const { container } = render(<MapView />);
    expect(container.querySelector('[data-army="passing"]')).toBeTruthy();
    expect(container.querySelector('[data-army="inc"]')).toBeTruthy();

    fireEvent.click(screen.getByRole("checkbox", { name: "Nearby armies" }));

    expect(useUiStore.getState().prefs.mapNearbyAttacks).toBe(false);
    expect(container.querySelector('[data-army="passing"]')).toBeNull();
    const inc = container.querySelector('[data-army="inc"]')!;
    expect(inc.getAttribute("data-kind")).toBe("incomingAttack");
    expect(inc.querySelector("line")!.getAttribute("stroke")).toBe("#ff2d2d");
  });
  it("own attacks, support and merchants have toggles of their own, and the panel sits above the map", () => {
    setup((g, home) => {
      const bot = makeBotVillage(g);
      addCmd(g, {
        id: "own",
        type: "attack",
        originId: home.id,
        targetId: bot.id,
        units: { axe: 3 },
      });
      addCmd(g, {
        id: "sup",
        type: "support",
        originId: home.id,
        targetId: bot.id,
      });
      addCmd(g, {
        id: "trade",
        type: "trade",
        originId: home.id,
        targetId: bot.id,
        merchants: 2,
      });
      addCmd(g, {
        id: "inc",
        type: "attack",
        originId: bot.id,
        targetId: home.id,
      });
    });
    const { container } = render(<MapView />);
    const ids = () =>
      [...container.querySelectorAll("[data-army]")]
        .map((e) => e.getAttribute("data-army"))
        .sort();
    expect(ids()).toEqual(["inc", "own", "sup", "trade"]);
    // The display options are a panel of their own, drawn over the map rather than in it: a bar above the frame, so it
    // reads as part of the map and leaves nothing hanging under it.
    const panel = container.querySelector("[data-map-display]")!;
    expect(panel).toBeTruthy();
    expect(panel.closest("[data-map-surface]")).toBeNull();
    expect(panel.closest("[data-above]")).toBeTruthy();
    expect(
      panel.compareDocumentPosition(
        container.querySelector("[data-map-surface]")!,
      ) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // No visible heading: the switches read for themselves, and the legend survives only as the fieldset's name.
    expect(panel.querySelector("legend")!.className).toContain("sr-only");
    // The map's navigation lives in the same bar, so the panel is the whole toolbar rather than a strip of switches
    // with a page-wide row of buttons stranded above it.
    expect(panel.textContent).toContain("My village");
    expect(panel.textContent).toContain("Go");
    expect(panel.querySelector('input[placeholder="x|y"]')).toBeTruthy();
    expect(container.querySelector("main h2")).toBeNull();

    fireEvent.click(screen.getByRole("checkbox", { name: "Own attacks" }));
    expect(ids()).toEqual(["inc", "sup", "trade"]);
    fireEvent.click(screen.getByRole("checkbox", { name: "Support" }));
    expect(ids()).toEqual(["inc", "trade"]);
    fireEvent.click(screen.getByRole("checkbox", { name: "Merchants" }));
    expect(ids()).toEqual(["inc"]);
    expect(useUiStore.getState().prefs).toMatchObject({
      mapOwnAttacks: false,
      mapSupport: false,
      mapMerchants: false,
    });
  });
  it('"Show farming" hides the farm raids and their way home, and comes back when ticked again', () => {
    setup((g, home) => {
      const bot = makeBotVillage(g);
      addCmd(g, {
        id: "farm",
        type: "attack",
        originId: home.id,
        targetId: bot.id,
        units: { lightCav: 3 },
        farm: true,
      });
      addCmd(g, {
        id: "own",
        type: "attack",
        originId: home.id,
        targetId: bot.id,
        units: { axe: 3 },
      });
    });
    // This switch is the player's preference kept by the world, not a local one, so the order it sends is what flips it.
    const hide = useGameStore.getState().hideFarmOnMap;
    useGameStore.setState({
      hideFarmOnMap: (on) => {
        const g = useGameStore.getState().game!;
        g.farmAssistant.hideOnMap = on;
        useGameStore.setState({ game: { ...g } });
        return Promise.resolve(null);
      },
    });
    try {
      const { container } = render(<MapView />);
      const ids = () =>
        [...container.querySelectorAll("[data-army]")]
          .map((e) => e.getAttribute("data-army"))
          .sort();
      expect(ids()).toEqual(["farm", "own"]);
      const box = screen.getByRole("checkbox", {
        name: "Show farming",
      }) as HTMLInputElement;
      expect(box.checked).toBe(true);

      fireEvent.click(box);
      expect(box.checked).toBe(false);
      expect(ids()).toEqual(["own"]);

      fireEvent.click(box);
      expect(box.checked).toBe(true);
      expect(ids()).toEqual(["farm", "own"]);
    } finally {
      useGameStore.setState({ hideFarmOnMap: hide });
    }
  });
  it("the toolbar folds away to a bare glyph and comes back without losing a switch", () => {
    setup((g, home) => {
      addCmd(g, {
        id: "own",
        type: "attack",
        originId: home.id,
        targetId: makeBotVillage(g).id,
        units: { axe: 3 },
      });
    });
    const { container } = render(<MapView />);
    const toggle = container.querySelector(
      "[data-map-toolbar-toggle]",
    ) as HTMLButtonElement;
    // The switch is a glyph at the end of the row, not a header: on a phone the bar is a third of the screen, and a
    // header would hand back the space folding it saves.
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("checkbox", { name: "Own attacks" })).toBeTruthy();

    // Open, it is a full-width panel spanning the room.
    const fieldset = () =>
      container.querySelector("[data-map-display]") as HTMLElement;
    expect(fieldset().className).toContain("panel");

    fireEvent.click(toggle);
    expect(useUiStore.getState().prefs.mapToolbarOpen).toBe(false);
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(fieldset().querySelector("input[type=checkbox]")).toBeNull();
    expect(fieldset().textContent).not.toContain("My village");
    // Folded, it must not be the same full-width panel with the switches pulled out: an empty wooden box the width of the
    // map reads as a broken bar, not a folded one. It shrinks to the glyph and keeps no frame.
    expect(fieldset().className).not.toContain("panel");
    expect(fieldset().className).toContain("w-fit");
    // Folding the switches must not fold the map: the army keeps its line and arrowhead.
    expect(
      [...container.querySelectorAll("[data-army]")].map((e) =>
        e.getAttribute("data-army"),
      ),
    ).toEqual(["own"]);

    fireEvent.click(toggle);
    expect(useUiStore.getState().prefs.mapToolbarOpen).toBe(true);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("checkbox", { name: "Own attacks" })).toBeTruthy();
    expect(container.querySelector('input[placeholder="x|y"]')).toBeTruthy();
  });
  it("player colours and the village size limit sit in the same panel and still drive the map", () => {
    let barb!: { x: number; y: number };
    setup((g, home) => {
      const bot = makeBotVillage(g);
      addCmd(g, {
        id: "own",
        type: "attack",
        originId: home.id,
        targetId: bot.id,
        units: { axe: 3 },
      });
      // A barbarian in view, above the limit the test then sets.
      barb = { x: home.x + 2, y: home.y + 2 };
      g.villages[`v:${barb.x}:${barb.y}`] = {
        ...structuredClone(bot),
        id: `v:${barb.x}:${barb.y}`,
        x: barb.x,
        y: barb.y,
        ownerId: null,
        points: 500,
      };
    });
    const { container } = render(<MapView />);
    const panel = container.querySelector("[data-map-display]")!;
    expect(panel.textContent).toContain("Player colours");
    expect(panel.textContent).toContain("Hide villages");
    // A village tinted in its owner's colour loses the tint once colours are off.
    const tinted = () => !!container.querySelector("[data-roof]");
    expect(tinted()).toBe(true);
    fireEvent.click(screen.getByRole("checkbox", { name: "Player colours" }));
    expect(tinted()).toBe(false);
    // A barbarian above the limit fades out; the villages of players do not.
    const opacity = (x: number, y: number) =>
      container.querySelector<HTMLElement>(
        `[data-tile="${x}|${y}"] [data-pixel-village]`,
      )!.style.opacity;
    const bot = Object.values(useGameStore.getState().game!.villages).find(
      (v) => v.ownerId,
    )!;
    expect(opacity(barb.x, barb.y)).toBe("1");
    fireEvent.change(
      screen.getByRole("spinbutton", { name: /Hide villages/ }),
      { target: { value: "100" } },
    );
    expect(opacity(barb.x, barb.y)).toBe("0.45");
    expect(opacity(bot.x, bot.y)).toBe("1");
  });
});
