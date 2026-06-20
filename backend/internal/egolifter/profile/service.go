package profile

import (
	"context"
	"fmt"
	"strings"
)

// maxBodyMetric bounds height (cm) and weight (kg) to sane values.
const maxBodyMetric = 500

// ProfileService defines the contract for profile business logic.
type ProfileService interface {
	GetProfile(ctx context.Context, userID string) (*Profile, error)
	UpdateProfile(ctx context.Context, userID string, req *UpdateProfileRequest) (*Profile, error)
}

type profileService struct {
	profileRepo ProfileRepository
}

// NewProfileService creates a new ProfileService.
func NewProfileService(profileRepo ProfileRepository) ProfileService {
	return &profileService{profileRepo: profileRepo}
}

func (s *profileService) GetProfile(ctx context.Context, userID string) (*Profile, error) {
	return s.profileRepo.Get(ctx, userID)
}

// UpdateProfile validates and saves the user's profile. Names are optional; height
// and weight must be within [0, maxBodyMetric].
func (s *profileService) UpdateProfile(ctx context.Context, userID string, req *UpdateProfileRequest) (*Profile, error) {
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)

	if req.HeightCm < 0 || req.HeightCm > maxBodyMetric {
		return nil, fmt.Errorf("validation: height_cm must be between 0 and %d", maxBodyMetric)
	}
	if req.WeightKg < 0 || req.WeightKg > maxBodyMetric {
		return nil, fmt.Errorf("validation: weight_kg must be between 0 and %d", maxBodyMetric)
	}

	return s.profileRepo.Upsert(ctx, userID, req)
}
