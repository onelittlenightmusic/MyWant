package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/core"
	"time"
)

const agentRestaurantName = "agent_restaurant_premium"

func init() {
	RegisterDoAgent(agentRestaurantName, executeRestaurantReservation)
}

// executeRestaurantReservation performs a premium restaurant reservation
func executeRestaurantReservation(ctx context.Context, want *Want) error {
	schedule := generateRestaurantSchedule(want)
	err := executeReservation(want, agentRestaurantName, schedule, func(s interface{}) (string, string) {
		sch := s.(RestaurantSchedule)
		activity := fmt.Sprintf("Restaurant reservation has been booked at %s %s for %.1f hours",
			sch.RestaurantType, sch.ReservationTime.Format("15:04 Jan 2"), sch.DurationHours)
		logMsg := fmt.Sprintf("Restaurant reservation completed: %s at %s for %.1f hours",
			sch.RestaurantType, sch.ReservationTime.Format("15:04 Jan 2"), sch.DurationHours)
		return activity, logMsg
	})
	if err != nil {
		return err
	}

	return nil
}

// generateRestaurantSchedule creates a restaurant reservation schedule
func generateRestaurantSchedule(want *Want) RestaurantSchedule {
	want.StoreLog("Processing restaurant reservation for %s with premium service", want.Metadata.Name)

	baseDate := time.Now()
	reservationTime := GenerateRandomTimeInRange(baseDate, DinnerTimeRange)
	durationHours := GenerateRandomDuration(1.5, 3.0)

	restaurantType := want.GetStringParam("restaurant_type", "fine dining")
	premiumLevel := want.GetStringParam("premium_level", "premium")
	serviceTier := want.GetStringParam("service_tier", "premium")

	restaurantName := generateRealisticRestaurantName(restaurantType)
	partySize := want.GetIntParam("party_size", 2)
	restaurantCost := want.GetFloatParam("cost", 300.0)
	reservationReference := generateReservationReference()
	formattedReservationName := fmt.Sprintf("%s - Party of %d (%s)", restaurantName, partySize, reservationReference)

	return RestaurantSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		RestaurantType:   restaurantType,
		ReservationName:  formattedReservationName,
		Cost:             restaurantCost,
		PremiumLevel:     premiumLevel,
		ServiceTier:      serviceTier,
		PremiumAmenities: []string{"wine_pairing", "chef_special", "priority_seating"},
	}
}

// generateReservationReference generates a realistic reservation reference code
func generateReservationReference() string {
	referenceNumber := 10000 + rand.Intn(90000)
	return fmt.Sprintf("RES-%d", referenceNumber)
}

// generateRealisticRestaurantName generates realistic restaurant names based on cuisine type
func generateRealisticRestaurantName(cuisineType string) string {
	var names = map[string][]string{
		"fine dining": {
			"L'Élégance", "The Michelin House", "Le Bernardin", "Per Se", "The French Laundry",
			"Alinea", "The Ledbury", "Noma", "Chef's Table", "Sous Vide",
			"La Maison Classique", "The Grand Époque", "Étoile Dorée", "Sage & Stone",
			"The Sterling", "Prestige", "Luxe", "Refined Plate", "Artisan's Haven",
		},
		"casual": {
			"The Garden Bistro", "Rustic Table", "Harvest Kitchen", "Homestead",
			"The Local Taste", "Farm to Fork", "The Cozy Kitchen", "Urban Eats",
			"Downtown Cafe", "The Neighborhood Table", "Common Table", "The Rustic Room",
			"Simple Pleasures", "Kitchen + Co", "The Good Fork", "Community Kitchen",
		},
		"italian": {
			"Trattoria Roma", "La Bella Italia", "Osteria Romana", "Casa Pasta",
			"Il Forno Toscano", "Bella Notte", "Ristorante Venezia", "Amore e Pasta",
			"Tre Stelle", "Cibo Italiano", "La Cucina", "Palazzo Rosso",
			"Dolce Vita", "Aromas of Italy", "La Piazza", "Vinello",
		},
		"french": {
			"Le Petit Parisien", "Maison Provence", "Café de Lyon", "Bistro Montparnasse",
			"Au Bon Goût", "La Belle Époque", "Brasserie Classique", "Le Cordon Bleu",
			"Crêperie Marie", "Petit Salon", "Le Fleur", "Maison Francaise",
			"Au Coin de Rue", "La Boucherie", "Café Lumière", "Petite Escargot",
		},
		"asian": {
			"Jade Garden", "Zen Kitchen", "Cherry Blossom", "Bamboo House",
			"The Golden Dragon", "Mandarin Palace", "Lotus Pavilion", "Silk Road",
			"Wok Master", "Orchid Garden", "Pacific Rim", "Asia House",
			"Royal Thai", "Sakura", "The Spice Route", "Imperial Garden",
		},
		"mexican": {
			"Casa Fiesta", "El Corazón", "Tequila Sunrise", "La Familia",
			"Mariachi's Kitchen", "Hacienda Verde", "Día de Fiesta", "The Bold Flavor",
			"Salsa & Sabor", "Casa de Comida", "Mexicana Auténtica", "El Rancho",
			"Siesta Café", "Fonda de los Andes", "Cantina Real", "Casa Caliente",
		},
		"japanese": {
			"Sakura Sushi", "Koi House", "Zen Ramen", "Tokyo Express",
			"Miyako", "Fumio's Kitchen", "Osaka Grill", "Matcha Garden",
			"Shogun", "Kobe House", "The Sushi Bar", "Tempura House",
			"Arigato", "Hakone", "Yokohama Kitchen", "Tsukiji",
		},
		"steakhouse": {
			"The Prime Cut", "Bone & Barrel", "Wagyu House", "The Smokehouse",
			"Texas Grill", "Rare & Medium", "The Cattleman's", "Iron Range",
			"Beef House", "The Chop", "Steak & Stone", "The Butcher's Table",
			"Crown of Beef", "The Premium", "Ribeye Room", "BBQ & Co",
		},
		"seafood": {
			"The Lobster Cove", "Catch of the Day", "The Oyster House", "Sea Pearl",
			"Harbor View", "Dockside Grille", "The Fish House", "Reef Restaurant",
			"Captain's Table", "Neptune's Kitchen", "The Fisherman's Wharf", "Coastal Catch",
			"Blue Water", "The Crab Shack", "Salted Anchor", "Marina Fish House",
		},
		"mediterranean": {
			"Solstice", "The Greek Taverna", "Olive Grove", "Mediterranean Dream",
			"Villa Tuscana", "The Aegean", "Sunny Terrace", "Coastal Haven",
			"Piazza Toscana", "The Mezze House", "Cyprus Kitchen", "Santorini Blue",
			"Olive & Grape", "Aqua Marina", "The Grecian", "Island Kitchen",
		},
		"vegetarian": {
			"The Green Leaf", "Garden Kitchen", "Sprout & Root", "Harvest Plate",
			"Greenery", "The Plant Based", "Wholesome Table", "Farm Fresh Kitchen",
			"Nature's Bounty", "Organic Garden", "Green Plate", "Roots & Shoots",
			"The Veggie House", "Leaf & Grain", "Pure Green", "Botanical Kitchen",
		},
	}

	if list, exists := names[cuisineType]; exists {
		return list[rand.Intn(len(list))]
	}

	return names["fine dining"][rand.Intn(len(names["fine dining"]))]
}
