package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	mywant "mywant/engine/core"
)

// Devices: the browsers and apps connected to this install, and which of them
// does what. Two kinds of fact, kept apart (core/device_store.go):
//
//   - who is connected is presence — each browser announces itself in
//     gui_state's `devices` every thirty seconds and drops out minutes after
//     it stops — so it is read from there, and a device is never "created"
//     here: it appears by connecting;
//   - which browser browser work is pinned to, which sends its location and to
//     which want, is configuration about this machine (devices.yaml).
//
//	GET    /api/v1/devices            {devices: [...], settings}
//	GET    /api/v1/devices/settings   {homeBrowserDevice, activeLocationDevice, locationWantId}
//	PUT    /api/v1/devices/settings   any of them; "" unsets
//	GET    /api/v1/devices/{id}       one, with the characters it is assigned to
//	DELETE /api/v1/devices/{id}       forget it: off the list, off every
//	                                  character, and unpinned where it was pinned

// deviceView is one connected device, as the API gives it.
type deviceView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	LastSeen int64  `json:"lastSeen"`
	// Home: browser work runs in it. SendsLocation: it is the one whose
	// location is written to the location want.
	Home          bool `json:"home,omitempty"`
	SendsLocation bool `json:"sendsLocation,omitempty"`
	// Characters it is assigned to.
	Characters []string `json:"characters,omitempty"`
}

// deviceRoster is gui_state's `devices`, as the browsers wrote it.
func (s *Server) deviceRoster() []deviceView {
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		return nil
	}
	raw, ok := want.GetAllState()[mywant.DeviceKeyRoster]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var list []deviceView
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return list
}

// deviceViews is the roster with what each device does and who it is assigned to.
func (s *Server) deviceViews() []deviceView {
	settings := mywant.GetDeviceStore().Settings()
	assigned := map[string][]string{}
	for _, c := range mywant.GetCharacterManager().List() {
		for _, d := range c.AssignedDeviceIDs {
			assigned[d] = append(assigned[d], c.ID)
		}
	}
	list := s.deviceRoster()
	for i := range list {
		list[i].Home = list[i].ID == settings.HomeBrowserDevice
		list[i].SendsLocation = list[i].ID == settings.ActiveLocationDevice
		list[i].Characters = assigned[list[i].ID]
	}
	if list == nil {
		list = []deviceView{}
	}
	return list
}

// GET /api/v1/devices
func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"devices":  s.deviceViews(),
		"settings": mywant.GetDeviceStore().Settings(),
	})
}

// GET /api/v1/devices/{id}
func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	for _, d := range s.deviceViews() {
		if d.ID == id {
			s.JSONResponse(w, http.StatusOK, d)
			return
		}
	}
	s.JSONError(w, r, http.StatusNotFound, "no such device", id)
}

// DELETE /api/v1/devices/{id}
func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	want := s.findWantByIDInAll(guiStateWantID)
	found := false
	if want != nil {
		var kept []deviceView
		for _, d := range s.deviceRoster() {
			if d.ID == id {
				found = true
				continue
			}
			kept = append(kept, deviceView{ID: d.ID, Name: d.Name, LastSeen: d.LastSeen})
		}
		if found {
			if kept == nil {
				kept = []deviceView{}
			}
			want.StoreState(mywant.DeviceKeyRoster, kept)
		}
	}
	// Off every character, and unpinned where it was pinned — whether or not
	// it was still on the list (a device long gone can still be named there).
	mywant.GetCharacterManager().PruneDevices([]string{id})
	settings := mywant.GetDeviceStore().Settings()
	unpin := map[string]any{}
	if settings.HomeBrowserDevice == id {
		unpin[mywant.DeviceKeyHome] = ""
	}
	if settings.ActiveLocationDevice == id {
		unpin[mywant.DeviceKeyActiveLocation] = ""
	}
	if len(unpin) > 0 {
		mywant.GetDeviceStore().ApplyGUIUpdates(unpin)
		found = true
	}
	if !found {
		s.JSONError(w, r, http.StatusNotFound, "no such device", id)
		return
	}
	s.broadcastGUIState(want)
	go broadcastSSE("character_changed", "")
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "forgotten", "id": id})
}

// GET /api/v1/devices/settings
func (s *Server) getDeviceSettings(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, mywant.GetDeviceStore().Settings())
}

// PUT /api/v1/devices/settings  {homeBrowserDevice?, activeLocationDevice?, locationWantId?}
func (s *Server) putDeviceSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Home     *string `json:"homeBrowserDevice"`
		Location *string `json:"activeLocationDevice"`
		Want     *string `json:"locationWantId"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	updates := map[string]any{}
	if body.Home != nil {
		updates[mywant.DeviceKeyHome] = *body.Home
	}
	if body.Location != nil {
		updates[mywant.DeviceKeyActiveLocation] = *body.Location
	}
	if body.Want != nil {
		updates[mywant.DeviceKeyLocationWant] = *body.Want
	}
	mywant.GetDeviceStore().ApplyGUIUpdates(updates)
	// Mirrored into gui_state, where clients read them (guiStateWithConfig).
	want := s.findWantByIDInAll(guiStateWantID)
	if want != nil {
		for k, v := range updates {
			want.StoreState(k, v)
		}
	}
	s.broadcastGUIState(want)
	s.JSONResponse(w, http.StatusOK, mywant.GetDeviceStore().Settings())
}

// broadcastGUIState tells every client gui_state changed, as a PUT to it does.
func (s *Server) broadcastGUIState(want *mywant.Want) {
	if want == nil {
		return
	}
	go broadcastSSE("gui_state", guiStateResponse{Seq: nextGUIStateSeq(), State: s.guiStateWithConfig(want)})
}
