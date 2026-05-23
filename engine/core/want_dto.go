package mywant

import want_spec "github.com/onelittlenightmusic/want-spec"

// WantDTOToRuntime converts a *want_spec.Want DTO to a runtime *Want.
// Metadata and WantSpec are type aliases so the fields are directly assignable.
func WantDTOToRuntime(dto *want_spec.Want) *Want {
	return &Want{
		Metadata: dto.Metadata,
		Spec:     dto.Spec,
	}
}

// WantDTOSliceToRuntime converts []*want_spec.Want to []*Want.
func WantDTOSliceToRuntime(dtos []*want_spec.Want) []*Want {
	wants := make([]*Want, len(dtos))
	for i, dto := range dtos {
		wants[i] = WantDTOToRuntime(dto)
	}
	return wants
}

// RuntimeWantToDTO converts a runtime *Want to a *want_spec.Want DTO.
func RuntimeWantToDTO(w *Want) *want_spec.Want {
	return &want_spec.Want{
		Metadata: w.Metadata,
		Spec:     w.Spec,
	}
}

// RuntimeWantsToDTOSlice converts []*Want to []*want_spec.Want.
func RuntimeWantsToDTOSlice(wants []*Want) []*want_spec.Want {
	dtos := make([]*want_spec.Want, len(wants))
	for i, w := range wants {
		dtos[i] = RuntimeWantToDTO(w)
	}
	return dtos
}
