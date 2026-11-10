package types

import "testing"

func TestNearestPlaceIsTheNearestWithinItsRadius(t *testing.T) {
	places := []savedPlace{
		{name: "会社", lat: 35.6895, lng: 139.6917, radius: 150},
		{name: "駅", lat: 35.6900, lng: 139.6917, radius: 150}, // ~56m north of 会社
		{name: "家", lat: 35.7000, lng: 139.7000, radius: 150},
	}
	// ~11m north of 会社: both 会社 and 駅 are near; 会社 is nearer.
	p, _, ok := nearestPlace(places, 35.6896, 139.6917, 1)
	if !ok || p.name != "会社" {
		t.Fatalf("got %q %v, want 会社", p.name, ok)
	}
	// ~1km away from all of them.
	if p, _, ok := nearestPlace(places, 35.6800, 139.6917, 1); ok {
		t.Fatalf("got %q, want none", p.name)
	}
	// 200m from 家: out of its radius, inside it times the leave factor.
	if _, _, ok := nearestPlace(places[2:], 35.7018, 139.7000, 1); ok {
		t.Fatal("200m from 家 counted as at it")
	}
	if _, _, ok := nearestPlace(places[2:], 35.7018, 139.7000, placeLeaveFactor); !ok {
		t.Fatal("200m from 家 counted as left it")
	}
}
