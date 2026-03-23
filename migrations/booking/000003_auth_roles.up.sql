CREATE TYPE user_role AS ENUM ('admin', 'client', 'specialist');

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role user_role NOT NULL,
    client_id BIGINT UNIQUE REFERENCES clients(id) ON DELETE SET NULL,
    specialist_id BIGINT UNIQUE REFERENCES specialists(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_role_refs_chk CHECK (
        (role = 'admin' AND client_id IS NULL AND specialist_id IS NULL)
        OR
        (role = 'client' AND client_id IS NOT NULL AND specialist_id IS NULL)
        OR
        (role = 'specialist' AND specialist_id IS NOT NULL AND client_id IS NULL)
    )
);

CREATE INDEX idx_users_role ON users (role);
