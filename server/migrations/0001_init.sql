CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    provider   TEXT NOT NULL,             -- 'google' | 'apple' | 'dev'
    subject    TEXT NOT NULL,             -- provider's stable user ID
    email      TEXT NOT NULL DEFAULT '',  -- lowercased, verified by provider
    name       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, subject)
);
CREATE INDEX users_email ON users (email);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,          -- sha256 of the token; tokens are never stored
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user ON sessions (user_id);

CREATE TABLE events (
    id        BIGSERIAL PRIMARY KEY,
    user_id   BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title     TEXT NOT NULL,
    starts_at TIMESTAMPTZ,                -- timed items
    day       DATE,                       -- all-day items, in the user's calendar
    CHECK ((starts_at IS NULL) <> (day IS NULL))
);
CREATE INDEX events_user ON events (user_id);

-- owner lets viewer see (read-only) the owner's calendar.
CREATE TABLE shares (
    owner_id   BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    viewer_id  BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, viewer_id)
);
CREATE INDEX shares_viewer ON shares (viewer_id);

CREATE TABLE invites (
    token_hash TEXT PRIMARY KEY,
    owner_id   BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);
