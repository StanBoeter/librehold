package game

var neutralModifiers = SideModifiers{Resources: 1, Recruitment: 1, Building: 1}

// IsHumanID reports whether a player id is a human's: humans are `h<accountID>` (plus an optional rejoin
// suffix), bots `bot_<i>`. Pure formulas use it to pick a village's side without touching the world.
func IsHumanID(id PlayerID) bool {
	return len(id) >= 2 && id[0] == 'h' && id[1] >= '0' && id[1] <= '9'
}

func sideModifiers(cfg *WorldConfig, owner PlayerRef) SideModifiers {
	if owner == "" {
		return neutralModifiers
	}
	if IsHumanID(PlayerID(owner)) {
		return cfg.HumanModifiers
	}
	return cfg.BotModifiers
}

// BarbarianModifiersOf is the resource multiplier of barbarian villages. Only its resource share is a setting,
// and every part left at 0 (a world made before the setting existed, or a form that posted some of it) means 1.
func BarbarianModifiersOf(cfg *WorldConfig) SideModifiers {
	m := cfg.BarbarianModifiers
	if m.Resources <= 0 {
		m.Resources = 1
	}
	if m.Recruitment <= 0 {
		m.Recruitment = 1
	}
	if m.Building <= 0 {
		m.Building = 1
	}
	return m
}

// ModifiersOfVillage picks the multipliers of a village's side: barbarians have their own resource share so their
// earnings can be sped up or slowed down without touching players or bots.
func ModifiersOfVillage(cfg *WorldConfig, v *Village) SideModifiers {
	if IsBarbarianVillage(v) {
		return BarbarianModifiersOf(cfg)
	}
	return sideModifiers(cfg, v.OwnerID)
}

// ModifiersFor returns the multipliers of the side a village belongs to: humans, bots or barbarians.
func ModifiersFor(w World, v *Village) SideModifiers {
	return ModifiersOfVillage(w.Config(), v)
}
