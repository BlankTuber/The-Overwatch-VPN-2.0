package regions

import (
	"strings"

	"quidque.no/ow2-ip-puller/internal/api"
)

// Define regions
type Region string

const (
	EU  Region = "EU"      // Europe
	NA  Region = "NA"      // North America
	SA  Region = "SA"      // South America
	AFR Region = "Afr"     // Africa
	AS  Region = "As"      // Asia
	ME  Region = "ME"      // Middle East
	OCE Region = "Oce"     // Oceania
	UNK Region = "Unknown" // Unknown
)

// Map country codes to regions
var regionMap map[string]Region

// InitRegionMap initializes the region map
func InitRegionMap() {
	regionMap = make(map[string]Region)

	// Add countries to Europe region
	addCountriesToRegion(EU, []string{
		"AL", "AD", "AT", "BY", "BE", "BA", "BG", "HR", "CY", "CZ", "DK",
		"EE", "FI", "FR", "DE", "GR", "HU", "IS", "IE", "IT", "LV", "LI",
		"LT", "LU", "MK", "MT", "MD", "MC", "ME", "NL", "NO", "PL", "PT",
		"RO", "RU", "SM", "RS", "SK", "SI", "ES", "SE", "CH", "UA", "GB", "VA", "UK",
	})

	// Add countries to North America region
	addCountriesToRegion(NA, []string{
		"CA", "US", "MX", "GL", "BM", "PM",
	})

	// Add countries to South America region
	addCountriesToRegion(SA, []string{
		"AR", "BO", "BR", "CL", "CO", "EC", "FK", "GF", "GY", "PY", "PE", "SR", "UY", "VE",
	})

	// Add countries to Africa region
	addCountriesToRegion(AFR, []string{
		"DZ", "AO", "BJ", "BW", "BF", "BI", "CM", "CV", "CF", "TD", "KM", "CG", "CD",
		"DJ", "EG", "GQ", "ER", "ET", "GA", "GM", "GH", "GN", "GW", "CI", "KE", "LS",
		"LR", "LY", "MG", "MW", "ML", "MR", "MU", "MA", "MZ", "NA", "NE", "NG", "RW",
		"ST", "SN", "SC", "SL", "SO", "ZA", "SS", "SD", "SZ", "TZ", "TG", "TN", "UG", "ZM", "ZW",
	})

	// Add countries to Asia region
	addCountriesToRegion(AS, []string{
		"AF", "AM", "AZ", "BD", "BT", "BN", "KH", "CN", "GE", "HK", "IN", "ID",
		"JP", "KZ", "KP", "KR", "KG", "LA", "MO", "MY", "MV", "MN", "MM", "NP",
		"PK", "PH", "SG", "LK", "TW", "TJ", "TH", "TL", "TM", "UZ", "VN",
	})

	// Add countries to Middle East region
	addCountriesToRegion(ME, []string{
		"BH", "IR", "IQ", "IL", "JO", "KW", "LB", "OM", "PS", "QA", "SA", "SY", "TR", "AE", "YE",
	})

	// Add countries to Oceania region
	addCountriesToRegion(OCE, []string{
		"AU", "FJ", "KI", "MH", "FM", "NR", "NZ", "PW", "PG", "WS", "SB", "TO", "TV", "VU",
	})
}

// Add multiple countries to a region
func addCountriesToRegion(region Region, countryCodes []string) {
	for _, code := range countryCodes {
		regionMap[code] = region
	}
}

// GetRegionByCountryCode returns the region for a country code
func GetRegionByCountryCode(countryCode string) Region {
	if countryCode == "" {
		return UNK
	}

	region, exists := regionMap[strings.ToUpper(countryCode)]
	if !exists {
		return UNK
	}
	return region
}

// CategorizeIPsByRegion categorizes IPs by region
func CategorizeIPsByRegion(response *api.Response) map[Region][]string {
	ipsByRegion := make(map[Region][]string)

	// Process IPv4 prefixes
	for _, prefix := range response.Data.IPv4Prefixes {
		region := GetRegionByCountryCode(prefix.CountryCode)
		ipsByRegion[region] = append(ipsByRegion[region], prefix.Prefix)
	}

	// Process IPv6 prefixes
	for _, prefix := range response.Data.IPv6Prefixes {
		region := GetRegionByCountryCode(prefix.CountryCode)
		ipsByRegion[region] = append(ipsByRegion[region], prefix.Prefix)
	}

	return ipsByRegion
}
