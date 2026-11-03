import { militaryCommands } from '../../engine/commands';
import type { Command } from '../../engine/types';
import { useT } from '../../i18n/i18n';
import { selectActiveVillage, useGameStore } from '../../state/gameStore';
import { Countdown } from '../components/Countdown';
import { UnitCountList } from '../components/UnitCountList';
import { PixelIcon } from '../pixelIcons';
import { commandEnds, COMMAND_ICON } from '../villageName';

/** Active military movements of the village — attacks and support on their way out, and returns heading home — with arrival countdowns. */
export function MovementsPanel() {
  const json = useGameStore((s) => JSON.stringify(
    militaryCommands(s.game!, selectActiveVillage(s).id).sort((a, b) => a.arrivesAt - b.arrivesAt),
  ));
  const cmds = JSON.parse(json) as Command[];
  const game = useGameStore((s) => s.game!);
  const t = useT();
  return (
    <div className="px-2 py-1.5 text-[12px]">
      {cmds.length === 0 ? (
        <p className="text-muted italic">{t('overview.noMoves')}</p>
      ) : (
        <ul className="divide-y divide-wood-border/40">
          {cmds.map((c) => {
            const ends = commandEnds(game, c);
            return (
              <li key={c.id} className="py-[3px]">
                <div className="flex items-center gap-1.5">
                  <PixelIcon name={COMMAND_ICON[c.type]} size={14} />
                  <span className="font-bold">{t(`rally.cmd.${c.type}`)}</span>
                  <span className="text-muted text-[11px] truncate" title={`${ends.from} → ${ends.to}`}>{ends.from} → {ends.to}</span>
                </div>
                <div className="flex items-center justify-between gap-2 pl-[20px]">
                  <UnitCountList units={c.units} size={13} />
                  <Countdown until={c.arrivesAt} />
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
