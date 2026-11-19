# Interface

The village view, the map, the art layers and the phone layout.

## Village illustration

The village illustration is a purely presentational layer: `src/ui/scene/sceneModel.ts` computes a scene model from `GameState` (building art stages, construction sites, smoke, alarm, training ground, research progress…), and `pixel/PixelVillageScene.tsx` assembles it into an 880×600 SVG from ready pixel art sprites (`src/assets/pixel/village/`, exported with `npm run assets:pixel` — see `ASSETS.md`). Layers are sorted by `z` depth; the wall is a ring in 8 px bands split into a back and a front half, so buildings inside are drawn between them. Building animations are frame strips with `steps()`, while villagers, guards and the gate are driven by a single `requestAnimationFrame` loop (`pixel/Walkers.tsx`).

- **Every building has three separate pieces of art** — not one rescaled. Level → tier is computed by `variantFor(level, maxLevel)` (`ceil(level/max·3)`), files `buildings/<id>-<tier>.png`. Preview of the whole village outside the game: `python3 scripts/pixel/compose.py` (usage in the script header).
- Modes: Settings → Interface → Village animations **full / limited / off**; `prefers-reduced-motion` ⇒ limited by default; paused when the tab is hidden. In limited mode the frame animations (mill wheel, ironworks fire, grain), the gate and the villagers stop; status markers stay (construction site, upgrade progress bar, ⏳ for the queue, alarm, smoke after a defeat).
- Constants: `SCENE_BALANCE` (`sceneModel.ts`) — cycle 1200–6000 ms, beat throttling 800 ms, clearing beats after a skip > 60 s, a limit of 30 animated elements, alarm/ruins windows.
- Sounds: `src/ui/sound.ts` (WebAudio, synthesised), turned on in Settings.
- Map: pixel art terrain drawn in chunks on a canvas (`src/ui/map/TerrainLayer.tsx`): a biome field from the game noise
  (`biomes.ts`, `terrain.ts`), Wang tiles, trees, peaks and meadow variety (`terrainPaint.ts`); the minimap from the
  same field (`minimap.ts`). Terrain does not affect gameplay. Art and export: `ASSETS.md`.
