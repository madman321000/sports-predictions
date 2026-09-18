package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/madman321000/sports-predictions/internal/stats"
)

type StatsRepository struct{ pool *pgxpool.Pool }

func NewStatsRepository(pool *pgxpool.Pool) *StatsRepository { return &StatsRepository{pool: pool} }

// Browse returns public columns only. All filters are bound parameters, never SQL fragments.
func (r *StatsRepository) Browse(ctx context.Context, resource string, f stats.Filter) (stats.Page, error) {
	args := []any{}
	bind := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	var query, order string
	switch resource {
	case "leagues":
		query = "SELECT id::text AS id,name,abbreviation,sport FROM leagues"
		order = "abbreviation,id"
	case "seasons":
		query = `SELECT g.season,g.season_type,COUNT(*) AS games,
 COUNT(*) FILTER(WHERE g.status='final') AS final_games,
 MIN(g.starts_at) AS first_game,MAX(g.starts_at) AS last_game
 FROM games g JOIN leagues l ON l.id=g.league_id WHERE g.provider='espn' AND l.abbreviation=` + bind(f.League) + ` GROUP BY g.season,g.season_type`
		order = "season DESC,season_type"
	default:
		scope := `g.provider='espn' AND l.abbreviation=` + bind(f.League) + ` AND g.season=` + bind(f.Season) + ` AND g.season_type=` + bind(f.SeasonType)
		switch resource {
		case "teams":
			query = `SELECT t.id::text AS id,t.external_id,t.name,t.abbreviation FROM teams t WHERE EXISTS (
 SELECT 1 FROM games g JOIN leagues l ON l.id=g.league_id WHERE ` + scope + ` AND (g.home_team_id=t.id OR g.away_team_id=t.id))`
			order = "name,id"
		case "games":
			if f.TeamID != 0 {
				v := bind(f.TeamID)
				scope += ` AND (g.home_team_id=` + v + ` OR g.away_team_id=` + v + `)`
			}
			if f.Status != "" {
				scope += ` AND g.status=` + bind(f.Status)
			}
			query = `SELECT g.id::text AS id,g.external_id,g.starts_at,g.status,g.week,
 h.name AS home_team,a.name AS away_team,g.home_score,g.away_score,
 EXISTS(SELECT 1 FROM player_complete_games c WHERE c.id=g.id) AS player_stats_complete
 FROM games g JOIN leagues l ON l.id=g.league_id JOIN teams h ON h.id=g.home_team_id JOIN teams a ON a.id=g.away_team_id WHERE ` + scope
			order = "starts_at DESC,id"
		case "players", "player-games":
			if f.TeamID != 0 {
				scope += ` AND s.team_id=` + bind(f.TeamID)
			}
			if f.PlayerID != 0 {
				scope += ` AND s.player_id=` + bind(f.PlayerID)
			}
			if f.GameID != 0 {
				scope += ` AND s.game_id=` + bind(f.GameID)
			}
			if f.Category != "" {
				scope += ` AND s.category=` + bind(f.Category)
			}
			if f.Search != "" {
				scope += ` AND strpos(lower(p.name),lower(` + bind(f.Search) + `))>0`
			}
			joins := ` FROM player_game_stats s JOIN player_complete_games g ON g.id=s.game_id JOIN leagues l ON l.id=g.league_id JOIN players p ON p.id=s.player_id JOIN teams t ON t.id=s.team_id WHERE ` + scope
			if resource == "players" {
				query = `SELECT p.id::text AS id,p.external_id,p.name,COUNT(DISTINCT g.id) AS games,
 string_agg(DISTINCT t.name,', ' ORDER BY t.name) AS teams` + joins + ` GROUP BY p.id,p.external_id,p.name`
				order = "name,id"
			} else {
				query = `SELECT p.id::text AS player_id,p.external_id AS player_external_id,p.name AS player,
 g.id::text AS game_id,g.external_id AS game_external_id,g.starts_at,t.name AS team,
 s.category,s.position,s.jersey,s.did_not_play,s.starter,s.stats` + joins
				order = "starts_at DESC,game_id,player_id,category"
			}
		case "totals":
			// The totals view exposes only additive metrics from complete imported games.
			scope = `v.provider='espn' AND l.abbreviation=$1 AND v.season=$2 AND v.season_type=$3`
			if f.TeamID != 0 {
				scope += ` AND v.team_id=` + bind(f.TeamID)
			}
			if f.PlayerID != 0 {
				scope += ` AND v.player_id=` + bind(f.PlayerID)
			}
			if f.Category != "" {
				scope += ` AND v.category=` + bind(f.Category)
			}
			query = `SELECT v.player_id::text AS player_id,p.name AS player,v.team_id::text AS team_id,t.name AS team,
 v.category,v.metric,v.total,v.games_with_metric FROM player_season_totals v
 JOIN leagues l ON l.id=v.league_id JOIN players p ON p.id=v.player_id JOIN teams t ON t.id=v.team_id WHERE ` + scope
			order = "player,player_id,team_id,category,metric"
		default:
			return stats.Page{}, fmt.Errorf("unsupported statistics resource")
		}
	}
	limit, offset := bind(f.Limit), bind(f.Offset)
	query = `WITH results AS (` + query + `), page AS (SELECT * FROM results ORDER BY ` + order + ` LIMIT ` + limit + ` OFFSET ` + offset + `) SELECT (SELECT COUNT(*) FROM results),COALESCE(jsonb_agg(to_jsonb(page) ORDER BY ` + order + `),'[]'::jsonb) FROM page`
	page := stats.Page{Limit: f.Limit, Offset: f.Offset}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&page.Total, &page.Items)
	return page, err
}
