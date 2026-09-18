import React, { useEffect, useState } from "react";
import { get } from "./api";

const resources = {
  games: "Games",
  teams: "Teams",
  players: "Players",
  "player-games": "Player box scores",
  totals: "Season totals",
};
const display = (value) => (value == null ? "—" : String(value));

export default function DataExplorer() {
  const [leagues, setLeagues] = useState([]),
    [league, setLeague] = useState("NBA");
  const [seasons, setSeasons] = useState([]),
    [seasonKey, setSeasonKey] = useState("");
  const [teams, setTeams] = useState([]),
    [team, setTeam] = useState("");
  const [resource, setResource] = useState("games"),
    [search, setSearch] = useState(""),
    [category, setCategory] = useState(""),
    [status, setStatus] = useState("");
  const [player, setPlayer] = useState(null),
    [game, setGame] = useState(null);
  const [page, setPage] = useState(0),
    [loaded, setLoaded] = useState(null),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0),
    [loadingSeasons, setLoadingSeasons] = useState(true);
  const [season, seasonType] = seasonKey.split(":");
  const queryKey = JSON.stringify([
    league,
    seasonKey,
    team,
    resource,
    search,
    category,
    status,
    player?.id,
    game?.id,
    page,
    retry,
  ]);
  const result = loaded?.key === queryKey ? loaded.data : null;
  useEffect(() => {
    const c = new AbortController();
    get("/api/stats/leagues?limit=100", c.signal)
      .then((x) => setLeagues(x.items))
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => c.abort();
  }, [retry]);
  useEffect(() => {
    const c = new AbortController();
    setError("");
    setLoadingSeasons(true);
    setSeasons([]);
    setSeasonKey("");
    setTeams([]);
    setLoaded(null);
    get(`/api/stats/seasons?league=${league}&limit=100`, c.signal)
      .then((x) => {
        setSeasons(x.items);
        const first = x.items.find((s) => s.season_type === 2) || x.items[0];
        setSeasonKey(first ? `${first.season}:${first.season_type}` : "");
        setLoadingSeasons(false);
      })
      .catch((e) => {
        if (e.name !== "AbortError") {
          setError(e.message);
          setLoadingSeasons(false);
        }
      });
    return () => c.abort();
  }, [league, retry]);
  useEffect(() => {
    if (!seasonKey) return;
    const c = new AbortController();
    setTeams([]);
    get(
      `/api/stats/teams?league=${league}&season=${season}&season_type=${seasonType}&limit=100`,
      c.signal,
    )
      .then((x) => setTeams(x.items))
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => c.abort();
  }, [league, seasonKey, retry]);
  useEffect(() => {
    if (!seasonKey) return;
    const c = new AbortController();
    setLoaded(null);
    setError("");
    const q = new URLSearchParams({
      league,
      season,
      season_type: seasonType,
      limit: "25",
      offset: String(page * 25),
    });
    if (team && resource !== "teams") q.set("team_id", team);
    if (resource === "players" && search) q.set("q", search);
    if (resource === "games" && status) q.set("status", status);
    if (resource === "player-games" || resource === "totals") {
      if (player) q.set("player_id", player.id);
      if (category) q.set("category", category);
    }
    if (resource === "player-games" && game) q.set("game_id", game.id);
    get(`/api/stats/${resource}?${q}`, c.signal)
      .then((data) => setLoaded({ key: queryKey, data }))
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => c.abort();
  }, [
    league,
    seasonKey,
    team,
    resource,
    search,
    category,
    status,
    player,
    game,
    page,
    retry,
  ]);
  const reset = () => {
    setTeam("");
    setPlayer(null);
    setGame(null);
    setPage(0);
    setSearch("");
    setCategory("");
    setStatus("");
  };
  const chosen = seasons.find(
    (x) => `${x.season}:${x.season_type}` === seasonKey,
  );
  const chooseResource = (value) => {
    setResource(value);
    setPage(0);
    setGame(null);
    setCategory("");
    if (value !== "totals" && value !== "player-games") setPlayer(null);
  };
  return (
    <>
      <header>
        <a className="brand" href="/">
          ◈ COURTSIDE <span>DATA EXPLORER</span>
        </a>
      </header>
      <main>
        <h1>Explore the season.</h1>
        <p className="subtitle">
          Imported schedules, teams and historical player statistics.
        </p>
        <p className="notice">
          Player records describe box-score appearances, including DNP
          entries—not complete rosters. Totals include only complete imported
          games and supported additive statistics, and stay separate for each
          team.
        </p>
        {error && (
          <div role="alert" className="error">
            {error}
            <button onClick={() => setRetry((x) => x + 1)}>Retry data</button>
          </div>
        )}
        <div className="filters">
          <label>
            League
            <select
              value={league}
              onChange={(e) => {
                reset();
                setSeasonKey("");
                setLeague(e.target.value);
              }}
            >
              {(leagues.length
                ? leagues
                : [{ abbreviation: "NBA" }, { abbreviation: "NFL" }]
              )
                .filter((l) => ["NBA", "NFL"].includes(l.abbreviation))
                .map((l) => (
                  <option key={l.abbreviation}>{l.abbreviation}</option>
                ))}
            </select>
          </label>
          <label>
            Season
            <select
              value={seasonKey}
              onChange={(e) => {
                reset();
                setSeasonKey(e.target.value);
              }}
            >
              <option value="">Select season</option>
              {seasons.map((s) => (
                <option
                  key={`${s.season}:${s.season_type}`}
                  value={`${s.season}:${s.season_type}`}
                >
                  {s.season} ·{" "}
                  {{
                    1: "Preseason",
                    2: "Regular season",
                    3: "Postseason",
                    4: "Other",
                  }[s.season_type] || `Type ${s.season_type}`}
                </option>
              ))}
            </select>
          </label>
          <label>
            Browse
            <select
              value={resource}
              onChange={(e) => chooseResource(e.target.value)}
            >
              {Object.entries(resources).map(([key, name]) => (
                <option key={key} value={key}>
                  {name}
                </option>
              ))}
            </select>
          </label>
        </div>
        {loadingSeasons && !error && <p role="status">Loading seasons…</p>}
        {!loadingSeasons && !seasonKey && !error && (
          <p>No imported seasons for this league.</p>
        )}
        {chosen && (
          <section className="panel season-summary">
            <h2>
              {league} {season} coverage
            </h2>
            <p>
              {chosen.final_games} final / {chosen.games} stored games. This
              counts stored records, not proof of a complete schedule.
            </p>
            <progress
              aria-label="Final games among stored games"
              value={chosen.final_games}
              max={chosen.games || 1}
            />
          </section>
        )}
        {seasonKey && (
          <>
            <div className="filters">
              {resource !== "teams" && (
                <label>
                  Team
                  <select
                    value={team}
                    onChange={(e) => {
                      setTeam(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">All teams</option>
                    {teams.map((t) => (
                      <option key={t.id} value={t.id}>
                        {t.name}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {resource === "players" && (
                <label>
                  Player name
                  <input
                    aria-label="Player name"
                    value={search}
                    maxLength={100}
                    onChange={(e) => {
                      setSearch(e.target.value);
                      setPage(0);
                    }}
                    placeholder="Search by name"
                  />
                </label>
              )}
              {resource === "games" && (
                <label>
                  Game status
                  <select
                    value={status}
                    onChange={(e) => {
                      setStatus(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">All statuses</option>
                    {[
                      "final",
                      "scheduled",
                      "in_progress",
                      "postponed",
                      "canceled",
                      "suspended",
                      "delayed",
                    ].map((s) => (
                      <option key={s}>{s}</option>
                    ))}
                  </select>
                </label>
              )}
              {(resource === "player-games" || resource === "totals") && (
                <label>
                  Stat category
                  <input
                    value={category}
                    maxLength={80}
                    onChange={(e) => {
                      setCategory(e.target.value);
                      setPage(0);
                    }}
                    placeholder="All, or general / passing / rushing"
                  />
                </label>
              )}
            </div>
            {(player || game) && (
              <p className="scope-selection">
                {player && `Player: ${player.name} `}
                {game && `Game: ${game.name}`}{" "}
                <button
                  onClick={() => {
                    setPlayer(null);
                    setGame(null);
                    setPage(0);
                  }}
                >
                  Clear selection
                </button>
              </p>
            )}
            <section className="panel">
              <h2>{resources[resource]}</h2>
              {!result && !error && <p role="status">Loading data…</p>}
              {result && (
                <>
                  <p>
                    {result.total} matching{" "}
                    {resource === "totals"
                      ? "metric rows"
                      : resource === "player-games"
                        ? "category rows"
                        : "records"}
                  </p>
                  {!result.items.length ? (
                    <p>
                      No records match these filters. Player data requires
                      complete game imports.
                    </p>
                  ) : (
                    <div className="scroll">
                      <table>
                        <thead>
                          <tr>
                            {{
                              teams: ["Team", "Abbreviation", "ESPN ID"],
                              games: [
                                "Date (UTC)",
                                "Home",
                                "Away",
                                "Score",
                                "Status",
                                "Player stats",
                              ],
                              players: [
                                "Player",
                                "ESPN ID",
                                "Observed teams",
                                "Box-score games",
                                "Details",
                              ],
                              "player-games": [
                                "Date (UTC)",
                                "Player",
                                "Team",
                                "Game",
                                "Category",
                                "Participation",
                                "Statistics",
                              ],
                              totals: [
                                "Player",
                                "Team",
                                "Category",
                                "Metric",
                                "Total",
                                "Games with metric",
                              ],
                            }[resource].map((h) => (
                              <th key={h} scope="col">
                                {h}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {result.items.map((row, i) => (
                            <tr key={i}>
                              {resource === "teams" && (
                                <>
                                  <td>{row.name}</td>
                                  <td>{row.abbreviation}</td>
                                  <td>{row.external_id}</td>
                                </>
                              )}
                              {resource === "games" && (
                                <>
                                  <td>{row.starts_at.slice(0, 10)}</td>
                                  <td>{row.home_team}</td>
                                  <td>{row.away_team}</td>
                                  <td>
                                    {display(row.home_score)} –{" "}
                                    {display(row.away_score)}
                                  </td>
                                  <td>{row.status}</td>
                                  <td>
                                    <button
                                      disabled={!row.player_stats_complete}
                                      onClick={() => {
                                        setGame({
                                          id: row.id,
                                          name: `${row.home_team} vs ${row.away_team}`,
                                        });
                                        setPlayer(null);
                                        setResource("player-games");
                                        setCategory("");
                                        setPage(0);
                                      }}
                                    >
                                      View box score
                                    </button>
                                  </td>
                                </>
                              )}
                              {resource === "players" && (
                                <>
                                  <td>{row.name}</td>
                                  <td>{row.external_id}</td>
                                  <td>{row.teams}</td>
                                  <td>{row.games}</td>
                                  <td>
                                    <button
                                      onClick={() => {
                                        setPlayer({
                                          id: row.id,
                                          name: row.name,
                                        });
                                        setGame(null);
                                        setResource("player-games");
                                        setPage(0);
                                      }}
                                    >
                                      Player games
                                    </button>{" "}
                                    <button
                                      onClick={() => {
                                        setPlayer({
                                          id: row.id,
                                          name: row.name,
                                        });
                                        setGame(null);
                                        setResource("totals");
                                        setCategory("");
                                        setPage(0);
                                      }}
                                    >
                                      Player totals
                                    </button>
                                  </td>
                                </>
                              )}
                              {resource === "player-games" && (
                                <>
                                  <td>{row.starts_at.slice(0, 10)}</td>
                                  <td>{row.player}</td>
                                  <td>{row.team}</td>
                                  <td>{row.game_external_id}</td>
                                  <td>{row.category}</td>
                                  <td>
                                    {row.did_not_play
                                      ? "DNP"
                                      : row.starter === true
                                        ? "Starter"
                                        : "Other / unknown"}
                                  </td>
                                  <td>
                                    <details>
                                      <summary>View statistics</summary>
                                      <dl className="stat-values">
                                        {Object.entries(row.stats).map(
                                          ([k, v]) => (
                                            <div key={k}>
                                              <dt>{k}</dt>
                                              <dd>{display(v)}</dd>
                                            </div>
                                          ),
                                        )}
                                      </dl>
                                    </details>
                                  </td>
                                </>
                              )}
                              {resource === "totals" && (
                                <>
                                  <td>{row.player}</td>
                                  <td>{row.team}</td>
                                  <td>{row.category}</td>
                                  <td>{row.metric}</td>
                                  <td>{display(row.total)}</td>
                                  <td>{row.games_with_metric}</td>
                                </>
                              )}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                  <div className="pagination">
                    <button
                      disabled={page === 0}
                      onClick={() => setPage((x) => x - 1)}
                    >
                      Previous data
                    </button>
                    <span>Page {page + 1}</span>
                    <button
                      disabled={(page + 1) * 25 >= result.total}
                      onClick={() => setPage((x) => x + 1)}
                    >
                      Next data
                    </button>
                  </div>
                </>
              )}
            </section>
          </>
        )}
      </main>
    </>
  );
}
