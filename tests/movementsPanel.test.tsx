// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { setLocale, t } from '../src/i18n/i18n';
import type { GameState, Village } from '../src/engine/types';
import { militaryCommands } from '../src/engine/commands';
import { selectActiveVillage, useGameStore } from '../src/state/gameStore';
import { villageLabelOf } from '../src/ui/villageName';
import { MovementsPanel } from '../src/ui/views/MovementsPanel';
import { SidePanel } from '../src/ui/views/VillageView';
import { activeVillage, addCommand, addOwnVillage, newTestGame } from './helpers';

const HOUR = 3_600_000;
let g: GameState;
let first: Village;
let second: Village;

beforeEach(() => {
  setLocale('en');
  g = newTestGame();
  first = activeVillage(g);
  second = addOwnVillage(g, first.x + 3, first.y + 2, 'Second');
  g.activeVillageId = second.id;
});
afterEach(() => cleanup());

/** The "Movements" card as the village view renders it: hidden when the village has no movements. */
function MovementsCard() {
  const n = useGameStore((s) => militaryCommands(s.game!, selectActiveVillage(s).id).length);
  if (n === 0) return null;
  return <SidePanel title={t('village.movements')}><MovementsPanel /></SidePanel>;
}

const show = () => {
  useGameStore.setState({ game: g, phase: 'ready' });
  return render(<MovementsCard />);
};

describe('movements panel', () => {
  it('is hidden when the village has no movements', () => {
    const { container } = show();
    expect(container.textContent).toBe('');
  });

  it('is shown with the "Movements" title while troops are on the move', () => {
    addCommand(g, { type: 'attack', originId: second.id, targetId: first.id, units: { spear: 40 } });
    show();
    expect(screen.getByText(t('village.movements'))).toBeTruthy();
  });

  it('lists an own attack with its route and troops, home to target', () => {
    addCommand(g, { type: 'attack', originId: second.id, targetId: first.id, units: { spear: 40 } });
    show();
    expect(screen.getByText('Attack')).toBeTruthy();
    expect(containerText()).toContain(villageLabelOf(g, second));
    expect(containerText()).toContain(villageLabelOf(g, first));
    expect(screen.getAllByTitle('Spearman').length).toBeGreaterThan(0);
  });

  it('lists an own support as moving to the target village', () => {
    addCommand(g, { type: 'support', originId: second.id, targetId: first.id, units: { sword: 12 } });
    show();
    expect(screen.getByText('Support')).toBeTruthy();
    expect(containerText()).toContain(villageLabelOf(g, first));
  });

  it('lists a return under the village it goes home to', () => {
    addCommand(g, { type: 'return', originId: first.id, targetId: second.id, units: { spear: 7 } });
    show();
    expect(screen.getByText('Return')).toBeTruthy();
    expect(containerText()).toContain(villageLabelOf(g, second));
  });

  it('does not list other villages\' movements or trade', () => {
    const third = addOwnVillage(g, first.x + 9, first.y + 8, 'Third');
    addCommand(g, { type: 'attack', originId: third.id, targetId: second.id, units: { spear: 5 } });
    addCommand(g, { type: 'trade', originId: second.id, targetId: first.id });
    const { container } = show();
    expect(container.textContent).toBe('');
  });
});

function containerText(): string {
  return document.body.textContent ?? '';
}
