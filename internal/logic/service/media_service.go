package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gochat/internal/logic/model"
	"gochat/internal/pkg/minio"
	"gochat/internal/pkg/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrMediaForbidden = errors.New("无权访问文件")
var ErrUploadConflict = errors.New("上传偏移不一致或上传未完成")
var ErrMediaExpired = errors.New("上传或文件已过期")

type MediaService struct{ Store *minio.Client }

func mediaID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func (s *MediaService) CreateSession(ctx context.Context, session *model.UploadSession) error {
	if session.GroupID > 0 {
		var count int64
		if err := mysql.Client.WithContext(ctx).Model(&model.GroupMember{}).Where("group_id=? AND user_id=?", session.GroupID, session.UserID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrMediaForbidden
		}
	}
	id, err := mediaID()
	if err != nil {
		return err
	}
	session.ID = id
	directory := os.Getenv("GOCHAT_UPLOAD_DIR")
	if directory == "" {
		directory = filepath.Join(os.TempDir(), "gochat-uploads")
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, "upload-*")
	if err != nil {
		return err
	}
	session.TempPath = file.Name()
	file.Close()
	session.ExpiresAt = time.Now().Add(24 * time.Hour)
	err = mysql.Client.WithContext(ctx).Create(session).Error
	if err != nil {
		os.Remove(session.TempPath)
	}
	return err
}

func (s *MediaService) Session(ctx context.Context, id string, userID int64) (*model.UploadSession, error) {
	var session model.UploadSession
	err := mysql.Client.WithContext(ctx).Where("id=? AND user_id=?", id, userID).First(&session).Error
	if err != nil {
		return nil, err
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrMediaExpired
	}
	return &session, nil
}

// Append serializes competing chunks through a row lock. An interrupted write
// is truncated back to the committed offset on the next request.
func (s *MediaService) Append(ctx context.Context, id string, userID, offset int64, reader io.Reader) error {
	return mysql.Client.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", id, userID).First(&session).Error; err != nil {
			return err
		}
		if time.Now().After(session.ExpiresAt) {
			return ErrMediaExpired
		}
		if session.Offset != offset || session.AssetID != "" {
			return ErrUploadConflict
		}
		file, err := os.OpenFile(session.TempPath, os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		if err = file.Truncate(offset); err != nil {
			return err
		}
		if _, err = file.Seek(offset, 0); err != nil {
			return err
		}
		remaining := session.Size - offset
		if remaining > 8<<20 {
			remaining = 8 << 20
		}
		written, err := io.Copy(file, io.LimitReader(reader, remaining+1))
		if err != nil || written > remaining || written == 0 {
			file.Truncate(offset)
			if err != nil {
				return err
			}
			return ErrUploadConflict
		}
		if err = file.Sync(); err != nil {
			file.Truncate(offset)
			return err
		}
		return tx.Model(&session).Update("offset", offset+written).Error
	})
}

func (s *MediaService) Complete(ctx context.Context, id string, userID int64) (*model.MediaAsset, error) {
	var asset model.MediaAsset
	uploaded := false
	err := mysql.Client.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", id, userID).First(&session).Error; err != nil {
			return err
		}
		if time.Now().After(session.ExpiresAt) {
			return ErrMediaExpired
		}
		if session.AssetID != "" {
			return tx.Where("id=?", session.AssetID).First(&asset).Error
		}
		if session.Offset != session.Size {
			return ErrUploadConflict
		}
		if session.GroupID > 0 {
			var count int64
			if err := tx.Model(&model.GroupMember{}).Where("group_id=? AND user_id=?", session.GroupID, userID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrMediaForbidden
			}
		}
		asset = model.MediaAsset{ID: session.ID, UserID: userID, GroupID: session.GroupID, Name: session.Name, MIME: session.MIME, ExpiresAt: session.AssetExpiresAt}
		output, thumbnail, err := processMedia(ctx, session.TempPath, session.Type, &asset)
		defer os.Remove(session.TempPath + ".mp4")
		defer os.Remove(session.TempPath + ".thumb.jpg")
		if err != nil {
			return err
		}
		file, err := os.Open(output)
		if err != nil {
			return err
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil {
			return err
		}
		asset.Size = stat.Size()
		asset.ObjectPath = "attachments/" + asset.ID
		uploaded = true
		if _, err = s.Store.Upload(ctx, asset.ObjectPath, file, asset.Size, asset.MIME); err != nil {
			return err
		}
		if thumbnail != "" {
			thumb, err := os.Open(thumbnail)
			if err != nil {
				return err
			}
			defer thumb.Close()
			stat, err := thumb.Stat()
			if err != nil {
				return err
			}
			asset.ThumbnailPath = "thumbnails/" + asset.ID + ".jpg"
			if _, err = s.Store.Upload(ctx, asset.ThumbnailPath, thumb, stat.Size(), "image/jpeg"); err != nil {
				return err
			}
		}
		if err = tx.Create(&asset).Error; err != nil {
			return err
		}
		if err = tx.Model(&session).Update("asset_id", asset.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil && uploaded {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Preserve a confirmed commit if its acknowledgement was lost.
		_ = mysql.Client.WithContext(cleanup).Transaction(func(tx *gorm.DB) error {
			var session model.UploadSession
			if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "id=?", id).Error; lockErr != nil {
				return lockErr
			}
			var count int64
			if checkErr := tx.Model(&model.MediaAsset{}).Where("id=?", asset.ID).Count(&count).Error; checkErr != nil || count != 0 {
				return checkErr
			}
			if deleteErr := s.Store.Delete(cleanup, asset.ObjectPath); deleteErr != nil {
				return deleteErr
			}
			if asset.ThumbnailPath != "" {
				return s.Store.Delete(cleanup, asset.ThumbnailPath)
			}
			return nil
		})
	}
	if err == nil {
		var session model.UploadSession
		if mysql.Client.WithContext(ctx).Where("id=? AND user_id=?", id, userID).First(&session).Error == nil {
			os.Remove(session.TempPath)
		}
	}
	return &asset, err
}

