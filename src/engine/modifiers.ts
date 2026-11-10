import { HUMAN_ID, type SideModifiers, type Village, type WorldConfig } from './types';
import { isBarbarianVillage } from './players';

const NEUTRAL: SideModifiers = { resources: 1, recruitment: 1, building: 1 };

/**
 * The resource multiplier of barbarian villages. Only resources is a setting; anything absent or left at 0 — a
 * world made before the setting existed — means 1.
 */
export function barbarianModifiersOf(cfg: WorldConfig): SideModifiers {
  const m = cfg.barbarianModifiers;
  return { ...NEUTRAL, resources: m && m.resources > 0 ? m.resources : 1 };
}

/** Multipliers for the side a village belongs to: humans, bots or barbarians. */
export function modifiersFor(cfg: WorldConfig, v: Village): SideModifiers {
  if (isBarbarianVillage(v)) return barbarianModifiersOf(cfg);
  return v.ownerId === HUMAN_ID ? cfg.humanModifiers : cfg.botModifiers;
}
