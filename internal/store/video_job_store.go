package store

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// VideoJobStore persists async video generation lifecycle records.
type VideoJobStore struct {
	db *gorm.DB
}

// NewVideoJobStore creates a VideoJobStore.
func NewVideoJobStore(db *gorm.DB) *VideoJobStore {
	return &VideoJobStore{db: db}
}

// Create inserts a new video generation job.
func (s *VideoJobStore) Create(ctx context.Context, job *VideoGenerationJob) error {
	return s.db.WithContext(ctx).Create(job).Error
}

// GetByID returns one video generation job.
func (s *VideoJobStore) GetByID(ctx context.Context, jobID string) (*VideoGenerationJob, error) {
	var job VideoGenerationJob
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// Save persists the current video job state.
func (s *VideoJobStore) Save(ctx context.Context, job *VideoGenerationJob) error {
	if job == nil {
		return errors.New("video job is nil")
	}
	return s.db.WithContext(ctx).Save(job).Error
}
