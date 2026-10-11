// im-maintenance applies explicit object-store lifecycle and expires upload
// sessions/attachments. Run periodically with the same config/upload volume as
// im-server; attachment expiry is an opt-in per-upload setting.
package main

import (
	"context"
	"flag"
	"gochat/internal/logic/service"
	"gochat/internal/pkg/config"
	"gochat/internal/pkg/minio"
	"gochat/internal/pkg/mysql"
	"log"
	"time"
)

func main() {
	filename := flag.String("config", "internal/config/config-dev.yaml", "server configuration")
	lifecycleDays := flag.Int("lifecycle-days", 0, "set staging expiry and incomplete multipart cleanup; 0 leaves bucket rules unchanged")
	flag.Parse()
	if err := run(*filename, *lifecycleDays); err != nil {
		log.Fatal(err)
	}
}
func run(filename string, days int) error {
	cfg, err := config.Load(filename)
	if err != nil {
		return err
	}
	if err = mysql.Init(&cfg.MySQL); err != nil {
		return err
	}
	defer mysql.Close()
	store, err := minio.NewClient(&cfg.MinIO)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if days > 0 {
		if err = store.SetUploadLifecycle(ctx, days); err != nil {
			return err
		}
	}
	return (&service.MediaService{Store: store}).Cleanup(ctx)
}
