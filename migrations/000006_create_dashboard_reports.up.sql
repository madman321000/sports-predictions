-- Standalone publication table: can also be applied to a separate hosted DB.
CREATE TABLE dashboard_reports (
    id text PRIMARY KEY CHECK (id ~ '^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$'),
    body jsonb NOT NULL CHECK (jsonb_typeof(body) = 'object'),
    updated_at timestamptz NOT NULL DEFAULT now()
);
