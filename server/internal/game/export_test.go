package game

// JSONNear is jsonNear (combat_test.go) for the external test package.
var JSONNear = jsonNear

// NobleClaimer is nobleClaimer for the external test package.
var NobleClaimer = nobleClaimer

// CampaignScout is campaignScout for the external test package: the one place a bot's scouting party is sized.
var CampaignScout = campaignScout

// IntelStale is intelStale for the external test package.
var IntelStale = intelStale

// ScoutPartyFor is scoutPartyFor for the external test package.
var ScoutPartyFor = scoutPartyFor

// ScoutShare is scoutShare for the external test package.
var ScoutShare = scoutShare

// TroopTargetsOf is troopTargets for the external test package: the army one village is training towards.
func TroopTargetsOf(w World, v *Village, role VillageRole) map[UnitID]int {
	return troopTargets(w, v, role)
}

// RegisterTestHandler registers a handler for a test-only event kind and returns a func that removes it.
func RegisterTestHandler(kind string, h EventHandler) (restore func()) {
	prev, had := handlers[kind]
	registerHandler(kind, h)
	return func() {
		if had {
			handlers[kind] = prev
		} else {
			delete(handlers, kind)
		}
	}
}

// PopRoom is popRoom for the external test package.
var PopRoom = popRoom

// OwnedUnits is ownedUnits for the external test package.
var OwnedUnits = ownedUnits

// HarassReady is harassReady for the external test package.
var HarassReady = harassReady

// MinScoutParty is minScoutParty for the external test package.
const MinScoutParty = minScoutParty
