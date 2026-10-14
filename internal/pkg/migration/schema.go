package migration

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type column struct {
	kind     string
	size     int64
	nullable bool
}
type index struct {
	columns []string
	unique  bool
	partial bool
}
type table struct {
	columns map[string]column
	indexes map[string]*index
	base    bool
}
type schema map[string]*table

func inspectSchema(ctx context.Context, q queryer) (schema, error) {
	s := schema{}
	rows, err := q.QueryContext(ctx, "SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		s[name] = &table{columns: map[string]column{}, indexes: map[string]*index{}, base: kind == "BASE TABLE"}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, "SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, COALESCE(CHARACTER_MAXIMUM_LENGTH,0), IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE()")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var name, field, nullable string
		var c column
		if err := rows.Scan(&name, &field, &c.kind, &c.size, &nullable); err != nil {
			rows.Close()
			return nil, err
		}
		c.nullable = nullable == "YES"
		if t := s[name]; t != nil {
			t.columns[field] = c
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, "SELECT TABLE_NAME, INDEX_NAME, COLUMN_NAME, NON_UNIQUE, COALESCE(SUB_PART,0) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, key string
		var field sql.NullString
		var nonUnique, prefix int
		if err := rows.Scan(&name, &key, &field, &nonUnique, &prefix); err != nil {
			return nil, err
		}
		if t := s[name]; t != nil {
			idx := t.indexes[key]
			if idx == nil {
				idx = &index{unique: nonUnique == 0}
				t.indexes[key] = idx
			}
			idx.columns = append(idx.columns, field.String)
			idx.partial = idx.partial || prefix != 0
		}
	}
	return s, rows.Err()
}

func (s schema) empty() bool {
	for name := range s {
		if name != VersionTable {
			return false
		}
	}
	return true
}

type fieldRequirement struct {
	definition string
	since      int
}
type keyRequirement struct {
	name, fields string
	unique       bool
	since        int
}
type tableRequirement struct {
	since  int
	fields []fieldRequirement
	keys   []keyRequirement
}

// This manifest describes the checked-in SQL, not GORM's AutoMigrate output.
// Check both the requested version and absence of later migration features.
func requirements(version int) map[string]tableRequirement {
	field := func(def string, since int) fieldRequirement { return fieldRequirement{def, since} }
	pk := func(fields string) keyRequirement { return keyRequirement{"PRIMARY", fields, true, 0} }
	key := func(name, fields string, unique bool, since int) keyRequirement {
		return keyRequirement{name, fields, unique, since}
	}
	r := map[string]tableRequirement{
		"conversation":            {since: 6, fields: []fieldRequirement{field("from_id:bigint to_id:bigint group_id:bigint last_message_id:bigint last_message_at:datetime unread_count:bigint read_at:datetime pinned:tinyint muted:tinyint draft:text created_at:datetime updated_at:datetime", 6)}, keys: []keyRequirement{pk("from_id,to_id,group_id"), key("idx_conversation_owner_latest", "from_id,pinned,last_message_at", false, 6), key("idx_conversation_group", "group_id,from_id", false, 6)}},
		"user":                    {fields: []fieldRequirement{field("id:bigint username:varchar32 password:varchar64 nickname:varchar64 avatar:varchar255 status:tinyint created_at:timestamp updated_at:timestamp", 0)}, keys: []keyRequirement{pk("id"), key("username", "username", true, 0), key("idx_status", "status", false, 0), key("idx_username", "username", false, 0)}},
		"friend":                  {fields: []fieldRequirement{field("id:bigint user_id:bigint friend_id:bigint remark:varchar64 created_at:timestamp", 0), field("recipient_seen_at:datetime", 5)}, keys: []keyRequirement{pk("id"), key("uk_user_friend", "user_id,friend_id", true, 0), key("idx_user", "user_id", false, 0), key("idx_friend_recipient", "friend_id", false, 5)}},
		"group":                   {fields: []fieldRequirement{field("id:bigint name:varchar128 avatar:varchar255 owner_id:bigint max_members:int created_at:timestamp updated_at:timestamp", 0), field("announcement:text", 4)}, keys: []keyRequirement{pk("id"), key("idx_owner", "owner_id", false, 0)}},
		"group_member":            {fields: []fieldRequirement{field("id:bigint group_id:bigint user_id:bigint role:tinyint joined_at:timestamp", 0), field("nickname:varchar128 muted_until:datetime last_read_message_id:bigint read_at:datetime", 4)}, keys: []keyRequirement{pk("id"), key("uk_group_user", "group_id,user_id", true, 0), key("idx_group", "group_id", false, 0), key("idx_user", "user_id", false, 0)}},
		"message":                 {fields: []fieldRequirement{field("id:bigint from_id:bigint to_id:bigint group_id:bigint msg_type:varchar20 content:text is_revoked:tinyint created_at:timestamp", 0), field("client_seq:varchar128", 2)}, keys: []keyRequirement{pk("id"), key("idx_from", "from_id", false, 0), key("idx_to", "to_id", false, 0), key("idx_group", "group_id", false, 0), key("idx_created", "created_at", false, 0), key("uk_message_sender_seq", "from_id,client_seq", true, 2)}},
		"conversation_preference": {since: 3, fields: []fieldRequirement{field("user_id:bigint scope:varchar64 pinned:tinyint muted:tinyint draft:text updated_at:datetime", 3)}, keys: []keyRequirement{pk("user_id,scope")}},
		"upload_session":          {since: 3, fields: []fieldRequirement{field("id:varchar64 user_id:bigint group_id:bigint name:varchar255 type:varchar20 mime:varchar128 size:bigint offset:bigint temp_path:varchar1024 asset_id:varchar64 expires_at:datetime asset_expires_at:datetime created_at:datetime", 3)}, keys: []keyRequirement{pk("id"), key("idx_session_expiry", "expires_at", false, 3), key("idx_session_user", "user_id", false, 3)}},
		"media_asset":             {since: 3, fields: []fieldRequirement{field("id:varchar64 user_id:bigint group_id:bigint object_path:varchar512 thumbnail_path:varchar512 name:varchar255 mime:varchar128 size:bigint width:int height:int duration:double expires_at:datetime created_at:datetime", 3)}, keys: []keyRequirement{pk("id"), key("idx_asset_group", "group_id,created_at", false, 3), key("idx_asset_expiry", "expires_at", false, 3), key("uk_asset_path", "object_path", true, 3)}},
		"message_asset":           {since: 3, fields: []fieldRequirement{field("message_id:bigint asset_id:varchar64", 3)}, keys: []keyRequirement{pk("message_id,asset_id"), key("idx_message_asset", "asset_id", false, 3)}},
		"private_pair":            {since: 5, fields: []fieldRequirement{field("low_id:bigint high_id:bigint low_intro_used:tinyint high_intro_used:tinyint", 5)}, keys: []keyRequirement{pk("low_id,high_id")}},
		"group_application":       {since: 5, fields: []fieldRequirement{field("id:bigint group_id:bigint user_id:bigint group_name:varchar128 status:varchar16 reason:varchar512 handled_by:bigint created_at:datetime handled_at:datetime", 5)}, keys: []keyRequirement{pk("id"), key("idx_application_group_status", "group_id,status", false, 5), key("idx_application_user", "user_id", false, 5)}},
	}
	if version == 0 {
		r["unread_count"] = tableRequirement{fields: []fieldRequirement{field("user_id:bigint from_id:bigint group_id:bigint count:bigint read_at:timestamp", 0), field("unread_count:bigint latest_message_id:bigint latest_sender_id:bigint latest_message_type:varchar20 latest_preview:varchar512 latest_message_at:datetime created_at:datetime updated_at:datetime", 1)}, keys: []keyRequirement{pk("user_id,from_id,group_id"), key("idx_user", "user_id", false, 0)}}
	} else {
		r["unread_count"] = tableRequirement{fields: []fieldRequirement{field("user_id:bigint from_id:bigint group_id:bigint unread_count:bigint latest_message_id:bigint latest_sender_id:bigint latest_message_type:varchar20 latest_preview:varchar512 latest_message_at:datetime read_at:datetime created_at:datetime updated_at:datetime", 1)}, keys: []keyRequirement{pk("user_id,from_id,group_id"), key("idx_unread_user_latest", "user_id,latest_message_at", false, 1), key("idx_unread_user_count", "user_id,unread_count", false, 1)}}
	}
	return r
}

func (s schema) validate(version int) error {
	if version < 0 || version > LatestVersion {
		return fmt.Errorf("unsupported version %d", version)
	}
	if _, exists := s["unread_count_v2"]; exists {
		return fmt.Errorf("unfinished unread migration: unread_count_v2 exists")
	}
	r := requirements(version)
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		req, t := r[name], s[name]
		if req.since > version {
			if t != nil {
				return fmt.Errorf("%s belongs to a later migration", name)
			}
			continue
		}
		if t == nil || !t.base {
			return fmt.Errorf("missing base table %s", name)
		}
		for _, group := range req.fields {
			for _, def := range strings.Fields(group.definition) {
				parts := strings.SplitN(def, ":", 2)
				field, expected := parts[0], parts[1]
				c, exists := t.columns[field]
				if group.since > version {
					if exists {
						return fmt.Errorf("%s.%s belongs to a later migration", name, field)
					}
					continue
				}
				if !exists {
					return fmt.Errorf("missing column %s.%s", name, field)
				}
				actual := c.kind
				if actual == "varchar" {
					actual = fmt.Sprintf("varchar%d", c.size)
				}
				if actual != expected {
					return fmt.Errorf("%s.%s type is %s, expected %s", name, field, actual, expected)
				}
			}
		}
		for _, key := range req.keys {
			idx := t.indexes[key.name]
			if key.since > version {
				if idx != nil {
					return fmt.Errorf("%s.%s belongs to a later migration", name, key.name)
				}
				continue
			}
			if idx == nil || strings.Join(idx.columns, ",") != key.fields || idx.unique != key.unique || idx.partial {
				return fmt.Errorf("missing or incompatible index %s.%s", name, key.name)
			}
		}
	}
	if version >= 1 {
		if _, exists := s["unread_count"].columns["count"]; exists {
			return fmt.Errorf("legacy unread_count.count remains")
		}
	}
	if version >= 2 && !s["message"].columns["client_seq"].nullable {
		return fmt.Errorf("message.client_seq must allow NULL for legacy messages")
	}
	if version >= 6 {
		for _, field := range []string{"from_id", "to_id", "group_id", "unread_count", "pinned", "muted", "draft", "created_at", "updated_at"} {
			if s["conversation"].columns[field].nullable {
				return fmt.Errorf("conversation.%s must not allow NULL", field)
			}
		}
		for _, field := range []string{"last_message_id", "last_message_at", "read_at"} {
			if !s["conversation"].columns[field].nullable {
				return fmt.Errorf("conversation.%s must allow NULL for empty scopes", field)
			}
		}
	}
	return nil
}
