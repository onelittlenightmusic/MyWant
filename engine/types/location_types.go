package types

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[LocationWant, LocationLocals]("location")
	})
}

type LocationLocals struct{}

// LocationWant receives browser geolocation data via webhook and exposes
// lat/lng/accuracy/altitude/device_id/device_name/updated_at as state fields.
// After a significant position change it reverse-geocodes via Nominatim to
// populate city/prefecture/address.
type LocationWant struct{ Want }

func (lw *LocationWant) GetLocals() *LocationLocals {
	return CheckLocalsInitialized[LocationLocals](&lw.Want)
}

func (lw *LocationWant) Initialize() {
	label := lw.GetStringParam("label", "My Location")
	lw.SetCurrent("label", label)
}

func (lw *LocationWant) IsAchieved() bool { return false }

func (lw *LocationWant) Progress() {
	ConsumeWebhookAction(&lw.Want, "last_location_at", func(_ string, payload map[string]any) bool {
		lat, hasLat := asFloat64(payload["lat"])
		lng, hasLng := asFloat64(payload["lng"])
		if !hasLat || !hasLng {
			return false
		}

		lw.SetCurrent("lat", lat)
		lw.SetCurrent("lng", lng)
		// A single self-described coordinate object, so the current position can
		// be named as a place in one X-press (naming lat alone would capture only
		// the latitude number). Its "type" makes the naming UI file it under the
		// location_coordinate catalog, which place_arrival resolves.
		lw.SetCurrent("coordinate", map[string]any{"lat": lat, "lng": lng, "type": "location_coordinate"})
		lw.SetCurrent("final_result", fmt.Sprintf(`{"lat":%f,"lng":%f}`, lat, lng))
		if acc, ok := asFloat64(payload["accuracy"]); ok {
			lw.SetCurrent("accuracy", acc)
		}
		if alt, ok := asFloat64(payload["altitude"]); ok {
			lw.SetCurrent("altitude", alt)
		}
		if id, ok := payload["device_id"].(string); ok {
			lw.SetCurrent("device_id", id)
		}
		if name, ok := payload["device_name"].(string); ok {
			lw.SetCurrent("device_name", name)
		}
		if ts, ok := asFloat64(payload["timestamp"]); ok {
			lw.SetCurrent("updated_at", ts)
		}

		// Reverse-geocode if position moved significantly (>~100m) OR if city is not yet set
		lastLat, _ := lw.GetStateFloat64("last_geocoded_lat", 0)
		lastLng, _ := lw.GetStateFloat64("last_geocoded_lng", 0)
		currentCity := GetCurrent[string](&lw.Want, "city", "")
		if currentCity == "" || haversineKm(lat, lng, lastLat, lastLng) > 0.1 {
			if city, prefecture, address, err := reverseGeocode(lat, lng); err == nil {
				lw.SetCurrent("city", city)
				lw.SetCurrent("prefecture", prefecture)
				lw.SetCurrent("address", address)
				lw.StoreState("last_geocoded_lat", lat)
				lw.StoreState("last_geocoded_lng", lng)
			}
		}

		return true
	})
}

type nominatimResponse struct {
	DisplayName string `json:"display_name"`
	Address     struct {
		City          string `json:"city"`
		Municipality  string `json:"municipality"`
		Town          string `json:"town"`
		Village       string `json:"village"`
		CityDistrict  string `json:"city_district"`
		Suburb        string `json:"suburb"`
		Quarter       string `json:"quarter"`
		Neighbourhood string `json:"neighbourhood"`
		County        string `json:"county"`
		Province      string `json:"province"`
		State         string `json:"state"`
		ISO3166Lvl4   string `json:"ISO3166-2-lvl4"`
		Country       string `json:"country"`
	} `json:"address"`
}

var jpPrefectures = map[string]string{
	"JP-01": "北海道", "JP-02": "青森県", "JP-03": "岩手県", "JP-04": "宮城県",
	"JP-05": "秋田県", "JP-06": "山形県", "JP-07": "福島県", "JP-08": "茨城県",
	"JP-09": "栃木県", "JP-10": "群馬県", "JP-11": "埼玉県", "JP-12": "千葉県",
	"JP-13": "東京都", "JP-14": "神奈川県", "JP-15": "新潟県", "JP-16": "富山県",
	"JP-17": "石川県", "JP-18": "福井県", "JP-19": "山梨県", "JP-20": "長野県",
	"JP-21": "岐阜県", "JP-22": "静岡県", "JP-23": "愛知県", "JP-24": "三重県",
	"JP-25": "滋賀県", "JP-26": "京都府", "JP-27": "大阪府", "JP-28": "兵庫県",
	"JP-29": "奈良県", "JP-30": "和歌山県", "JP-31": "鳥取県", "JP-32": "島根県",
	"JP-33": "岡山県", "JP-34": "広島県", "JP-35": "山口県", "JP-36": "徳島県",
	"JP-37": "香川県", "JP-38": "愛媛県", "JP-39": "高知県", "JP-40": "福岡県",
	"JP-41": "佐賀県", "JP-42": "長崎県", "JP-43": "熊本県", "JP-44": "大分県",
	"JP-45": "宮崎県", "JP-46": "鹿児島県", "JP-47": "沖縄県",
}

func reverseGeocode(lat, lng float64) (city, prefecture, address string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/reverse?lat=%f&lon=%f&format=json&accept-language=ja",
		lat, lng,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "mywant-location-want/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var nr nominatimResponse
	if err = json.Unmarshal(body, &nr); err != nil {
		return
	}

	for _, v := range []string{
		nr.Address.City, nr.Address.Municipality, nr.Address.Town,
		nr.Address.CityDistrict, nr.Address.Village, nr.Address.Suburb,
		nr.Address.Quarter, nr.Address.County,
	} {
		if v != "" {
			city = v
			break
		}
	}

	for _, v := range []string{nr.Address.Province, nr.Address.State} {
		if v != "" {
			prefecture = v
			break
		}
	}
	if prefecture == "" {
		if p, ok := jpPrefectures[nr.Address.ISO3166Lvl4]; ok {
			prefecture = p
		}
	}
	address = nr.DisplayName
	return
}

// haversineKm returns approximate distance in km between two lat/lng points.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
