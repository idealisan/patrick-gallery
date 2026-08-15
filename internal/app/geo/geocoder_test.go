package geo

import "testing"

func TestLoadAndReverse(t *testing.T) {
	g, err := NewGeocoder()
	if err != nil {
		t.Fatalf("NewGeocoder: %v", err)
	}
	if g.Cities() == 0 {
		t.Fatal("expected cities to be loaded")
	}
	if len(g.countries) == 0 {
		t.Fatal("expected countries to be loaded")
	}

	// A point in the centre of Paris should resolve to Paris, France.
	city, state, country := g.Reverse(48.8566, 2.3522)
	if city == "" || country == "" {
		t.Fatalf("Paris reverse-geocode returned empty: city=%q country=%q", city, country)
	}
	if country != "France" {
		t.Errorf("expected country France, got %q (city=%q)", country, city)
	}
	t.Logf("Paris -> city=%q state=%q country=%q", city, state, country)

	// A point in central Tokyo should resolve to Tokyo, Japan.
	city2, _, country2 := g.Reverse(35.6895, 139.6917)
	if country2 != "Japan" {
		t.Errorf("expected country Japan, got %q (city=%q)", country2, city2)
	}

	// A point in the middle of an ocean should return empty (no city within
	// 100km).
	if c, _, co := g.Reverse(0.0, -160.0); c != "" || co != "" {
		t.Errorf("open-ocean point should not reverse-geocode, got city=%q country=%q", c, co)
	}
}

func TestSharedSingleton(t *testing.T) {
	a, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, _ := Load()
	if a == nil {
		t.Fatal("Load returned nil")
	}
	if a != b {
		t.Error("Load should return the same singleton instance")
	}
}
