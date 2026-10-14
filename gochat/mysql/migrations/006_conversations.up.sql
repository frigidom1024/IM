-- Stop all application writers before this migration. Legacy state stays as backup.
CREATE TABLE conversation (
  from_id BIGINT NOT NULL,
  to_id BIGINT NOT NULL DEFAULT 0,
  group_id BIGINT NOT NULL DEFAULT 0,
  last_message_id BIGINT NULL,
  last_message_at DATETIME(3) NULL,
  unread_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  read_at DATETIME(3) NULL,
  pinned BOOLEAN NOT NULL DEFAULT FALSE,
  muted BOOLEAN NOT NULL DEFAULT FALSE,
  draft TEXT NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (from_id,to_id,group_id),
  KEY idx_conversation_owner_latest (from_id,pinned,last_message_at),
  KEY idx_conversation_group (group_id,from_id),
  CONSTRAINT chk_conversation_scope CHECK (
    (from_id>0 AND group_id=0 AND to_id>0 AND from_id<>to_id) OR
    (from_id>0 AND group_id>0 AND to_id=0)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO conversation (from_id,to_id,group_id,draft)
SELECT owner,peer,gid,'' FROM (
  SELECT from_id AS owner,to_id AS peer,0 AS gid FROM message WHERE group_id=0 AND from_id>0 AND to_id>0 AND from_id<>to_id
  UNION SELECT to_id,from_id,0 FROM message WHERE group_id=0 AND from_id>0 AND to_id>0 AND from_id<>to_id
  UNION SELECT gm.user_id,0,gm.group_id FROM group_member gm JOIN `group` g ON g.id=gm.group_id JOIN message m ON m.group_id=gm.group_id
  UNION SELECT user_id,from_id,0 FROM unread_count WHERE group_id=0 AND user_id>0 AND from_id>0 AND user_id<>from_id
  UNION SELECT u.user_id,0,u.group_id FROM unread_count u JOIN group_member gm ON gm.group_id=u.group_id AND gm.user_id=u.user_id JOIN `group` g ON g.id=u.group_id WHERE u.group_id>0
  UNION SELECT user_id,CAST(SUBSTRING(scope,9) AS SIGNED),0 FROM conversation_preference
    WHERE scope REGEXP '^private:[1-9][0-9]{0,18}$' AND user_id>0 AND CAST(SUBSTRING(scope,9) AS DECIMAL(20))<=9223372036854775807 AND user_id<>CAST(SUBSTRING(scope,9) AS SIGNED)
  UNION SELECT user_id,0,CAST(SUBSTRING(scope,7) AS SIGNED) FROM conversation_preference WHERE scope REGEXP '^group:[1-9][0-9]{0,18}$' AND user_id>0 AND CAST(SUBSTRING(scope,7) AS DECIMAL(20))<=9223372036854775807
) scopes;

-- One private message is projected into both owners' views.
UPDATE conversation c JOIN (
  SELECT id,created_at,LEAST(from_id,to_id) AS low_id,GREATEST(from_id,to_id) AS high_id,
    ROW_NUMBER() OVER (PARTITION BY LEAST(from_id,to_id),GREATEST(from_id,to_id) ORDER BY created_at DESC,id DESC) AS position
  FROM message WHERE group_id=0 AND from_id>0 AND to_id>0 AND from_id<>to_id
) m ON c.group_id=0 AND LEAST(c.from_id,c.to_id)=m.low_id AND GREATEST(c.from_id,c.to_id)=m.high_id AND m.position=1
SET c.last_message_id=m.id,c.last_message_at=m.created_at;

UPDATE conversation c JOIN (
  SELECT id,group_id,created_at,ROW_NUMBER() OVER (PARTITION BY group_id ORDER BY created_at DESC,id DESC) AS position
  FROM message WHERE group_id>0
) m ON c.group_id=m.group_id AND m.position=1
SET c.last_message_id=m.id,c.last_message_at=m.created_at;

UPDATE conversation c JOIN unread_count u ON c.from_id=u.user_id AND c.group_id=u.group_id AND c.to_id=IF(u.group_id=0,u.from_id,0)
SET c.unread_count=u.unread_count,c.read_at=u.read_at,c.created_at=u.created_at,c.updated_at=u.updated_at;

UPDATE conversation c JOIN group_member gm ON c.group_id=gm.group_id AND c.from_id=gm.user_id
SET c.read_at=IF(c.read_at IS NULL OR gm.read_at>c.read_at,gm.read_at,c.read_at)
WHERE c.group_id>0;

UPDATE conversation c JOIN conversation_preference p ON c.from_id=p.user_id AND p.scope=IF(c.group_id=0,CONCAT('private:',c.to_id),CONCAT('group:',c.group_id))
SET c.pinned=p.pinned,c.muted=p.muted,c.draft=p.draft,c.updated_at=GREATEST(c.updated_at,p.updated_at);
