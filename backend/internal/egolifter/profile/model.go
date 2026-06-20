package profile

// Profile is the current user's editable personal stats.
type Profile struct {
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	HeightCm  float64 `json:"height_cm"`
	WeightKg  float64 `json:"weight_kg"`
}

// UpdateProfileRequest is the incoming payload for PUT /profile.
type UpdateProfileRequest struct {
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	HeightCm  float64 `json:"height_cm"`
	WeightKg  float64 `json:"weight_kg"`
}
