-- P1 group administration. Apply once after init.sql and migration 003.
ALTER TABLE `group` ADD COLUMN `announcement` TEXT NULL;
ALTER TABLE `group_member`
 ADD COLUMN `nickname` VARCHAR(128) NOT NULL DEFAULT '',
 ADD COLUMN `muted_until` DATETIME(3) NULL,
 ADD COLUMN `last_read_message_id` BIGINT NOT NULL DEFAULT 0,
 ADD COLUMN `read_at` DATETIME(3) NULL;

-- Older HTTP handlers used 1/2/3 while the model used 2/1/0.
-- owner_id is authoritative; never grant an extra owner during normalization.
UPDATE `group_member` gm JOIN `group` g ON g.id = gm.group_id
 SET gm.role = CASE WHEN gm.user_id = g.owner_id THEN 2
                   WHEN gm.role = 1 THEN 1 ELSE 0 END;
INSERT INTO `group_member` (`group_id`, `user_id`, `role`)
 SELECT g.id, g.owner_id, 2 FROM `group` g
 LEFT JOIN `group_member` gm ON gm.group_id = g.id AND gm.user_id = g.owner_id
 WHERE gm.id IS NULL;
