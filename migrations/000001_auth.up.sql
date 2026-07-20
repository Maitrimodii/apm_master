CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE orgs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    org_id UUID NOT NULL
        REFERENCES orgs(id) ON DELETE CASCADE,

    is_email_verified BOOLEAN NOT NULL DEFAULT FALSE,

    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,

    email TEXT NOT NULL,
    password_hash TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(org_id, email)
);

CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    org_id UUID NOT NULL
        REFERENCES orgs(id) ON DELETE CASCADE,

    name TEXT NOT NULL,
    description TEXT,

    is_system BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(org_id, name)
);

CREATE TYPE permission_action AS ENUM (
    'read', 
    'write',
    'update',
    'delete', 
    'approve',
    'reject'
);

CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name TEXT NOT NULL,
    action permission_action NOT NULL,
    description TEXT,

    UNIQUE(name, action)
);

CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    org_id UUID NOT NULL
        REFERENCES orgs(id) ON DELETE CASCADE,

    name TEXT NOT NULL,

    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(org_id, name)
);

CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE CASCADE,

    token_hash TEXT NOT NULL UNIQUE,

    user_agent TEXT,
    ip_address INET,
    device_name TEXT,

    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    org_id UUID NOT NULL
        REFERENCES orgs(id) ON DELETE CASCADE,

    invited_by UUID NOT NULL
        REFERENCES users(id),

    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,

    email TEXT NOT NULL,

    token_hash TEXT NOT NULL UNIQUE,

    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invite_roles (
    invite_id UUID NOT NULL
        REFERENCES invites(id) ON DELETE CASCADE,

    role_id UUID NOT NULL
        REFERENCES roles(id) ON DELETE CASCADE,

    PRIMARY KEY (invite_id, role_id)
);

CREATE INDEX idx_invites_token_hash ON invites(token_hash);
CREATE INDEX idx_invites_org_id ON invites(org_id);

CREATE INDEX idx_users_org_email
ON users(org_id, email);

CREATE INDEX idx_roles_org_name
ON roles(org_id, name);

CREATE INDEX idx_refresh_tokens_user
ON refresh_tokens(user_id);

CREATE INDEX idx_refresh_tokens_hash
ON refresh_tokens(token_hash);

CREATE INDEX idx_api_keys_hash
ON api_keys(key_hash);

CREATE INDEX idx_role_permissions_role_id
ON role_permissions(role_id);

CREATE INDEX idx_user_roles_user_id
ON user_roles(user_id);

CREATE INDEX idx_invite_roles_invite_id
ON invite_roles(invite_id);

CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE CASCADE,

    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_password_reset_tokens_hash
ON password_reset_tokens(token_hash);

CREATE INDEX idx_password_reset_tokens_user_id
ON password_reset_tokens(user_id);