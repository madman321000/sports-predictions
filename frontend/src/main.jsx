import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import "./style.css";
import DataExplorer from "./DataExplorer";
import { get } from "./api";
const label = (s) => s.replaceAll("_", " ");
const pct = (x) => `${(x * 100).toFixed(1)}%`;
function Chart({ data }) {
  const bins = data.filter((b) => b.games);
  return (
    <svg
      viewBox="0 0 400 240"
      role="img"
      aria-label="Calibration: predicted versus observed home-win probability"
    >
      <path d="M45 15V205H375" fill="none" stroke="#a3b2c4" />
      <path d="M45 205L375 15" stroke="#a3b2c4" strokeDasharray="5 5" />
      {[0, 0.5, 1].map((t) => (
        <g key={t}>
          <text x={35} y={208 - t * 190} textAnchor="end">
            {pct(t)}
          </text>
          <text x={45 + t * 330} y="225" textAnchor="middle">
            {pct(t)}
          </text>
        </g>
      ))}
      <polyline
        points={bins
          .map(
            (b) =>
              `${45 + b.mean_probability * 330},${205 - b.observed_home_win_rate * 190}`,
          )
          .join(" ")}
        fill="none"
        stroke="#21b6a8"
        strokeWidth="3"
      />
      {bins.map((b, i) => (
        <circle
          key={i}
          cx={45 + b.mean_probability * 330}
          cy={205 - b.observed_home_win_rate * 190}
          r="5"
          fill="#21b6a8"
        >
          <title>{`${b.games} games: predicted ${pct(b.mean_probability)}, observed ${pct(b.observed_home_win_rate)}`}</title>
        </circle>
      ))}
    </svg>
  );
}
function App() {
  const [runs, setRuns] = useState([]),
    [id, setId] = useState(""),
    [run, setRun] = useState(null),
    [candidate, setCandidate] = useState(""),
    [page, setPage] = useState(0),
    [rows, setRows] = useState(null),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    setError("");
    get("/api/runs", c.signal)
      .then((x) => {
        setRuns(x);
        setId((v) => (x.some((r) => r.id === v) ? v : x[0]?.id || ""));
      })
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => c.abort();
  }, [retry]);
  useEffect(() => {
    if (!id) return;
    const c = new AbortController();
    setRun(null);
    setRows(null);
    setError("");
    get(`/api/runs/${encodeURIComponent(id)}`, c.signal)
      .then((x) => {
        setRun(x);
        setCandidate(Object.keys(x.candidates)[0]);
        setPage(0);
      })
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => c.abort();
  }, [id, retry]);
  useEffect(() => {
    if (!run || !candidate) return;
    const c = new AbortController();
    setRows(null);
    get(
      `/api/runs/${encodeURIComponent(id)}/predictions?candidate=${encodeURIComponent(candidate)}&offset=${page * 25}&limit=25`,
      c.signal,
    )
      .then(setRows)
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => c.abort();
  }, [id, run, candidate, page, retry]);
  const result = run?.candidates[candidate];
  return (
    <>
      <header>
        <a className="brand" href="/">
          ◈ COURTSIDE <span>MODEL LAB</span>
        </a>
        <span className="status">Historical research · NBA / NFL</span>
      </header>
      <main>
        <div className="intro">
          <div>
            <p className="eyebrow">MEASURE. COMPARE. LEARN.</p>
            <h1>
              A clearer view of
              <br />
              every prediction.
            </h1>
            <p className="subtitle">
              Explore team baselines, probability calibration and historical
              outcomes.
            </p>
          </div>
          <div className="notice">
            <strong>Development backtests</strong>
            <p>
              These are historical predictions on observed seasons, not live
              forecasts or a fresh holdout.
            </p>
          </div>
        </div>
        {error && (
          <div role="alert" className="error">
            {error} Check that the API is running and the browser address
            matches FRONTEND_ORIGIN.{" "}
            <button onClick={() => setRetry(retry + 1)}>Retry</button>
          </div>
        )}
        <div className="filters">
          <label>
            Published run
            <select value={id} onChange={(e) => setId(e.target.value)}>
              {runs.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.title}
                </option>
              ))}
            </select>
          </label>
          <label>
            Candidate
            <select
              value={candidate}
              onChange={(e) => {
                setCandidate(e.target.value);
                setPage(0);
              }}
            >
              {Object.keys(run?.candidates || {}).map((k) => (
                <option key={k}>{k}</option>
              ))}
            </select>
          </label>
          <span>
            {run?.league} {run?.training_season && `${run.training_season} → `}
            {run?.test_season}
          </span>
        </div>
        {!runs.length && !error && (
          <div className="panel">
            No published runs yet. Publish a completed backtest to begin.
          </div>
        )}
        {id && !run && !error && <p role="status">Loading run…</p>}
        {result && (
          <>
            <div className="metrics">
              {[
                ["Test games", result.metrics.games],
                ["Accuracy", pct(result.metrics.accuracy)],
                ["Log loss", result.metrics.log_loss.toFixed(4)],
                ["Brier score", result.metrics.brier_score.toFixed(4)],
              ].map(([name, val]) => (
                <div className="metric" key={name}>
                  <span>{name}</span>
                  <strong>{val}</strong>
                </div>
              ))}
            </div>
            <div className="grid">
              <section className="panel">
                <p className="eyebrow">PROBABILITY CHECK</p>
                <h2>Calibration</h2>
                <p>
                  Predicted probability (horizontal) vs. observed home wins
                  (vertical).
                </p>
                <Chart data={result.calibration || []} />
                <p className="caption">
                  Dashed line: perfect calibration. Small bins are noisy.
                </p>
              </section>
              <section className="panel">
                <p className="eyebrow">SAME GAMES, DIFFERENT MODELS</p>
                <h2>Candidate comparison</h2>
                <div className="scroll">
                  <table>
                    <thead>
                      <tr>
                        <th>Candidate</th>
                        <th>Log loss ↓</th>
                        <th>Accuracy</th>
                      </tr>
                    </thead>
                    <tbody>
                      {Object.entries(run.candidates).map(([k, v]) => (
                        <tr key={k}>
                          <td>{label(k)}</td>
                          <td>{v.metrics.log_loss.toFixed(4)}</td>
                          <td>{pct(v.metrics.accuracy)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <p className="caption">
                  Lower log loss and Brier score are better. Accuracy uses a
                  fixed 50% threshold.
                </p>
              </section>
            </div>
            <section className="panel">
              <div className="sectiontitle">
                <div>
                  <p className="eyebrow">PREDICTION EXPLORER</p>
                  <h2>Historical games</h2>
                </div>
                <span>{rows?.total ?? "…"} predictions</span>
              </div>
              <div className="scroll">
                <table>
                  <thead>
                    <tr>
                      <th>Date</th>
                      <th>Game / ESPN teams</th>
                      <th>Final score</th>
                      <th>Home win probability</th>
                      <th>Actual winner</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows?.items.map((r) => (
                      <tr key={r.game_id}>
                        <td>{r.date}</td>
                        <td>
                          {r.game_id}
                          <small>
                            {r.home_team_id
                              ? `${r.home_team_id} (home) vs ${r.away_team_id}`
                              : "Team IDs unavailable"}
                          </small>
                        </td>
                        <td>
                          {r.home_score != null
                            ? `${r.home_score} – ${r.away_score}`
                            : "—"}
                        </td>
                        <td>
                          <meter min="0" max="1" value={r.probability} />
                          {pct(r.probability)}
                        </td>
                        <td>
                          <span className="pill">
                            {r.home_win ? "Home" : "Away"}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {!rows && <p role="status">Loading predictions…</p>}
              <div className="pagination">
                <button
                  disabled={!page || !rows}
                  onClick={() => setPage(page - 1)}
                >
                  ← Previous
                </button>
                <span>Page {page + 1}</span>
                <button
                  disabled={!rows || (page + 1) * 25 >= rows.total}
                  onClick={() => setPage(page + 1)}
                >
                  Next →
                </button>
              </div>
            </section>
            <details className="panel">
              <summary>Scope and limitations</summary>
              <ul>
                {run.limitations?.map((x) => (
                  <li key={x}>{x}</li>
                ))}
              </ul>
              <p>Published {run.generated_at}</p>
            </details>
          </>
        )}
      </main>
      <footer>
        COURTSIDE / Sports Predictions · Learning from the game, one season at a
        time.
      </footer>
    </>
  );
}
function Shell() {
  const [view, setView] = useState("models");
  return (
    <>
      <nav className="view-tabs" aria-label="Dashboard views">
        <button
          aria-pressed={view === "models"}
          onClick={() => setView("models")}
        >
          Model results
        </button>
        <button
          aria-pressed={view === "stats"}
          onClick={() => setView("stats")}
        >
          Sports data
        </button>
      </nav>
      {view === "models" ? <App /> : <DataExplorer />}
    </>
  );
}
createRoot(document.getElementById("root")).render(<Shell />);
