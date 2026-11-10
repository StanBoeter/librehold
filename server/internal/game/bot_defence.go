package game

import (
	"math"
	"slices"
	"sort"
)

// Bot defence: reacting to incoming attacks with support, dodges and the wall.

// garrisonUnitIDs are the units that never leave on a raid: the ones whose defence is what they are for. A raid is
// what a village can spare, never its garrison. The spear is not among them — 10 attack against 15 defence makes it
// the cheap all-rounder a village farms with long before it can afford swords. This is the one list: what is not in
// it is what a raid is made of, so the two can never drift apart.
var garrisonUnitIDs = []UnitID{USword, UArcher, UMountedArcher, UHeavyCav, UPaladin}

// supportUnitIDs are the units a village answers an attack with when a neighbour's support is what would save it.
// The spear defends well enough to be worth marching next door, unlike the haulers.
var supportUnitIDs = []UnitID{USpear, USword, UArcher, UHeavyCav}

// unitPop is the farm population a group of units takes.
func unitPop(u UnitCounts) int {
	pop := 0
	for _, id := range UnitIDs {
		pop += u[id] * UnitDefs[id].Pop
	}
	return pop
}

func villagePop(v *Village) int { return unitPop(UnitCounts(v.Units)) }

// guardPool is the floor a raiding village keeps at home regardless of what may go out: HomeGuardPop, so even a
// village with nothing but raiders shows a scout somebody. No share of the army is held back — the raiders exist to
// raid, and keeping a quarter of them home would only make the bot farm worse than the player it imitates.
func guardPool() float64 {
	return BotConfig.HomeGuardPop
}

// garrisonPop is what may never leave: the defensive units, or at least the guard floor, whichever is more.
func garrisonPop(v *Village) float64 {
	held := 0
	for _, u := range garrisonUnitIDs {
		held += v.Units[u] * UnitDefs[u].Pop
	}
	return math.Max(float64(held), guardPool())
}

// raidable is the population that may still leave the village. Below zero the garrison is already short.
func raidable(v *Village) float64 { return float64(villagePop(v)) - garrisonPop(v) }

// offUnit: only what is not the garrison may leave. A noble is never raiding — it is the one unit a conquest is
// built around, and it leaves only on an operation the bot plans as one.
func offUnit(u UnitID) bool { return u != UNoble && !slices.Contains(garrisonUnitIDs, u) }

// offScouts is how many scouts a raid may take: all but the share kept back for looking at its own doorstep.
func offScouts(v *Village) int {
	keep := max(1, int(math.Ceil(BotConfig.HomeScoutShare*float64(v.Units[UScout]))))
	return max(0, v.Units[UScout]-keep)
}

// withinGuard keeps the part of a group that may leave without breaking the garrison, trimming what will not fit.
// It reports false when nothing can go at all, which is the case for a village that is still all defence.
func withinGuard(v *Village, u UnitCounts) (UnitCounts, bool) {
	room := raidable(v)
	if room <= 0 {
		return UnitCounts{}, false
	}
	var out UnitCounts
	for _, id := range UnitIDs {
		if offUnit(id) {
			out[id] = u[id]
		}
	}
	if out[UScout] > 0 {
		out[UScout] = min(out[UScout], offScouts(v))
	}
	if out.Total() == 0 {
		return UnitCounts{}, false
	}
	for _, id := range []UnitID{UCatapult, URam, UAxe, ULightCav, USpear, UScout} {
		if float64(unitPop(out)) <= room {
			break
		}
		over := float64(unitPop(out)) - room
		cut := min(out[id], int(math.Ceil(over/float64(UnitDefs[id].Pop))))
		out[id] -= cut
	}
	if out.Total() == 0 || float64(unitPop(out)) > room {
		return UnitCounts{}, false
	}
	return out, true
}

func defenceOf(u UnitCounts) float64 {
	d := 0.0
	for _, id := range UnitIDs {
		d += float64(u[id]) * UnitDefs[id].Defense
	}
	return d
}

