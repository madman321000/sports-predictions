package espn

import "fmt"

func leaguePath(league string) (string, error) {
	switch league {
	case "NBA":
		return "/apis/site/v2/sports/basketball/nba", nil
	case "NFL":
		return "/apis/site/v2/sports/football/nfl", nil
	default:
		return "", fmt.Errorf("unsupported league %q (use NBA or NFL)", league)
	}
}
