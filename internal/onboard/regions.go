package onboard

// RegionMapping represents a country and its available GCP regions
type RegionMapping struct {
	Country     string
	DisplayName string
	Regions     []Region
}

// Region represents a GCP region with a display name
type Region struct {
	Code        string // GCP region code (e.g., "us-central1")
	DisplayName string // Human-readable name (e.g., "Iowa")
}

// GetRegionMappings returns all available GCP region mappings
func GetRegionMappings() []RegionMapping {
	return []RegionMapping{
		{
			Country:     "us",
			DisplayName: "United States",
			Regions: []Region{
				{Code: "us-central1", DisplayName: "Iowa"},
				{Code: "us-east1", DisplayName: "South Carolina"},
				{Code: "us-east4", DisplayName: "Northern Virginia"},
				{Code: "us-east5", DisplayName: "Columbus, Ohio"},
				{Code: "us-south1", DisplayName: "Dallas, Texas"},
				{Code: "us-west1", DisplayName: "Oregon"},
				{Code: "us-west2", DisplayName: "Los Angeles"},
				{Code: "us-west3", DisplayName: "Salt Lake City"},
				{Code: "us-west4", DisplayName: "Las Vegas"},
			},
		},
		{
			Country:     "ca",
			DisplayName: "Canada",
			Regions: []Region{
				{Code: "northamerica-northeast1", DisplayName: "Montreal"},
				{Code: "northamerica-northeast2", DisplayName: "Toronto"},
			},
		},
		{
			Country:     "eu",
			DisplayName: "Europe",
			Regions: []Region{
				{Code: "europe-west1", DisplayName: "Belgium"},
				{Code: "europe-west2", DisplayName: "London, UK"},
				{Code: "europe-west3", DisplayName: "Frankfurt, Germany"},
				{Code: "europe-west4", DisplayName: "Netherlands"},
				{Code: "europe-west6", DisplayName: "Zurich, Switzerland"},
				{Code: "europe-west8", DisplayName: "Milan, Italy"},
				{Code: "europe-west9", DisplayName: "Paris, France"},
				{Code: "europe-west10", DisplayName: "Berlin, Germany"},
				{Code: "europe-west12", DisplayName: "Turin, Italy"},
				{Code: "europe-north1", DisplayName: "Finland"},
				{Code: "europe-central2", DisplayName: "Warsaw, Poland"},
				{Code: "europe-southwest1", DisplayName: "Madrid, Spain"},
			},
		},
		{
			Country:     "asia",
			DisplayName: "Asia Pacific",
			Regions: []Region{
				{Code: "asia-east1", DisplayName: "Taiwan"},
				{Code: "asia-east2", DisplayName: "Hong Kong"},
				{Code: "asia-northeast1", DisplayName: "Tokyo, Japan"},
				{Code: "asia-northeast2", DisplayName: "Osaka, Japan"},
				{Code: "asia-northeast3", DisplayName: "Seoul, South Korea"},
				{Code: "asia-south1", DisplayName: "Mumbai, India"},
				{Code: "asia-south2", DisplayName: "Delhi, India"},
				{Code: "asia-southeast1", DisplayName: "Singapore"},
				{Code: "asia-southeast2", DisplayName: "Jakarta, Indonesia"},
			},
		},
		{
			Country:     "au",
			DisplayName: "Australia",
			Regions: []Region{
				{Code: "australia-southeast1", DisplayName: "Sydney"},
				{Code: "australia-southeast2", DisplayName: "Melbourne"},
			},
		},
		{
			Country:     "sa",
			DisplayName: "South America",
			Regions: []Region{
				{Code: "southamerica-east1", DisplayName: "Sao Paulo, Brazil"},
				{Code: "southamerica-west1", DisplayName: "Santiago, Chile"},
			},
		},
	}
}

// GetRegionsByCountry returns regions for a specific country code
func GetRegionsByCountry(country string) []Region {
	mappings := GetRegionMappings()
	for _, mapping := range mappings {
		if mapping.Country == country {
			return mapping.Regions
		}
	}
	return []Region{}
}

// ValidateRegion checks if a region code is valid for a given country
func ValidateRegion(country, regionCode string) bool {
	regions := GetRegionsByCountry(country)
	for _, region := range regions {
		if region.Code == regionCode {
			return true
		}
	}
	return false
}
