// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { SimulatorTab } from '../src/ui/views/SimulatorView';
import { RallyPointView } from '../src/ui/views/RallyPointView';
import { ReportsView } from '../src/ui/views/ReportsView';
import { useGameStore } from '../src/state/gameStore';
import { useUiStore } from '../src/state/uiStore';
import { setLocale } from '../src/i18n/i18n';
import { newDefaultGame } from './helpers';
import { stubServer } from './serverStub';

/** The battle the stub answers with, shaped like the server's own SimResult. */
const ANSWER = {
  kind: 'battle', outcome: 'attackerWon', attackPower: 4200, defensePower: 1800, luck: 0, morale: 1,
  attackerLosses: { sword: 120 }, attackerSurvivors: { sword: 380 },
  defenderLosses: { spear: 200 }, defenderSurvivors: {},
  wallBefore: 5, wallAfter: 3, haul: { wood: 300, clay: 200, iron: 100 }, haulCapacity: 4000,
  attackerCost: 12000, defenderCost: 8000,
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
  setLocale('en');
  useUiStore.setState({ simPrefill: null, rallyPrefill: null });
});
afterEach(() => {
  cleanup();
  useGameStore.getState().shutdown();
  vi.unstubAllGlobals();
  setLocale('pl');
});

describe('the battle simulator', () => {
  it('is reachable as a tab of the rally point', async () => {
    await boot();
    render(<RallyPointView />);
    expect(screen.getByRole('button', { name: 'Simulator' })).toBeTruthy();
  });

  it('asks the server and shows who won and what each side lost', async () => {
    const { paths } = await boot({ 'POST /api/v1/worlds/1/simulate': () => ({ status: 200, body: ANSWER }) });
    render(<SimulatorTab />);
    // An army to send: two swordsmen is enough for the stub to be asked.
    fireInput(atk('sword'), '500');
    screen.getByRole('button', { name: 'Simulate' }).click();

    expect(await screen.findByText('The attacker wins')).toBeTruthy();
    expect(paths.some((p) => p.includes('/simulate'))).toBe(true);
    // The losses are shown per unit, as the report-side tables do.
    expect(screen.getByText('Lost')).toBeTruthy();
    expect(screen.getByText('120')).toBeTruthy();
  });

  it('shows a wall that fell and the haul', async () => {
    await boot({ 'POST /api/v1/worlds/1/simulate': () => ({ status: 200, body: ANSWER }) });
    render(<SimulatorTab />);
    fireInput(atk('sword'), '500');
    screen.getByRole('button', { name: 'Simulate' }).click();
    expect(await screen.findByText(/The wall falls from 5 to 3/)).toBeTruthy();
    expect(screen.getByText(/Haul back: 600 resources/)).toBeTruthy();
  });

  it('does not ask the server before there are troops', async () => {
    const { paths } = await boot();
    render(<SimulatorTab />);
    const run = screen.getByRole('button', { name: 'Simulate' }) as HTMLButtonElement;
    expect(run.disabled).toBe(true);
    expect(paths.filter((p) => p.includes('/simulate')).length).toBe(0);
  });

  it('fills the attacker in from the village the player is looking at', async () => {
    await boot({}, (g) => { g.villages[g.activeVillageId]!.units.sword = 77; });
    render(<SimulatorTab />);
    screen.getByRole('button', { name: 'Fill in my troops' }).click();
    await waitFor(() => {
      expect((atk('sword') as HTMLInputElement).value).toBe('77');
    });
  });

  it('takes a battle handed to it from a report, once', async () => {
    await boot();
    useUiStore.getState().openSimulator({ attacker: { axe: 300 }, defender: { spear: 150 }, wall: 7 });
    render(<SimulatorTab />);
    await waitFor(() => {
      expect((atk('axe') as HTMLInputElement).value).toBe('300');
    });
    expect((document.getElementById('sim-def-spear') as HTMLInputElement).value).toBe('150');
    expect((screen.getByLabelText('Wall level') as HTMLInputElement).value).toBe('7');
    // One-off: the tab does not reset itself on the next render.
    expect(useUiStore.getState().simPrefill).toBe(null);
  });

  it('says so when the server refuses', async () => {
    await boot({ 'POST /api/v1/worlds/1/simulate': () => ({ status: 500, body: { code: 'boom' } }) });
    render(<SimulatorTab />);
    fireInput(atk('sword'), '10');
    screen.getByRole('button', { name: 'Simulate' }).click();
    expect(await screen.findByText(/The simulation did not answer/)).toBeTruthy();
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
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!;
  setter.call(input, value);
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

describe('the button on a battle report', () => {
  it('hands the battle to the simulator, both armies and the wall it was fought at', async () => {
    // The report has to be in the game the store loads, not in a copy made afterwards.
    await boot({}, (g) => {
      const home = g.villages[g.activeVillageId]!;
      g.reports.push({
        id: 'r1', time: g.now, type: 'attack', originId: home.id, targetId: 'v:9:9',
        origin: { id: home.id, name: 'Home', x: home.x, y: home.y, ownerId: home.ownerId ?? undefined },
        target: { id: 'v:9:9', name: 'Barbarians', x: 9, y: 9 },
        attacker: { units: { axe: 40 }, losses: { axe: 5 } }, defender: { units: { spear: 20 }, losses: { spear: 20 } },
        luck: 0.1, morale: 1, wallBefore: 6, wallAfter: 4, outcome: 'attackerWon', read: false,
      });
    });
    render(<ReportsView />);
    fireEvent.click(await screen.findByText(/→/));
    fireEvent.click(screen.getByRole('button', { name: 'Fill in simulator' }));

    const handed = useUiStore.getState().simPrefill!;
    expect(handed.attacker).toEqual({ axe: 40 });
    expect(handed.defender).toEqual({ spear: 20 });
    expect(handed.wall).toBe(6);
    expect(handed.luck).toBeCloseTo(0.1);
    expect(useUiStore.getState().view).toBe('rally');
  });
});

describe('the army fields', () => {
  it('shows the unit as an icon only, with the name kept for screen readers', async () => {
    await boot();
    const { container } = render(<SimulatorTab />);
    // The icon is there, and no visible text names the unit — twelve names twice over was the whole width.
    const icon = container.querySelector('img[alt="Swordsman"]');
    expect(icon).not.toBeNull();
    // Every bit of text in the fields is inside `.sr-only`, so nothing names the unit on screen. (textContent
    // would still contain it, which is the point of sr-only, so it is the wrapper that has to be checked.)
    // Only the army fields: the settings on the right name themselves on screen, as they should.
    for (const el of container.querySelectorAll('label[for^="sim-atk-"] > span, label[for^="sim-def-"] > span')) {
      const named = (el.textContent ?? '').trim() !== '';
      if (named) expect(el.classList.contains('sr-only')).toBe(true);
    }
    // The name still reaches the accessible label, and it says which side it belongs to.
    expect(screen.getByLabelText('Attacker Swordsman')).toBeTruthy();
    expect(screen.getByLabelText('Defender Swordsman')).toBeTruthy();
  });

  it('puts each unit on one row: its icon, then the attacker, then the defender', async () => {
    await boot();
    render(<SimulatorTab />);
    // happy-dom reports every rect as 0x0, so measuring positions proves nothing here. What carries the layout is
    // the grid order: every unit must own three consecutive cells. A wrapper element inside the grid emits two
    // items per unit instead of three and the columns drift a cell per row — which is exactly what went wrong.
    const grid = document.getElementById('sim-atk-sword')!.closest('.grid')!;
    // Three header cells, then three per unit.
    expect(grid.children.length).toBe(39);
    // The headings are the first row of the same grid, so they sit over the columns they name. In a grid of their
    // own they could not: an empty cell collapses to 0 px while the icon column does not, and both headings drifted
    // right of the fields under them.
    expect(grid.children[0]!.textContent).toBe('');
    expect(grid.children[1]!.textContent).toBe('Attacker');
    expect(grid.children[2]!.textContent).toBe('Defender');
    // Column two holds the attacker's fields, column three the defender's — the headings are in those same columns.
    expect(grid.children[4]!.querySelector('input')!.id).toBe('sim-atk-spear');
    expect(grid.children[5]!.querySelector('input')!.id).toBe('sim-def-spear');
    const rows: [string, string][] = [
      ['Spearman', 'spear'], ['Swordsman', 'sword'], ['Axeman', 'axe'], ['Archer', 'archer'],
      ['Scout', 'scout'], ['Light cavalryman', 'lightCav'], ['Mounted archer', 'mountedArcher'],
      ['Heavy cavalryman', 'heavyCav'], ['Ram', 'ram'], ['Catapult', 'catapult'],
      ['Knight', 'paladin'], ['Hetman', 'noble'],
    ];
    rows.forEach(([name, unit], i) => {
      const icon = grid.children[i * 3 + 3]!;
      const atk = grid.children[i * 3 + 4]!;
      const def = grid.children[i * 3 + 5]!;
      expect(icon.querySelector('img')!.getAttribute('alt')).toBe(name);
      expect(atk.querySelector('input')!.id).toBe(`sim-atk-${unit}`);
      expect(def.querySelector('input')!.id).toBe(`sim-def-${unit}`);
    });
  });
});

describe('the simulator layout', () => {
  it('keeps the army fields narrow enough to leave the settings room beside them', async () => {
    await boot();
    render(<SimulatorTab />);
    // A stretched input was the whole problem: the two columns must not claim the row.
    const widths = [...document.querySelectorAll<HTMLInputElement>('input[id^="sim-atk-"], input[id^="sim-def-"]')]
      .map((el) => el.className);
    expect(widths.length).toBe(24);
    expect(widths.every((c) => /\bw-16\b/.test(c) && !/\bw-full\b/.test(c))).toBe(true);
  });

  it('puts the wall, hideout, luck and morale in one column beside the armies on a wide screen', async () => {
    await boot();
    render(<SimulatorTab />);
    const armies = document.querySelector('input[id="sim-atk-sword"]')!.closest('.panel')!;
    const settings = document.getElementById('sim-wall')!.closest('.panel')!;
    // Side by side: one flex row that only turns into a column below the large breakpoint.
    const row = armies.parentElement!;
    expect(row).toBe(settings.parentElement);
    expect(row.className).toContain('lg:flex-row');
    expect(row.className).toContain('flex-col');
    // And every setting lives in that one panel, not scattered across a four-column strip.
    for (const id of ['sim-hide', 'sim-morale', 'sim-luck', 'sim-catapult']) {
      expect(document.getElementById(id)!.closest('.panel')).toBe(settings);
    }
  });

});

describe('the settings panel width', () => {
  it('never stretches across the page, stacked or beside the armies', async () => {
    await boot();
    render(<SimulatorTab />);
    const settings = document.getElementById('sim-wall')!.closest('.panel') as HTMLElement;
    // The fields inside are 64px, so the panel has no reason to grow: `w-full` alone let it run the whole width
    // when the settings drop under the armies.
    expect(settings.className).toContain('max-w-[280px]');
    expect(/\bw-full\b/.test(settings.className)).toBe(true);
    // And the same cap holds beside the armies on a wide screen.
    expect(settings.className).toContain('shrink-0');
  });
});

describe('the result summary lines', () => {
  const haulAnswer = (haul: { wood: number; clay: number; iron: number }) => ({
    'POST /api/v1/worlds/1/simulate': () => ({ status: 200, body: { ...ANSWER, haul } }),
  });

  it('says nothing about a haul when the raid brings nothing home', async () => {
    await boot(haulAnswer({ wood: 0, clay: 0, iron: 0 }));
    render(<SimulatorTab />);
    fireInput(atk('sword'), '500');
    screen.getByRole('button', { name: 'Simulate' }).click();
    expect(await screen.findByText('The attacker wins')).toBeTruthy();
    // "Haul back: 0 resources." is noise; the wall and building lines are left out for the same reason.
    expect(screen.queryByText(/Haul back/)).toBe(null);
  });

  it('reports the haul when there is one', async () => {
    await boot(haulAnswer({ wood: 300, clay: 200, iron: 100 }));
    render(<SimulatorTab />);
    fireInput(atk('sword'), '500');
    screen.getByRole('button', { name: 'Simulate' }).click();
    expect(await screen.findByText(/Haul back: 600 resources/)).toBeTruthy();
  });
});

describe('the result does not follow the next edit', () => {
  it('keeps showing the armies the run was made with after the fields are changed', async () => {
    await boot({ 'POST /api/v1/worlds/1/simulate': () => ({ status: 200, body: ANSWER }) });
    render(<SimulatorTab />);
    fireInput(atk('sword'), '500');
    screen.getByRole('button', { name: 'Simulate' }).click();
    expect(await screen.findByText('The attacker wins')).toBeTruthy();

    const sentOf = (side: 'atk' | 'def') => {
      // The result table's "Sent" cell for that side, read from the result block, not from the inputs.
      const block = document.querySelector('[data-sim-result]')!;
      const row = [...block.querySelectorAll('tbody tr')].find((tr) =>
        tr.textContent?.startsWith(side === 'atk' ? 'Attacker' : 'Defender'));
      return row?.querySelectorAll('td')[1]?.textContent ?? '';
    };
    expect(sentOf('atk')).toContain('500');

    // Now start typing the next fight: the last answer must not rewrite itself.
    fireInput(atk('sword'), '9');
    expect((atk('sword') as HTMLInputElement).value).toBe('9');
    expect(sentOf('atk')).toContain('500');
  });
});
