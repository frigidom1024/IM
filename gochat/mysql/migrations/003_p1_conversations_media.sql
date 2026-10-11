-- Apply after P0 migrations. Additive; dropping these tables loses P1 preferences/assets.
CREATE TABLE IF NOT EXISTS conversation_preference (
 user_id BIGINT NOT NULL, scope VARCHAR(64) NOT NULL,
 pinned BOOLEAN NOT NULL DEFAULT FALSE, muted BOOLEAN NOT NULL DEFAULT FALSE,
 draft TEXT NOT NULL, updated_at DATETIME(3) NOT NULL,
 PRIMARY KEY(user_id,scope)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS upload_session (
 id VARCHAR(64) PRIMARY KEY, user_id BIGINT NOT NULL, group_id BIGINT NOT NULL DEFAULT 0,
 name VARCHAR(255) NOT NULL, type VARCHAR(20) NOT NULL, mime VARCHAR(128) NOT NULL,
 size BIGINT NOT NULL, offset BIGINT NOT NULL DEFAULT 0, temp_path VARCHAR(1024) NOT NULL,
 asset_id VARCHAR(64) NOT NULL DEFAULT '', expires_at DATETIME(3) NOT NULL,
 asset_expires_at DATETIME(3) NULL, created_at DATETIME(3) NOT NULL,
 INDEX idx_session_expiry(expires_at), INDEX idx_session_user(user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS media_asset (
 id VARCHAR(64) PRIMARY KEY, user_id BIGINT NOT NULL, group_id BIGINT NOT NULL DEFAULT 0,
 object_path VARCHAR(512) NOT NULL, thumbnail_path VARCHAR(512) NOT NULL DEFAULT '',
 name VARCHAR(255) NOT NULL, mime VARCHAR(128) NOT NULL, size BIGINT NOT NULL,
 width INT NOT NULL DEFAULT 0, height INT NOT NULL DEFAULT 0, duration DOUBLE NOT NULL DEFAULT 0,
 expires_at DATETIME(3) NULL, created_at DATETIME(3) NOT NULL,
 INDEX idx_asset_group(group_id,created_at), INDEX idx_asset_expiry(expires_at),
 UNIQUE KEY uk_asset_path(object_path)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS message_asset (
 message_id BIGINT NOT NULL, asset_id VARCHAR(64) NOT NULL,
 PRIMARY KEY(message_id,asset_id), INDEX idx_message_asset(asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
