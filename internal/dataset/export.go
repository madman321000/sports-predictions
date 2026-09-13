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

// ExportOptions controls quality overrides and replacement of existing exports.
type ExportOptions struct{ AllowIncomplete, Overwrite bool }

// Export stages a complete dataset before publishing it. Overwrite requires an
// existing managed export for the same league, season and season type.
func Export(path string, scope Scope, data Snapshot, options ExportOptions) (Report, error) {
	if err := scope.Validate(); err != nil {
		return Report{}, err
	}
	report := Audit(scope, data)
	if !report.Ready && !options.AllowIncomplete {
		return report, fmt.Errorf("quality checks failed; inspect the quality report or explicitly use -allow-incomplete")
	}
	destination, err := filepath.Abs(path)
	if err != nil {
		return report, err
	}
	lock, err := os.OpenFile(destination+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return report, fmt.Errorf("lock export destination: %w", err)
	}
	_ = lock.Close()
	defer func() { _ = os.Remove(destination + ".lock") }()
	exists, err := validateDestination(destination, scope, options.Overwrite)
	if err != nil {
		return report, err
	}
	path, err = os.MkdirTemp(filepath.Dir(destination), ".export-stage-")
	if err != nil {
		return report, err
	}
	defer func() { _ = os.RemoveAll(path) }()
	games := [][]string{{"game_id", "league", "season", "season_type", "starts_at_utc", "home_team_id", "away_team_id", "home_score", "away_score", "player_import_complete"}}
	for _, g := range data.Games {
		if g.Status == "final" {
			games = append(games, []string{g.ID, scope.League, strconv.Itoa(scope.Season), strconv.Itoa(scope.SeasonType), g.StartsAt.UTC().Format(time.RFC3339Nano), g.Home, g.Away, optionalInt(g.HomeScore), optionalInt(g.AwayScore), strconv.FormatBool(g.PlayersComplete)})
		}
	}
	players := [][]string{{"game_id", "player_id", "player_name", "team_id", "category", "position", "jersey", "did_not_play", "starter", "stats_json", "participation_status"}}
	for _, p := range data.Players {
		starter := ""
		if p.Starter != nil {
			starter = strconv.FormatBool(*p.Starter)
		}
		players = append(players, []string{p.GameID, p.PlayerID, p.Name, p.Team, p.Category, p.Position, p.Jersey, strconv.FormatBool(p.DidNotPlay), starter, p.Stats, Participation(scope.League, p)})
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
	if exists {
		backup, err := os.MkdirTemp(filepath.Dir(destination), ".export-backup-")
		if err != nil {
			return report, err
		}
		if err := os.Remove(backup); err != nil {
			return report, err
		}
		if err := os.Rename(destination, backup); err != nil {
			return report, err
		}
		if err := os.Rename(path, destination); err != nil {
			if restoreErr := os.Rename(backup, destination); restoreErr != nil {
				return report, fmt.Errorf("publish: %w; restore failed: %v; previous export retained at %s", err, restoreErr, backup)
			}
			return report, err
		}
		if err := os.RemoveAll(backup); err != nil {
			return report, fmt.Errorf("export published; remove previous export at %s: %w", backup, err)
		}
	} else if err := os.Rename(path, destination); err != nil {
		return report, err
	}
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

func validateDestination(path string, scope Scope, overwrite bool) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !overwrite {
		return false, fmt.Errorf("export already exists; use -overwrite to replace it")
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("overwrite requires a real export directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	if len(entries) != 3 {
		return false, fmt.Errorf("refusing to overwrite a directory containing unexpected files")
	}
	for _, entry := range entries {
		if entry.Name() != "games.csv" && entry.Name() != "players.csv" && entry.Name() != "report.json" {
			return false, fmt.Errorf("unexpected export file %s", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return false, err
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("export files must be regular files")
		}
	}
	body, err := os.ReadFile(filepath.Join(path, "report.json"))
	if err != nil {
		return false, err
	}
	var previous Report
	if err := json.Unmarshal(body, &previous); err != nil {
		return false, fmt.Errorf("read previous report: %w", err)
	}
	if previous.SchemaVersion < 1 || previous.SchemaVersion > 3 || previous.Scope.League != scope.League || previous.Scope.Season != scope.Season || previous.Scope.SeasonType != scope.SeasonType {
		return false, fmt.Errorf("previous export has an incompatible schema or season scope")
	}
	return true, nil
}
