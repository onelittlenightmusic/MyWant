package types

import (
	"context"
	"fmt"
	"math/rand"
	. "mywant/engine/src"
	"time"
)

// AgentRestaurant extends DoAgent with restaurant reservation capabilities
type AgentRestaurant struct {
	DoAgent
	PremiumLevel string
	ServiceTier  string
}

// NewAgentRestaurant creates a new restaurant agent
func NewAgentRestaurant(name string, capabilities []string, uses []string, premiumLevel string) *AgentRestaurant {
	return &AgentRestaurant{
		DoAgent: DoAgent{
			BaseAgent: *NewBaseAgent(name, capabilities, DoAgentType),
		},
		PremiumLevel: premiumLevel,
		ServiceTier:  "premium",
	}
}

// Exec executes restaurant agent actions and returns RestaurantSchedule
func (a *AgentRestaurant) Exec(ctx context.Context, want *Want) error {
	// Generate restaurant reservation schedule
	schedule := a.generateRestaurantSchedule(want)
	want.StoreStateForAgent("agent_result", schedule)

	// Record activity description for agent history
	activity := fmt.Sprintf("Restaurant reservation has been booked at %s %s for %.1f hours",
		schedule.RestaurantType, schedule.ReservationTime.Format("15:04 Jan 2"), schedule.DurationHours)
	want.SetAgentActivity(a.Name, activity)

	want.StoreLog(fmt.Sprintf("Restaurant reservation completed: %s at %s for %.1f hours",
		schedule.RestaurantType, schedule.ReservationTime.Format("15:04 Jan 2"), schedule.DurationHours))

	return nil
}

// generateRestaurantSchedule creates a restaurant reservation schedule
func (a *AgentRestaurant) generateRestaurantSchedule(want *Want) RestaurantSchedule {
	want.StoreLog(fmt.Sprintf("Processing restaurant reservation for %s with premium service", want.Metadata.Name))

	// Generate restaurant reservation with appropriate timing
	baseDate := time.Now()
	// Restaurant reservations typically in evening hours (6-9 PM)
	reservationTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
		18+rand.Intn(3), rand.Intn(60), 0, 0, time.Local) // 6-9 PM

	// Restaurant meals typically 1.5-3 hours
	durationHours := 1.5 + rand.Float64()*1.5 // 1.5-3 hours

	// Extract restaurant type from want parameters
	restaurantType := want.GetStringParam("restaurant_type", "fine dining")

	// Generate realistic restaurant names
	restaurantName := a.generateRealisticRestaurantName(restaurantType)

	// Generate realistic reservation name with party size reference
	partySize := want.GetIntParam("party_size", 2)
	reservationReference := a.generateReservationReference()
	formattedReservationName := fmt.Sprintf("%s - Party of %d (%s)", restaurantName, partySize, reservationReference)
	return RestaurantSchedule{
		ReservationTime:  reservationTime,
		DurationHours:    durationHours,
		RestaurantType:   restaurantType,
		ReservationName:  formattedReservationName,
		PremiumLevel:     a.PremiumLevel,
		ServiceTier:      a.ServiceTier,
		PremiumAmenities: []string{"wine_pairing", "chef_special", "priority_seating"},
	}
}

// generateReservationReference generates a realistic reservation reference code
func (a *AgentRestaurant) generateReservationReference() string {
	// Generate reference codes like "RES-12345", "RES-67890"
	referenceNumber := 10000 + rand.Intn(90000)
	return fmt.Sprintf("RES-%d", referenceNumber)
}

// generateRealisticRestaurantName generates realistic restaurant names based on cuisine type
func (a *AgentRestaurant) generateRealisticRestaurantName(cuisineType string) string {
	// Realistic restaurant name patterns by cuisine type
	var names map[string][]string = map[string][]string{
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

	// Select appropriate category or default to fine dining
	category := cuisineType
	if list, exists := names[category]; exists {
		return list[rand.Intn(len(list))]
	}

	// Fallback for unknown cuisine types
	return names["fine dining"][rand.Intn(len(names["fine dining"]))]
}
