package game

import "time"

type Game struct {
	ExternalID         string
	HomeTeamExternalID string
	AwayTeamExternalID string
	StartsAt           time.Time
	Status             string
	HomeScore          *int
	AwayScore          *int
	Season             int
	SeasonType         int
	Week               *int
	ObservedAt         time.Time
}