// estimateWave: the sender's known offence from memory, else a full nuke for siege/nobles, 10 % for the rest.
func estimateWave(w World, botID PlayerID, c *Command) float64 {
	if k := brainOf(w, botID).Memory.Known[c.OriginID]; k != nil && k.Units != nil {
		if a := unitsAttack(*k.Units); a > 0 {
			return a
		}
	}
	switch LabelIncoming(w, c) {
	case "scout":
		return 0
	case "siege", "noble":
		return BotConfig.NukeAttack
	}
	return 0.1 * BotConfig.NukeAttack
}

func defensiveUnits(v *Village) UnitCounts {
	var out UnitCounts
	for _, u := range supportUnitIDs {
		out[u] = v.Units[u]
	}
	return out
}

func stationedDefence(v *Village) float64 {
	d := defenceOf(UnitCounts(v.Units))
	for _, sup := range v.SupportFrom {
		d += defenceOf(sup)
	}
	return d
}

type helperVillage struct {
	v     *Village
	units UnitCounts
}

// underAttack: an enemy attack on the village is still on its way. A village that is itself about to be hit does
// not send its army next door — that is how both villages fall to one wave.
func underAttack(w World, botID PlayerID, v *Village) bool {
	for _, c := range w.CommandsTo(v.ID) {
		if c.Type != "attack" {
			continue
		}
		if owner := w.Village(c.OriginID); owner != nil && owner.OwnerID == Ref(botID) {
			continue
		}
		return true
	}
	return false
}

// supportShare is the most of a village's defence another village may draw on: the rest stays where it is, because
// the helper may be the next wave's target and neither village knows what else is coming.
const supportShare = 0.7

// supportable is the part of a village's garrison it will lend to a neighbour.
func supportable(v *Village) UnitCounts {
	var out UnitCounts
	for _, u := range supportUnitIDs {
		out[u] = int(math.Floor(float64(v.Units[u]) * supportShare))
	}
	return out
}

// tryStack: support from defensive villages that makes it before the hit; sent when together they hold 1.5 × wave.
// A helper that is under attack itself sends nothing: it needs its army more than its neighbour does.
func tryStack(w World, botID PlayerID, v *Village, c *Command, need float64) bool {
	b := brainOf(w, botID)
	inbound := 0.0
	for _, x := range w.CommandsTo(v.ID) {
		if x.Type == "support" && x.ArrivesAt < c.ArrivesAt {
			inbound += defenceOf(x.Units)
		}
	}
	have := stationedDefence(v) + inbound
	if have >= need {
		return true
	}
	var helpers []helperVillage
	for _, o := range PlayerVillages(w, botID) {
		role, ok := b.Roles[o.ID]
		if !ok {
			role = RoleMixed
		}
		if o.ID == v.ID || role != RoleDef {
			continue
		}
		if underAttack(w, botID, o) {
			continue
		}
		units := supportable(o)
		if units.Total() > 0 && w.Now()+TravelTimeForUnits(w, o, v.X, v.Y, units) < c.ArrivesAt {
			helpers = append(helpers, helperVillage{o, units})
		}
	}
	sort.SliceStable(helpers, func(i, j int) bool {
		return Distance(helpers[i].v.X, helpers[i].v.Y, v.X, v.Y) < Distance(helpers[j].v.X, helpers[j].v.Y, v.X, v.Y)
	})
	var plan []helperVillage
	for _, h := range helpers {
		if have >= need {
			break
		}
		plan = append(plan, h)
		have += defenceOf(h.units)
	}
	if have < need {
		return false
	}
	for _, h := range plan {
		if len(CheckSend(w, botID, h.v.ID, v.X, v.Y, h.units, "support").Blockers) == 0 {
			act(w, botID, "send", sendArgs(h.v.ID, v.X, v.Y, h.units, "support"), func() error {
				_, err := SendCommand(w, botID, h.v.ID, v.X, v.Y, h.units, "support", SendOpts{})
				return err
			})
		}
	}
	return true
}

