package app

import (
	"context"

	"librehold/internal/game"
)

// SimulateBattle runs a battle the player describes and changes nothing. It reads the world only for the rules that
// are the world's own — how losses are counted — so a simulator run here agrees with a raid sent in the same world.
func (s *Service) SimulateBattle(ctx context.Context, acct *Account, worldID int64, in game.SimInput) (game.SimResult, error) {
	if err := checkSimInput(in); err != nil {
		return game.SimResult{}, err
	}
	var out game.SimResult
	err := s.view(ctx, acct, worldID, func(w WorldTx, _ game.PlayerID) error {
		out = game.SimulateBattle(in, w.Config().BattleLosses)
		return nil
	})
	if err != nil {
		return game.SimResult{}, err
	}
	return out, nil
}

// checkSimInput refuses what could only be a mistake or an attempt to make the arithmetic overflow, before the world
// is locked for a read that would be thrown away.
func checkSimInput(in game.SimInput) error {
	for _, side := range []game.SimSide{in.Attacker, in.Defender} {
		for _, n := range side.Units {
			if n < 0 {
				return &ErrValidation{Field: "units", Code: "invalid"}
			}
			if n > game.MaxSimUnits {
				return &ErrValidation{Field: "units", Code: "tooMany"}
			}
		}
		if side.Wall < 0 || side.Wall > maxSimWall {
			return &ErrValidation{Field: "wall", Code: "invalid"}
		}
		if side.HideLevel < 0 || side.HideLevel > maxSimBuildingLevel {
			return &ErrValidation{Field: "hideLevel", Code: "invalid"}
		}
		for _, lvl := range side.Buildings {
			if lvl < 0 || lvl > maxSimBuildingLevel {
				return &ErrValidation{Field: "buildings", Code: "invalid"}
			}
		}
	}
	if in.Morale < 0 || in.Morale > maxSimMorale {
		return &ErrValidation{Field: "morale", Code: "invalid"}
	}
	if in.CatapultTarget != nil {
		if *in.CatapultTarget < 0 || int(*in.CatapultTarget) >= int(game.NumBuildings) {
			return &ErrValidation{Field: "catapultTarget", Code: "invalid"}
		}
	}
	return nil
}

const (
	// maxSimWall is above any wall a village can reach (20); a larger one would read as a typed-in mistake.
	maxSimWall = 30
	// maxSimBuildingLevel covers the highest building level in the game, with room to spare.
	maxSimBuildingLevel = 30
	// maxSimMorale is above 1 because a world may hand a side a bonus above the base.
	maxSimMorale = 4
)
