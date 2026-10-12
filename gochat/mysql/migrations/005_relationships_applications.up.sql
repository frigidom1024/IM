-- Apply once after 004. Keep existing friends, memberships and message history.
ALTER TABLE friend
 ADD COLUMN recipient_seen_at DATETIME(3) NULL,
 ADD INDEX idx_friend_recipient (friend_id);

CREATE TABLE IF NOT EXISTS private_pair (
 low_id BIGINT NOT NULL,
 high_id BIGINT NOT NULL,
 low_intro_used BOOLEAN NOT NULL DEFAULT FALSE,
 high_intro_used BOOLEAN NOT NULL DEFAULT FALSE,
 PRIMARY KEY (low_id, high_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- The legacy system did not record whether a send happened before mutual
-- friendship. Conservatively consume the intro in every historical direction.
INSERT INTO private_pair (low_id,high_id,low_intro_used,high_intro_used)
 SELECT LEAST(from_id,to_id),GREATEST(from_id,to_id),
 MAX(from_id<to_id),MAX(from_id>to_id)
 FROM message WHERE group_id=0 AND from_id>0 AND to_id>0 AND from_id<>to_id
 GROUP BY LEAST(from_id,to_id),GREATEST(from_id,to_id)
 ON DUPLICATE KEY UPDATE
 low_intro_used=low_intro_used OR VALUES(low_intro_used),
 high_intro_used=high_intro_used OR VALUES(high_intro_used);

CREATE TABLE IF NOT EXISTS group_application (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 group_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 group_name VARCHAR(128) NOT NULL,
 status VARCHAR(16) NOT NULL,
 reason VARCHAR(512) NOT NULL DEFAULT '',
 handled_by BIGINT NOT NULL DEFAULT 0,
 created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 handled_at DATETIME(3) NULL,
 INDEX idx_application_group_status (group_id,status),
 INDEX idx_application_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
