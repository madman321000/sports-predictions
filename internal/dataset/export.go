package dataset

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Export writes a new directory, refusing existing paths. Failure removes the
// partial directory. report.json is written last and is the completion marker.
func Export(path string, scope Scope, data Snapshot, allowIncomplete bool) (Report, error) {
	if err := scope.Validate(); err != nil {
		return Report{}, err
	}
	report := Audit(scope, data)
	if !report.Ready && !allowIncomplete {
		return report, fmt.Errorf("quality checks failed; inspect the quality report or explicitly use -allow-incomplete")
	}
	if err := os.Mkdir(path, 0755); err != nil {
		return report, fmt.Errorf("create new export directory: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(path)
		}
	}()
	games := [][]string{{"game_id", "league", "season", "season_type", "starts_at_utc", "home_team_id", "away_team_id", "home_score", "away_score", "player_import_complete"}}
	for _, g := range data.Games {
		if g.Status == "final" {
			games = append(games, []string{g.ID, scope.League, strconv.Itoa(scope.Season), strconv.Itoa(scope.SeasonType), g.StartsAt.UTC().Format(time.RFC3339Nano), g.Home, g.Away, optionalInt(g.HomeScore), optionalInt(g.AwayScore), strconv.FormatBool(g.PlayersComplete)})
		}
	}
	players := [][]string{{"game_id", "player_id", "player_name", "team_id", "category", "position", "jersey", "did_not_play", "starter", "stats_json"}}
	for _, p := range data.Players {
		starter := ""
		if p.Starter != nil {
			starter = strconv.FormatBool(*p.Starter)
		}
		players = append(players, []string{p.GameID, p.PlayerID, p.Name, p.Team, p.Category, p.Position, p.Jersey, strconv.FormatBool(p.DidNotPlay), starter, p.Stats})
	}
	for _, file := range []struct {
		name string
		rows [][]string
	}{{"games.csv", games}, {"players.csv", players}} {
		if err := writeCSV(filepath.Join(path, file.name), file.rows); err != nil {
			return report, err
		}
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return report, err
	}
	if err := os.WriteFile(filepath.Join(path, "report.json"), append(body, '\n'), 0644); err != nil {
		return report, err
	}
	success = true
	return report, nil
}
func optionalInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
func writeCSV(path string, rows [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	err = writer.WriteAll(rows)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
