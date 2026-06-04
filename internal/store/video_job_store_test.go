package store

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupVideoJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	return db
}

func TestVideoJobStore_CreateGetAndSave(t *testing.T) {
	db := setupVideoJobTestDB(t)
	s := NewVideoJobStore(db)
	ctx := context.Background()
	now := time.Now().UTC()
	job := &VideoGenerationJob{
		JobID:       "vid_job_test",
		Model:       "grok-imagine-1.0-video",
		Status:      "queued",
		PayloadHash: "abc",
		PayloadSize: 123,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	require.NoError(t, s.Create(ctx, job))
	found, err := s.GetByID(ctx, job.JobID)
	require.NoError(t, err)
	require.Equal(t, "queued", found.Status)

	found.Status = "completed"
	found.VideoURL = "/api/files/video/test.mp4"
	require.NoError(t, s.Save(ctx, found))

	updated, err := s.GetByID(ctx, job.JobID)
	require.NoError(t, err)
	require.Equal(t, "completed", updated.Status)
	require.Equal(t, "/api/files/video/test.mp4", updated.VideoURL)
}
