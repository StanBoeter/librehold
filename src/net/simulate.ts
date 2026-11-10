import { UNIT_IDS, type BuildingId, type UnitId } from '../engine/types';
import { api } from './api';

/** One side of a simulated battle: its army, its research, and (for the defender) its village. */
export interface SimSide {
  units: Partial<Record<UnitId, number>>;
  research?: Partial<Record<UnitId, number>>;
  /** The defender's wall level; ignored on the attacker. */
  wall?: number;
  /** The defender's buildings, so catapults have something to aim at. */
  buildings?: Partial<Record<BuildingId, number>>;
  resources?: { wood: number; clay: number; iron: number };
  hideLevel?: number;
}

export interface SimRequest {
  attacker: SimSide;
  defender: SimSide;
  luck: number;
  morale: number;
  catapultTarget?: BuildingId;
}

/** What the server answers: the battle as a real one would resolve it, plus the losses valued in resources. */
export interface SimResult {
  kind: 'battle' | 'scout';
  outcome: 'attackerWon' | 'defenderWon';
  attackPower: number;
  defensePower: number;
  luck: number;
  morale: number;
  attackerLosses: Partial<Record<UnitId, number>>;
  attackerSurvivors: Partial<Record<UnitId, number>>;
  defenderLosses: Partial<Record<UnitId, number>>;
  defenderSurvivors: Partial<Record<UnitId, number>>;
  wallBefore: number;
  wallAfter: number;
  buildingDamage?: { building: BuildingId; from: number; to: number };
  haul: { wood: number; clay: number; iron: number };
  haulCapacity: number;
  attackerCost: number;
  defenderCost: number;
}

/** Fills a partial count up to every unit, so the table can show a row per unit without checking for holes. */
export function fullUnits(units: Partial<Record<UnitId, number>>): Record<UnitId, number> {
  const out = {} as Record<UnitId, number>;
  for (const id of UNIT_IDS) out[id] = units[id] ?? 0;
  return out;
}

/** Runs one simulated battle. It changes nothing in the world, so it is safe to send on every keystroke. */
export function simulateBattle(worldId: number, body: SimRequest): Promise<SimResult> {
  return api.post<SimResult>(`/api/v1/worlds/${worldId}/simulate`, body);
}