func (s *MediaService) AuthorizedAsset(ctx context.Context, id string, userID int64) (*model.MediaAsset, error) {
	var asset model.MediaAsset
	if err := mysql.Client.WithContext(ctx).Where("id=?", id).First(&asset).Error; err != nil {
		return nil, err
	}
	if asset.ExpiresAt != nil && !time.Now().Before(*asset.ExpiresAt) {
		return nil, ErrMediaExpired
	}
	if asset.UserID == userID {
		return &asset, nil
	}
	if asset.GroupID > 0 {
		var count int64
		err := mysql.Client.WithContext(ctx).Model(&model.GroupMember{}).Where("group_id=? AND user_id=?", asset.GroupID, userID).Count(&count).Error
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return &asset, nil
		}
	}
	// Only validated shares committed by SendWithUnread grant recipient access.
	var count int64
	query := `SELECT COUNT(*) FROM message m JOIN message_asset ma ON ma.message_id=m.id WHERE m.is_revoked=0 AND ma.asset_id=? AND
 ((m.group_id=0 AND (m.from_id=? OR m.to_id=?)) OR m.group_id IN (SELECT group_id FROM group_member WHERE user_id=?))`
	err := mysql.Client.WithContext(ctx).Raw(query, asset.ID, userID, userID, userID).Scan(&count).Error
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrMediaForbidden
	}
	return &asset, nil
}

func MediaMetadata(asset *model.MediaAsset) *minio.FileMeta {
	result := &minio.FileMeta{URL: "/api/file/" + asset.ID + "/download", Name: asset.Name, Size: asset.Size, MIME: asset.MIME, Width: asset.Width, Height: asset.Height, Duration: asset.Duration}
	if asset.ThumbnailPath != "" {
		result.Thumbnail = "/api/file/" + asset.ID + "/thumbnail"
	}
	return result
}

func (s *MediaService) Cleanup(ctx context.Context) error {
	// Each batch is bounded; cron can repeat for larger backlogs. Row locks
	// serialize maintenance with ongoing uploads/completion.
	var sessions []model.UploadSession
	if err := mysql.Client.WithContext(ctx).Where("expires_at < ?", time.Now()).Limit(100).Find(&sessions).Error; err != nil {
		return err
	}
	for _, session := range sessions {
		err := mysql.Client.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var current model.UploadSession
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", session.ID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			if err := os.Remove(current.TempPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			return tx.Delete(&current).Error
		})
		if err != nil {
			return err
		}
	}
	var assets []model.MediaAsset
	if err := mysql.Client.WithContext(ctx).Where("expires_at < ?", time.Now()).Limit(100).Find(&assets).Error; err != nil {
		return err
	}
	for _, asset := range assets {
		if err := s.Store.Delete(ctx, asset.ObjectPath); err != nil {
			return err
		}
		if asset.ThumbnailPath != "" {
			if err := s.Store.Delete(ctx, asset.ThumbnailPath); err != nil {
				return err
			}
		}
		if err := mysql.Client.WithContext(ctx).Delete(&asset).Error; err != nil {
			return err
		}
	}
	// Recover orphaned objects after an ambiguous database outage or a crash
	// between object upload and COMMIT. Recent objects cannot be removed here.
	deleted := 0
	return s.Store.VisitStaleManagedObjects(ctx, time.Now().Add(-24*time.Hour), func(objectPath string) (bool, error) {
		id := strings.TrimSuffix(filepath.Base(objectPath), ".jpg")
		if len(id) != 32 {
			return true, nil
		}
		if _, err := hex.DecodeString(id); err != nil {
			return true, nil
		}
		var count int64
		if err := mysql.Client.WithContext(ctx).Model(&model.MediaAsset{}).Where("id=?", id).Count(&count).Error; err != nil {
			return false, err
		}
		if count == 0 {
			if err := s.Store.Delete(ctx, objectPath); err != nil {
				return false, err
			}
			deleted++
		}
		return deleted < 100, nil
	})
}

func MediaContent(asset *model.MediaAsset) string {
	data, _ := json.Marshal(MediaMetadata(asset))
	return string(data)
}

// Abort removes only an owned unfinished session, under the same chunk lock.
func (s *MediaService) Abort(ctx context.Context, id string, userID int64) error {
	return mysql.Client.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", id, userID).First(&session).Error; err != nil {
			return err
		}
		if session.AssetID != "" {
			return fmt.Errorf("已完成的上传不能取消")
		}
		if err := os.Remove(session.TempPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return tx.Delete(&session).Error
	})
}
