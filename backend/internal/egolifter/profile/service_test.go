package profile

import (
	"context"
	"testing"
)

// stubProfileRepository records calls and returns configurable fixtures.
type stubProfileRepository struct {
	getProfile   *Profile
	upsertCalled bool
	upsertReq    *UpdateProfileRequest
}

func (s *stubProfileRepository) Get(ctx context.Context, userID string) (*Profile, error) {
	if s.getProfile != nil {
		return s.getProfile, nil
	}
	return &Profile{}, nil
}

func (s *stubProfileRepository) Upsert(ctx context.Context, userID string, req *UpdateProfileRequest) (*Profile, error) {
	s.upsertCalled = true
	s.upsertReq = req
	return &Profile{
		FirstName: req.FirstName,
		LastName:  req.LastName,
		HeightCm:  req.HeightCm,
		WeightKg:  req.WeightKg,
	}, nil
}

func TestUpdateProfile(t *testing.T) {
	tests := []struct {
		name       string
		req        UpdateProfileRequest
		wantErr    bool
		wantUpsert bool
	}{
		{
			name:       "valid profile is saved",
			req:        UpdateProfileRequest{FirstName: "Sam", LastName: "V", HeightCm: 180, WeightKg: 78},
			wantUpsert: true,
		},
		{
			name:       "names trimmed, zero metrics allowed",
			req:        UpdateProfileRequest{FirstName: "  Sam  ", HeightCm: 0, WeightKg: 0},
			wantUpsert: true,
		},
		{
			name:    "negative height rejected",
			req:     UpdateProfileRequest{HeightCm: -5},
			wantErr: true,
		},
		{
			name:    "over-range weight rejected",
			req:     UpdateProfileRequest{WeightKg: 1000},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubProfileRepository{}
			svc := NewProfileService(repo)

			_, err := svc.UpdateProfile(context.Background(), "user-1", &tc.req)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected a validation error, got nil")
				}
				if !isValidationErr(err) {
					t.Errorf("error %q is not a validation error", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if repo.upsertCalled != tc.wantUpsert {
				t.Errorf("upsertCalled = %v, want %v", repo.upsertCalled, tc.wantUpsert)
			}
			if tc.wantUpsert && repo.upsertReq != nil {
				if repo.upsertReq.FirstName != "Sam" && tc.req.FirstName != "" {
					t.Errorf("first name not trimmed/forwarded: %q", repo.upsertReq.FirstName)
				}
			}
		})
	}
}

func TestGetProfile(t *testing.T) {
	repo := &stubProfileRepository{getProfile: &Profile{FirstName: "Sam", HeightCm: 180}}
	svc := NewProfileService(repo)

	p, err := svc.GetProfile(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.FirstName != "Sam" || p.HeightCm != 180 {
		t.Errorf("GetProfile returned %+v, want Sam/180", p)
	}
}