// trySnipe snipes a hetman train with support from the nearest village that makes it in time.
func trySnipe(w World, botID PlayerID, v *Village, train Train) {
	cmds := slices.Clone(train.Commands)
	sort.SliceStable(cmds, func(i, j int) bool { return cmds[i].ArrivesAt < cmds[j].ArrivesAt })
	if len(cmds) < 2 {
		return
	}
	mid := cmds[0].ArrivesAt + (cmds[1].ArrivesAt-cmds[0].ArrivesAt)/2
	var best *helperVillage
	for _, o := range PlayerVillages(w, botID) {
		units := supportable(o)
		if o.ID == v.ID || units.Total() == 0 || underAttack(w, botID, o) ||
			w.Now()+TravelTimeForUnits(w, o, v.X, v.Y, units) > mid {
			continue
		}
		if best == nil || Distance(o.X, o.Y, v.X, v.Y) < Distance(best.v.X, best.v.Y, v.X, v.Y) {
			best = &helperVillage{o, units}
		}
	}
	if best != nil {
		ids := []string{cmds[0].ID, cmds[1].ID}
		args := logArgs{"commandIds": ids, "fromVillageId": best.v.ID, "units": best.units}
		actSoft(w, botID, "snipe", args, func() error {
			_, err := PlanSnipe(w, botID, ids, best.v.ID, best.units)
			return err
		})
	}
}

// PlanDefence, at every think (and phone glance): for each attacked village — stack when support makes it and
// holds, else dodge (weak: half the time it "does not notice"); a strong bot snipes hetman trains; before the hit
// it spends resources above 30 % of storage. One reaction per village at a time.
func PlanDefence(w World, botID PlayerID) {
	b := w.Brain(botID)
	if b == nil {
		return
	}
	mine := PlayerVillages(w, botID)
	own := map[string]bool{}
	for _, v := range mine {
		own[v.ID] = true
	}
	strong := b.Skill >= BotConfig.StrongSkill
	for _, v := range mine {
		var incoming []*Command
		for _, c := range w.CommandsTo(v.ID) {
			if c.Type == "attack" && !own[c.OriginID] {
				incoming = append(incoming, c)
			}
		}
		if len(incoming) == 0 {
			continue
		}
		storage := VillageStorageCapacity(v)
		if v.Resources.Wood > 0.3*storage || v.Resources.Clay > 0.3*storage || v.Resources.Iron > 0.3*storage {
			PlanBuilding(w, botID, v)
			PlanRecruitment(w, botID, v)
		}
		if strong {
			for _, train := range IncomingTrains(w, v.ID) {
				sniping := slices.ContainsFunc(w.ScheduledSendsOf(botID), func(s *ScheduledSend) bool {
					return s.Type == "support" && s.X == v.X && s.Y == v.Y
				})
				if !sniping {
					trySnipe(w, botID, v, train)
				}
			}
		}
		var worst *Command
		worstPower := 0.0
		for _, c := range incoming {
			p := estimateWave(w, botID, c)
			if p > worstPower || (p == worstPower && worst != nil && c.ArrivesAt < worst.ArrivesAt) {
				worst, worstPower = c, p
			}
		}
		if worst == nil || worstPower == 0 {
			continue
		}
		if tryStack(w, botID, v, worst, 1.5*worstPower) {
			continue
		}
		dodging := slices.ContainsFunc(w.ScheduledSendsOf(botID), func(s *ScheduledSend) bool {
			return s.OriginID == v.ID && s.CancelAt != nil
		})
		if dodging {
			continue
		}
		// A weak bot "does not notice" half the attacks — fixed per wave, not drawn at every think.
		if strong || Mulberry32(HiddenSeed(w, "notice", botID, worst.ID))() < 0.5 {
			c := worst
			actSoft(w, botID, "dodge", logArgs{"commandId": c.ID}, func() error { _, err := PlanDodge(w, botID, c.ID); return err })
		}
	}
}
