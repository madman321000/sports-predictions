import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { get } from "./api";
import "./style.css";

const percent = (value) => `${(value * 100).toFixed(1)}%`;
function App() {
  const [league, setLeague] = useState("NBA"),
    [data, setData] = useState(null),
    [error, setError] = useState(""),
    [refresh, setRefresh] = useState(0);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  useEffect(() => {
    const timer = setInterval(() => setRefresh((x) => x + 1), 60000);
    return () => clearInterval(timer);
  }, []);
  useEffect(() => {
    const c = new AbortController();
    setError("");
    get(`/api/today?${new URLSearchParams({ league, timezone })}`, c.signal)
      .then(setData)
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => c.abort();
  }, [league, timezone, refresh]);
  const current = data?.league === league ? data : null;
  return (
    <>
      <header>
        <a className="brand" href="/">
          ◈ COURTSIDE <span>TODAY</span>
        </a>
        <span className="status">NBA / NFL · Pregame predictions</span>
      </header>
      <main>
        <div className="intro">
          <div>
            <p className="eyebrow">THE DAY'S MATCHUPS</p>
            <h1>
              Today’s games.
              <br />A view before the whistle.
            </h1>
            <p className="subtitle">
              Game times and today’s date use your timezone:{" "}
              <strong>{timezone}</strong>.
            </p>
          </div>
          <div className="notice">
            <strong>Experimental win probabilities</strong>
            <p>
              Locally trained models. No player predictions yet. A missing
              prediction means the model or its required history is not ready.
            </p>
          </div>
        </div>
        <div className="today-toolbar">
          <div className="league-tabs" role="group" aria-label="League">
            {["NBA", "NFL"].map((l) => (
              <button
                key={l}
                aria-pressed={league === l}
                onClick={() => {
                  if (l === league) return;
                  setLeague(l);
                  setData(null);
                }}
              >
                {l}
              </button>
            ))}
          </div>
          <button onClick={() => setRefresh((x) => x + 1)}>
            Refresh games
          </button>
        </div>
        {error && (
          <div role="alert" className="error">
            {error === "Failed to fetch"
              ? "Cannot reach the API. Check that it is running and the browser address matches its configured origin."
              : error}
          </div>
        )}
        {!current && !error && <p role="status">Loading today’s schedule…</p>}
        {current && (
          <>
            <div className="day-heading">
              <h2>
                {league} · {current.date}
              </h2>
              <span className="caption">
                Schedule updated{" "}
                {new Date(current.updated_at).toLocaleTimeString()} · checks
                every minute
              </span>
            </div>
            {!current.model_id && (
              <p className="panel">
                No {league} model installed. Games are available; probabilities
                will appear after a locally trained model is installed.
              </p>
            )}
            {["warming", "stale", "unavailable"].includes(
              current.history_status,
            ) && (
              <p role="status" className="panel">
                {current.history_status === "warming"
                  ? "Preparing this season’s team history. The first load can take tens of minutes; keep the API running."
                  : "Team history is temporarily unavailable or out of date. New predictions are paused."}{" "}
                ESPN requests are cached and paced.
              </p>
            )}
            {!current.games.length ? (
              <section className="panel">
                <h2>No {league} games today</h2>
                <p>Check the other league or come back tomorrow.</p>
              </section>
            ) : (
              <div className="match-grid">
                {current.games.map((g) => (
                  <GameCard key={g.id} game={g} />
                ))}
              </div>
            )}
            <p className="caption">
              Regular-season win models require five prior games per team.
              NBA/NFL exhibition and preseason games are excluded. In-progress
              or final games show a pregame prediction only if this API recorded
              one before the start. Results are not used to invent predictions
              after a game begins.
            </p>
          </>
        )}
      </main>
      <footer>
        Training and evaluation stay local. This site serves game schedules and
        model inference only.
      </footer>
    </>
  );
}
function GameCard({ game: g }) {
  const p = g.prediction;
  const started = g.status !== "scheduled";
  return (
    <article className="panel match-card">
      <div className="match-meta">
        <time dateTime={g.starts_at}>
          {new Date(g.starts_at).toLocaleTimeString([], {
            hour: "numeric",
            minute: "2-digit",
          })}
        </time>
        <span className="pill">{g.status.replaceAll("_", " ")}</span>
      </div>
      <div className="match-team">
        <div>
          <span className="caption">AWAY</span>
          <h3>{g.away_team}</h3>
        </div>
        <strong>{started ? (g.away_score ?? "—") : "—"}</strong>
      </div>
      <div className="match-team">
        <div>
          <span className="caption">HOME</span>
          <h3>{g.home_team}</h3>
        </div>
        <strong>{started ? (g.home_score ?? "—") : "—"}</strong>
      </div>
      {p ? (
        <div className="prediction">
          <p className="eyebrow">PREGAME WIN PROBABILITY</p>
          <div className="probabilities">
            <span>
              {g.away_team}
              <strong>{percent(p.away_probability)}</strong>
            </span>
            <span>
              {g.home_team}
              <strong>{percent(p.home_probability)}</strong>
            </span>
          </div>
          <meter
            aria-label={`${g.home_team} home-win probability`}
            min="0"
            max="1"
            value={p.home_probability}
          />
          <p className="caption">
            Model {p.model_id} · calculated{" "}
            {new Date(p.generated_at).toLocaleTimeString()}
            <br />
            {p.away_history_games} away-team / {p.home_history_games} home-team
            prior games
          </p>
        </div>
      ) : (
        <div className="prediction">
          <strong>Prediction unavailable</strong>
          <p>{g.unavailable}</p>
        </div>
      )}
    </article>
  );
}
createRoot(document.getElementById("root")).render(<App />);
