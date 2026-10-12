package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-sql-driver/mysql"
)

var testSequence atomic.Uint64

// Every case gets a new database. Never execute migrations against the DB
// named by the environment variable, even if it is a deployment database.
func testRunner(t *testing.T) *Runner {
	t.Helper()
	dsn := os.Getenv("GOCHAT_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set GOCHAT_MIGRATION_TEST_DSN to a dedicated MySQL 8.0 test instance")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid migration test DSN")
	}
	cfg.DBName, cfg.MultiStatements, cfg.ParseTime = "", true, true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	name := fmt.Sprintf("gochat_migrate_test_%d_%d", time.Now().UnixNano(), testSequence.Add(1))
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !strings.HasPrefix(name, "gochat_migrate_test_") {
			t.Fatal("refusing to drop a database outside the test namespace")
		}
		if _, err := admin.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error(err)
		}
	})
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db)
}

func execSQL(t *testing.T, r *Runner, query string) {
	t.Helper()
	if _, err := r.db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func applyFile(t *testing.T, r *Runner, path string) {
	t.Helper()
	body, err := fs.ReadFile(r.scripts, path)
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, r, string(body))
}

func latestStatus(t *testing.T, r *Runner) Status {
	t.Helper()
	s, err := r.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func scalar(t *testing.T, r *Runner, query string) int {
	t.Helper()
	var value int
	if err := r.db.QueryRow(query).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestStatusDoesNotCreateTables(t *testing.T) {
	r := testRunner(t)
	for i := 0; i < 2; i++ {
		s := latestStatus(t, r)
		if !s.Empty || s.Version != -1 || s.Dirty {
			t.Fatalf("empty status = %#v", s)
		}
	}
	if n := scalar(t, r, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE()"); n != 0 {
		t.Fatalf("status created %d tables", n)
	}
}

func TestEmptyDatabaseUpgradeAndRepeat(t *testing.T) {
	r := testRunner(t)
	if err := r.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := latestStatus(t, r)
	if s.Version != 5 || s.Dirty || s.SchemaError != "" || len(s.Pending) != 0 {
		t.Fatalf("status = %#v", s)
	}
	execSQL(t, r, "INSERT INTO private_pair VALUES (1,2,FALSE,FALSE)")
	if err := r.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, r, "SELECT COUNT(*) FROM private_pair WHERE low_intro_used=FALSE AND high_intro_used=FALSE"); n != 1 {
		t.Fatal("repeat upgrade changed business data")
	}
	if err := r.Baseline(context.Background(), 5); err == nil {
		t.Fatal("baseline overwrote a registered version")
	}
	if err := r.Repair(context.Background(), 5); err == nil {
		t.Fatal("repair overwrote a healthy version")
	}
}

func TestLegacyUpgradePreservesAndTransformsData(t *testing.T) {
	r := testRunner(t)
	legacy, err := os.ReadFile("testdata/legacy.sql")
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, r, string(legacy))
	execSQL(t, r, `INSERT INTO user (id,username,password) VALUES (1,'one','hash'),(2,'two','hash'),(3,'three','hash'),(4,'four','hash');
INSERT INTO friend (user_id,friend_id,remark) VALUES (1,2,'keep me');
INSERT INTO `+"`group`"+` (id,name,owner_id) VALUES (10,'first',1),(20,'second',4);
INSERT INTO group_member (group_id,user_id,role) VALUES (10,1,1),(10,2,3),(10,3,1);
INSERT INTO message (id,from_id,to_id,group_id,msg_type,content) VALUES
 (100,2,1,0,'text','private'),(101,1,2,0,'text','reply'),(102,2,0,10,'text','group one'),(103,3,0,10,'text','group two');
INSERT INTO unread_count (user_id,from_id,group_id,count) VALUES (1,2,10,2),(1,3,10,4),(1,2,0,3);`)
	s := latestStatus(t, r)
	if s.Candidate != 0 || s.Version != -1 {
		t.Fatalf("legacy status = %#v", s)
	}
	if err := r.Up(context.Background()); err == nil {
		t.Fatal("unregistered legacy database was upgraded")
	}
	if n := scalar(t, r, "SELECT COUNT(*) FROM unread_count WHERE count>0"); n != 3 {
		t.Fatal("refused upgrade changed unread rows")
	}
	if err := r.Baseline(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := r.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	checks := map[string]int{
		"SELECT unread_count FROM unread_count WHERE user_id=1 AND from_id=0 AND group_id=10":                             6,
		"SELECT unread_count FROM unread_count WHERE user_id=1 AND from_id=2 AND group_id=0":                              3,
		"SELECT COUNT(*) FROM unread_count_legacy":                                                                        3,
		"SELECT COUNT(*) FROM message":                                                                                    4,
		"SELECT COUNT(*) FROM friend WHERE remark='keep me'":                                                              1,
		"SELECT role FROM group_member WHERE group_id=10 AND user_id=1":                                                   2,
		"SELECT role FROM group_member WHERE group_id=10 AND user_id=2":                                                   0,
		"SELECT role FROM group_member WHERE group_id=10 AND user_id=3":                                                   1,
		"SELECT role FROM group_member WHERE group_id=20 AND user_id=4":                                                   2,
		"SELECT COUNT(*) FROM private_pair WHERE low_id=1 AND high_id=2 AND low_intro_used=TRUE AND high_intro_used=TRUE": 1,
	}
	for query, want := range checks {
		if got := scalar(t, r, query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
	if s := latestStatus(t, r); s.Version != 5 || s.Dirty || s.SchemaError != "" {
		t.Fatalf("upgraded status = %#v", s)
	}
}

func TestManualDatabaseBaselineAtEachVersion(t *testing.T) {
	for version := 0; version <= 5; version++ {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			r := testRunner(t)
			legacy, err := os.ReadFile("testdata/legacy.sql")
			if err != nil {
				t.Fatal(err)
			}
			execSQL(t, r, string(legacy))
			files, err := fs.ReadDir(r.scripts, "migrations")
			if err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= version; i++ {
				for _, file := range files {
					if strings.HasPrefix(file.Name(), fmt.Sprintf("%03d_", i)) {
						applyFile(t, r, "migrations/"+file.Name())
					}
				}
			}
			if s := latestStatus(t, r); s.Candidate != version {
				t.Fatalf("candidate = %#v", s)
			}
			if n := scalar(t, r, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='"+VersionTable+"'"); n != 0 {
				t.Fatal("status created metadata")
			}
			if err := r.Baseline(context.Background(), version); err != nil {
				t.Fatal(err)
			}
			if err := r.Up(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBaselineRejectsPartialSchemaAndWrongVersion(t *testing.T) {
	r := testRunner(t)
	applyFile(t, r, "init.sql")
	if err := r.Baseline(context.Background(), 1); err == nil {
		t.Fatal("baseline 1 accepted version 2 features")
	}
	execSQL(t, r, "ALTER TABLE `group` ADD COLUMN announcement TEXT NULL")
	if s := latestStatus(t, r); s.Candidate != -1 {
		t.Fatalf("partial migration candidate = %#v", s)
	}
	if err := r.Baseline(context.Background(), 2); err == nil {
		t.Fatal("partial migration was baselined")
	}
	if err := r.Baseline(context.Background(), 4); err == nil {
		t.Fatal("incomplete migration was baselined")
	}
	execSQL(t, r, "ALTER TABLE `group` DROP COLUMN announcement; ALTER TABLE message DROP INDEX uk_message_sender_seq")
	if err := r.Baseline(context.Background(), 2); err == nil {
		t.Fatal("missing idempotency index was accepted")
	}
}

func copiedScripts(t *testing.T, r *Runner) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	if err := fs.WalkDir(r.scripts, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(r.scripts, path)
		if err != nil {
			return err
		}
		files[path] = &fstest.MapFile{Data: body}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestDirtyMigrationRequiresCompletedManualRepair(t *testing.T) {
	r := testRunner(t)
	applyFile(t, r, "init.sql")
	applyFile(t, r, "migrations/003_p1_conversations_media.up.sql")
	if err := r.Baseline(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	files := copiedScripts(t, r)
	files["migrations/004_p1_groups.up.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE `group` ADD COLUMN announcement TEXT NULL; SELECT * FROM deliberately_missing_table;")}
	r.scripts = files
	if err := r.Up(context.Background()); err == nil {
		t.Fatal("injected SQL error did not fail")
	}
	s := latestStatus(t, r)
	if s.Version != 4 || !s.Dirty {
		t.Fatalf("failed migration status = %#v", s)
	}
	if err := r.Up(context.Background()); err == nil {
		t.Fatal("dirty migration continued")
	}
	if err := r.Baseline(context.Background(), 4); err == nil {
		t.Fatal("baseline cleared dirty state")
	}
	if err := r.Repair(context.Background(), 3); err == nil {
		t.Fatal("repair changed the dirty version")
	}
	if err := r.Repair(context.Background(), 4); err == nil {
		t.Fatal("repair accepted incomplete SQL")
	}
	// The first ALTER committed. Complete only the remaining statements.
	body, err := fs.ReadFile(New(r.db).scripts, "migrations/004_p1_groups.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(body), "ALTER TABLE `group_member`")
	if start < 0 {
		t.Fatal("missing remainder of group migration")
	}
	execSQL(t, r, string(body[start:]))
	if err := r.Repair(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	r.scripts = New(r.db).scripts
	if err := r.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFailedInitializationStaysDirty(t *testing.T) {
	r := testRunner(t)
	files := copiedScripts(t, r)
	files["init.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE user (id BIGINT PRIMARY KEY); SELECT * FROM deliberately_missing_table;")}
	r.scripts = files
	if err := r.Up(context.Background()); err == nil {
		t.Fatal("initialization unexpectedly succeeded")
	}
	if s := latestStatus(t, r); s.Version != 2 || !s.Dirty {
		t.Fatalf("failed initialization status = %#v", s)
	}
	if err := r.Repair(context.Background(), 2); err == nil {
		t.Fatal("incomplete initialization repaired")
	}
	// Complete bootstrap manually, without rerunning CREATE TABLE on the
	// incompatible partial user table. The test table has no business rows.
	execSQL(t, r, "DROP TABLE user")
	r.scripts = New(r.db).scripts
	applyFile(t, r, "init.sql")
	if err := r.Repair(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if err := r.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCancellationStopsSQLAndPreservesDirtyVersion(t *testing.T) {
	r := testRunner(t)
	applyFile(t, r, "init.sql")
	applyFile(t, r, "migrations/003_p1_conversations_media.up.sql")
	if err := r.Baseline(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	files := copiedScripts(t, r)
	files["migrations/004_p1_groups.up.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE `group` ADD COLUMN announcement TEXT NULL; SELECT SLEEP(30);")}
	r.scripts = files
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Up(ctx) }()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		select {
		case err := <-done:
			t.Fatalf("SQL finished before cancellation: %v", err)
		case <-deadline:
			t.Fatal("migration did not begin")
		case <-ticker.C:
			if scalar(t, r, "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='group' AND COLUMN_NAME='announcement'") == 1 {
				break waiting
			}
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled command is still executing SQL")
	}
	if s := latestStatus(t, r); s.Version != 4 || !s.Dirty {
		t.Fatalf("cancelled status = %#v", s)
	}
}

func TestConcurrentUpSerializesInitializationAndUpgrade(t *testing.T) {
	r := testRunner(t)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := New(r.db).Up(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if s := latestStatus(t, r); s.Version != 5 || s.Dirty {
		t.Fatalf("concurrent status = %#v", s)
	}
}
