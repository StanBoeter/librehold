package game

import (
	"math"
	"strconv"
)

// Bot strategy: roles, troop targets and conquest campaigns.

// phaseWeights: how many "tickets" building and campaigning get per style.
var phaseWeights = map[BotStyle][2]float64{ // [build, campaign]
	StyleAggressor:    {1, 3},
	StyleTurtle:       {3, 0.5},
	StyleFarmer:       {2, 1},
	StylePointBuilder: {3, 0.3},
	StyleOpportunist:  {1, 1.5},
}

// UpdateMood: the next days' mood — calm building or a campaign (1–4 days), drawn by style (the current phase at
// half weight), so one cannot tell when an aggressor strikes. After losing a village: 1–2 days of consolidation.
func UpdateMood(w World, botID PlayerID) {
	b := w.Brain(botID)
	if b == nil {
		return
	}
	rng := Mulberry32(HiddenSeed(w, "mood", botID, w.Now()))
	if b.LastSessionStats.VillagesLost > 0 {
		b.Mood = Mood{Phase: "consolidate", Until: w.Now() + BotDur(w, int64(math.Round((1+rng())*float64(MsPerDay))))}
		if w.Config().BotThinkEverySec > 0 {
			b.LastSessionStats.VillagesLost = 0
		}
		return
	}
	if w.Now() >= b.Mood.Until {
		weights := phaseWeights[b.Style]
		switch b.Mood.Phase {
		case "build":
			weights[0] *= 0.5
		case "campaign":
			weights[1] *= 0.5
		}
		phase := "build"
		if rng()*(weights[0]+weights[1]) < weights[1] {
			phase = "campaign"
		}
		b.Mood = Mood{Phase: phase, Until: w.Now() + BotDur(w, int64(math.Round((1+rng()*3)*float64(MsPerDay))))}
	}
	if b.Mood.Phase == "campaign" {
		PickCampaignTarget(w, botID)
	}
}

// ScoutParty is the scouts a scouting send starts from when the bot knows nothing about the target. A scouting mission
// gets through only when it beats ScoutDefenseFactor per defending scout, so this is the baseline that clears an
// ordinary home guard: at 700 it rides through a village keeping 350.
const ScoutParty = 700

// scoutBands are the sizes a bot escalates through. They straddle the 1.6×–2.7× band where luck decides, so escalating
// a caught party walks it towards a fight that is genuinely open rather than one that is decided on paper.
var scoutBands = [...]int{600, 650, 850, 1100, 1500, 2000, 3000}

// scoutPartyFor is the scouts to commit from a village against a particular target.
//
// With no history it sends what it has, up to ScoutParty. From then on it follows the last attempt: a party that came
// home keeps its size, one that was caught grows a band. It deliberately does NOT size itself to the garrison — a caught
// party learns nothing about what beat it, so whether its own scouts came home is the only feedback a bot has.
//
// A village under minScoutParty sends its whole line rather than nothing. It used to send nothing, on the argument that
// a party under the defence line is caught by any ordinary home guard and teaches nothing. That reasoning is sound and
// the rule built on it was still wrong: most roles plan fewer than 600 scouts, so most villages could never scout at
// all, at any age, in any world — measured over 30 days with 50 bots, not one scouting mission was sent while the bots
// held 29,000 scouts between them. A small party is worse than a full one; never looking is worse than both. The floor
// now only decides when a party is *comfortable*, not whether one goes.
func scoutPartyFor(have int, k *KnownVillage) int {
	if have <= 0 {
		return 0
	}
	want := ScoutParty
	if k != nil && k.LastScoutParty > 0 {
		want = k.LastScoutParty
		if k.ScoutLoss != nil && *k.ScoutLoss {
			want = nextScoutBand(k.LastScoutParty)
		}
	}
	if have < minScoutParty {
		return have
	}
	return min(have, max(want, minScoutParty))
}

// minScoutParty is the size at which a party stops being a gamble and starts being able to clear an ordinary home
// guard, so a village holding this much opens at ScoutParty and escalates from there. Below it a village sends its whole
// line and expects to be caught: that is a real cost, and it is still better than a village that never looks.
const minScoutParty = 600

// nextScoutBand is the band above have, or the top band once it runs out.
//
// The bands straddle the ScoutDefenseFactor line on purpose. Luck moves a party's effective strength by LuckRange either
// way, so a fight is only undecided while the party sits between 2/(1+LuckRange) and 2/(1-LuckRange) times the guard —
// 1.6× to 2.7×. Bands placed well above that always win and well below always lose, which makes scouting a lookup table
// again. These straddle 2×, so escalating through them crosses the range where the outcome is genuinely open.
func nextScoutBand(have int) int {
	for _, b := range scoutBands {
		if b > have {
			return b
		}
	}
	return scoutBands[len(scoutBands)-1]
}

// campaignScout sends a real scouting party at a target whose defence it does not know. Scouts only: mixed into a
// real attack they are ordinary light troops, and an attack only teaches what it survives.
func campaignScout(w World, botID PlayerID, x, y int) bool {
	var best *Village
	for _, v := range PlayerVillages(w, botID) {
		if v.Units[UScout] > 0 && (best == nil || Distance(v.X, v.Y, x, y) < Distance(best.X, best.Y, x, y)) {
			best = v
		}
	}
	if best == nil {
		return false
	}
	units := UnitCounts{UScout: scoutPartyFor(best.Units[UScout], w.Brain(botID).Memory.Known[VillageIDFor(x, y)])}
	if len(CheckSend(w, botID, best.ID, x, y, units, "attack").Blockers) > 0 {
		return false
	}
	at := w.Now() + SendJitter(w, botID, "scout:"+strconv.Itoa(x)+":"+strconv.Itoa(y))
	if pastTheBell(w, at) {
		return false
	}
	in := ScheduleSendInput{OriginID: best.ID, X: x, Y: y, Units: units, Type: "attack", Mode: "sendAt", At: at}
	return actSoft(w, botID, "schedule-send", in, func() error {
		_, err := ScheduleSend(w, botID, in)
		return err
	})
}

// PlanCampaign, in a campaign: every so often (4–10 h, at random so the rhythm cannot be read) an operation on
// the campaign target; a target of unknown defence is scouted first.
func PlanCampaign(w World, botID PlayerID) {
	b := w.Brain(botID)
	if b == nil || b.Mood.Phase != "campaign" {
		return
	}
	PickCampaignTarget(w, botID)
	target := b.Memory.Known[b.Mood.TargetVillage]
	if b.Mood.TargetVillage == "" || target == nil {
		return
	}
	opAt := int64(0)
	if b.Mood.OpAt != nil {
		opAt = *b.Mood.OpAt
	}
	rng := Mulberry32(HiddenSeed(w, "op", botID, opAt))
	gap := float64(BotDur(w, int64((4+rng()*6)*float64(MsPerHour))))
	if b.Mood.OpAt != nil && float64(w.Now()-*b.Mood.OpAt) < gap {
		return
	}
	// Unknown or out-of-date defence — look first. A player does not send nobles blind, and intel ages: the owner
	// rebuilds, troops come home and support arrives, so what was seen yesterday is a guess today. The operation waits
	// for the scouts only while there is a chance of learning something; intelHopeless lets a bot that keeps getting its
	// scouts caught attack on public data instead of never attacking at all.
	if intelStale(w, target) && !intelHopeless(w, target) {
		if campaignScout(w, botID, target.X, target.Y) {
			b.Mood.OpAt = ptrTo(w.Now())
			return
		}
	}
	LaunchOperation(w, botID, target)
	b.Mood.OpAt = ptrTo(w.Now())
}
