package want_spec

// ToConnectivityMetadata converts RequireSpec to ConnectivityMetadata
func (r *RequireSpec) ToConnectivityMetadata(wantType string) ConnectivityMetadata {
	if r == nil {
		return ConnectivityMetadata{
			RequiredInputs:  0,
			MaxInputs:       -1,
			RequiredOutputs: 0,
			MaxOutputs:      -1,
			WantType:        wantType,
			Description:     "No connectivity requirements",
		}
	}

	requiredInputs := 0
	for _, p := range r.Providers {
		if p.Required {
			requiredInputs++
		}
	}

	requiredOutputs := 0
	for _, u := range r.Users {
		if u.Required {
			requiredOutputs++
		}
	}

	if len(r.Providers) == 0 && len(r.Users) == 0 {
		switch r.Type {
		case "providers":
			requiredInputs = 1
			requiredOutputs = 0
		case "users":
			requiredInputs = 0
			requiredOutputs = 1
		case "providers_and_users":
			requiredInputs = 1
			requiredOutputs = 1
		}
	}

	description := "No connectivity requirements"
	if requiredInputs > 0 && requiredOutputs > 0 {
		description = "Requires both input and output connections"
	} else if requiredInputs > 0 {
		description = "Requires input connections"
	} else if requiredOutputs > 0 {
		description = "Requires output connections"
	}

	return ConnectivityMetadata{
		RequiredInputs:  requiredInputs,
		MaxInputs:       -1,
		RequiredOutputs: requiredOutputs,
		MaxOutputs:      -1,
		WantType:        wantType,
		Description:     description,
	}
}

// ToConnectivityMetadata converts UsageLimitSpec to ConnectivityMetadata
func (u *UsageLimitSpec) ToConnectivityMetadata(wantType string) ConnectivityMetadata {
	if u == nil {
		return ConnectivityMetadata{
			RequiredInputs:  0,
			MaxInputs:       -1,
			RequiredOutputs: 0,
			MaxOutputs:      -1,
			WantType:        wantType,
			Description:     "No connectivity requirements",
		}
	}

	return ConnectivityMetadata{
		RequiredInputs:  u.Providers.Min,
		MaxInputs:       u.Providers.Max,
		RequiredOutputs: u.Users.Min,
		MaxOutputs:      u.Users.Max,
		WantType:        wantType,
		Description:     u.Description,
	}
}
