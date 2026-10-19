CREATE TABLE IF NOT EXISTS users (
    id         text PRIMARY KEY,
    email      text NOT NULL,
    name       text NOT NULL DEFAULT '',
    picture    text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash bytea PRIMARY KEY,
    user_id    text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_user_id ON sessions (user_id);

CREATE TABLE IF NOT EXISTS items (
    user_id    text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    collection text NOT NULL,
    id         text NOT NULL,
    data       jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, collection, id)
);