- **Villages on the map have six development stages** (points thresholds in `MAP.villageSizePoints`): hut → fenced
  settlement → palisade → stone wall → stronghold → castle; pixel art sprites in `src/ui/map/PixelVillage.tsx` (roofs
  in the owner's colour, slate for barbarians, a banner for a bonus and for the active village).

## Materials (UI chrome and fonts)

The UI chrome is our own pixel art as well: panels, panel headers, buttons, tabs, table headers, form fields, the
bottom bar, the scrollbar thumb, the page background and the `.frame-wood` frame of the map and the minimap are
9-slice pieces from `src/assets/pixel/ui/chrome/` (exported with `npm run assets:pixel-ui`). `src/ui/pixelSkin.css`
applies them, every rule scoped to `html.pixel` (set in `game.html`); the account site uses the same pieces
(`src/account/account.css`). The origin of every file is described in **`ASSETS.md`**, and `tests/assets.test.ts`
checks the fonts.

- `src/index.css` holds the base component styles under the skin, in flat colours from `@theme`; the map ground
  under the terrain is the flat `GRASS` colour (`MAP_GROUND` in `src/ui/map/MapArt.tsx`).
- Fonts (MedievalSharp and Cinzel, SIL OFL 1.1) are hosted locally (`src/assets/fonts/`), `game.html` does not query
  any CDN — the game works offline.

## Village view

- A 2.5D scene in a panel headed "Name (N points)" (the "−" button collapses the illustration; same as Settings → Interface → Village illustration). Art: pixel art from `src/assets/pixel/village/` (layout in `layout.json`), model: `src/ui/scene/sceneModel.ts`, scene assembly and animations: `src/ui/scene/pixel/`.
- The wall changes its look with the stage (1/3 and 2/3 of the maximum level): palisade (stages 1–2) → stone wall with towers (stage 3); the gate opens when a villager approaches. Buildings have three art stages by the same rule.
- Green badges = building level (a click highlights the row in the table). A building that is not built has only a click area (a construction site while it is being built).
- Right sidebar: Production (per hour), Units (troops in the village, "» recruit"), Current parameters. The building table shows only buildings whose requirements are met; when resources are short, the Cost column shows the time until they are gathered.

## Map — village menu and sizes

- Clicking a village on the map opens a menu next to the tile (Esc closes it). Own village: resources, population, construction, troops, incoming attacks, Village/Switch/Support/Reports/More… buttons, plus collapsed Quick build and Quick recruit sections. Quick build queues the next level of an available building; Quick recruit orders units from recruitment buildings already built in that village. Both use the same client-side checks and server orders as the village screens. Foreign village: distance and march times, the latest intel (only what the player got by scouting/attacking), the last attack, the threat from that village, our commands on the way, the 3 latest reports (a click opens them) and "all →" (Reports filtered by the village), Attack/Support/Scout/Template A/More… buttons.
- Hovering a village shows a card describing it (`src/ui/map/MapHoverCard.tsx`): name, coordinates and continent (K + hundreds of y + hundreds of x), points, the player with their ranking points, bonus, the last attack with loot, troops from scouting, wall, distance; for an own village resources, Farmstead, troops and incoming attacks. It disappears while dragging the map; on touch there is none (a tap opens the menu).
- The village icon depends on points — thresholds in `server/internal/game/data_world.go` (`MapConfig.VillageSizePoints`): < 300 hut, 300 fenced settlement, 1,000 palisade, 3,000 stone wall, 9,000 stronghold with towers, 11,000 castle. Art: `src/ui/map/PixelVillage.tsx`.
- Menu data: `src/ui/map/villageInfo.ts` (a pure function, tested in `tests/map.test.ts`).
- **The map toolbar** is a bar over the map (`[data-map-display]`, the `above` slot of `TileMap`) holding the whole of
  it — where the player is looking and what is drawn there — so no page-wide strip of buttons is stranded above the
  panel and no `Map` heading is needed: the nav tab already says which view this is. Its three groups are, in order,
  the navigation (My village, zoom, `x|y` + Go), **Armies** and **Icons**. The bar spans the room the map stands in,
  not the map window, so its controls never reflow while the corner is dragged. Its height comes off the *default*
  window: the map measures the bar and shrinks, so a bar that wraps to more lines never pushes the map below the fold.
  A window the player drags out of the corner is their call and may run past the fold — the page scrolls to reach it —
  which is why the drag ceiling leaves the bar out; otherwise the default would already sit at the ceiling and the
  corner could not pull the map any taller. The gaps are tight so all three groups still fit one line at 1280 px; a
  narrower screen wraps them, and the map shrinks to match. The panel keeps only a screen-reader legend naming the
  fieldset. Because the bar is a third of a phone screen, it folds away: the switch is a bare glyph (`−`/`+`) at the
  end of the row, not a header of its own — a header would give back the space folding it saves. Folded, the bar gives
  up its panel frame and shrinks to the glyph in the top right corner: the same full-width wooden box with its
  switches pulled out would read as a broken bar, not a folded one, and would still cost the map its height. The map
  keeps its armies, its routes and its arrowheads. The choice is a local preference (`mapToolbarOpen`). It keeps two axes apart, because they
  stack: **Armies** decides which armies are drawn *at all* (their route lines and arrowheads included), while
  **Icons** only strips the sprites and markers off the armies that are drawn — with icons off, every army above keeps
  its line and arrowhead. Next to them are "Player colours" (other players' villages in their colour) and "Hide
  villages > N pts" (villages above that many points left out); the colours and the limit are per visit. The army
  layers are filtered by `withArmyLayers` in `src/ui/map/armies.ts`; every toggle is on by default and remembered per
  browser in `lh-ui-prefs`:
  - "Show farming" takes the player's farm raids off the map, the armies and the command dots alike: raids sent from a
    farm template or by the auto-farm, and their way home. The server keeps this choice per player and world
    (`hide-farm-on-map`), and only the sender's state marks a command as a farm raid (`farm`).
  - "Icons" takes the army markers, the marching crowds and the command dots off the map; the villages stay (they are
    the map's content, not an army's icon), and so do the route lines and their arrows, so the map reads as terrain,
    villages and routes. With icons off the markers are not rendered at all, so their animation loop stops too.
  - "Own attacks" (`ownAttack`, `ownReturn`, `ownIntercept`) — the player's attacks and the armies coming home from them.
  - "Nearby armies" (`passingAttack`, `passingReturn`) — other players' armies passing within sight of the player's
    villages.
  - "Support" (`ownSupport`, `incomingSupport`, `passingSupport`) — support on its way, whoever sent it.
  - "Merchants" (`trade`) — the player's merchants on the road.
  An attack on the player (`incomingAttack`) and a threat to their armies (`threat`) have no toggle: no layer switch
  ever hides an attack on the player. The command dots are not a layer either — "Icons" is what takes them off.

## Phone (mobile layout)

The game works on screens from 360 px. Three mechanisms, all checked by `tests/mobile.test.tsx`:

- **Tabs.** Below 768 px the tabs at the top disappear and a bar appears at the bottom with the four main views
  (Village, Map, Rally Point, Reports) and a **More** button with the rest. The split is in `TABS` (`primary: true`) in
  `src/ui/layout/Nav.tsx`; the unread counters of hidden tabs add up on "More". The `0`–`9` shortcuts
  work unchanged. The bar respects `env(safe-area-inset-bottom)`.
- **Tables as cards.** The `table-stack` class (next to `table-game`) turns every row into a card below 700 px, and the
  column header moves in front of the value from the `data-label` attribute. A cell without `data-label` would be a
  bare number on a phone — a test checks this, so **a new column needs `data-label`** (or the `stack-inline` class
  for cells with only buttons, which stay on one line). Matrices (units × villages, losses in a report)
  stay tables with horizontal scrolling — splitting them into cards gains nothing.
- **Map.** The window size is computed by `mapViewport(availW, availH, zoom)` from `src/ui/map/viewport.ts` — on
  desktop it comes out at the default 15 × 13 tiles, on a phone the window shrinks to the screen width
  and the number of tiles follows from the size. The side panel and the minimap move below the map when space runs out
  (the `SIDE_COL` threshold), and the village menu slides in from the bottom instead of sticking to the tile. Dragging
  works on `pointerdown` (mouse and finger), a **two-finger pinch** changes the zoom level (`pinchZoomDelta`,
  threshold 1.3 — below that it is finger jitter), and the threshold that tells a tap from a drag is 8 px.
  The grey corner of the frame is a **resize grip** (`data-map-resize`): dragging it grows or shrinks the window
  between `MIN_W`/`MIN_H` and the room the page gives it, and the size is remembered per browser (`lh-map-window` in
  `localStorage`, `readMapWindow`/`saveMapWindow`). Once the map is dragged too wide for the side column
  (`beside`), the minimap moves under the map and is centred on the map's own width — the column under it is capped to
  the frame, so the map itself never moves. The grip only exists from the width that can hold
  the smallest window plus its minimap (`resizable`), so on a phone the map follows the screen as before.

On top of that: buttons and fields are at least 34 px with `pointer: coarse`, `overscroll-behavior-y: none` on `body`
(otherwise dragging the map down reloads the game) and `viewport-fit=cover` in `index.html`.
