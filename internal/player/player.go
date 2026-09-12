// Package player describes historical player box scores independently of ESPN.
package player

import "time"

// GameRef identifies a previously imported final game. SeasonType separates
// regular season (2) from postseason (3).
type GameRef struct {
	ID                                                 int64
	ExternalID, HomeTeamExternalID, AwayTeamExternalID string
	Season, SeasonType                                 int
}

// Line preserves provider strings: ratios, missing values and clock minutes
// must not be silently converted to zero. Categories disambiguate NFL stats.
type Line struct {
	ExternalID, Name, Position, Jersey, TeamExternalID, Category string
	DidNotPlay                                                   bool
	Starter                                                      *bool
	Stats                                                        map[string]string
}

type BoxScore struct {
	// UnidentifiedDNP counts excluded nonparticipating entries with no provider ID,
	// including inactive coach-decision placeholders with missing minutes and zero stats.
	UnidentifiedDNP int
	ObservedAt      time.Time
	Lines           []Line
}
