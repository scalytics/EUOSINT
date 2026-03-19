// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"regexp"
	"strings"
)

// countryGeo holds a country centroid for map placement.
type countryGeo struct {
	Name string
	Code string
	Lat  float64
	Lng  float64
}

// geoCountries is the lookup table for geocoding country mentions in
// alert titles/summaries. Centroids are approximate and intentionally
// simple — only enough precision for a world map pin.
var geoCountries = []countryGeo{
	{"Afghanistan", "AF", 33.93, 67.71},
	{"Albania", "AL", 41.15, 20.17},
	{"Algeria", "DZ", 28.03, 1.66},
	{"Angola", "AO", -11.20, 17.87},
	{"Argentina", "AR", -38.42, -63.62},
	{"Armenia", "AM", 40.07, 45.04},
	{"Australia", "AU", -25.27, 133.78},
	{"Austria", "AT", 47.52, 14.55},
	{"Azerbaijan", "AZ", 40.14, 47.58},
	{"Bahrain", "BH", 26.07, 50.55},
	{"Bangladesh", "BD", 23.68, 90.36},
	{"Belarus", "BY", 53.71, 27.95},
	{"Belgium", "BE", 50.50, 4.47},
	{"Benin", "BJ", 9.31, 2.32},
	{"Bolivia", "BO", -16.29, -63.59},
	{"Bosnia", "BA", 43.92, 17.68},
	{"Botswana", "BW", -22.33, 24.68},
	{"Brazil", "BR", -14.24, -51.93},
	{"Bulgaria", "BG", 42.73, 25.49},
	{"Burkina Faso", "BF", 12.24, -1.56},
	{"Burundi", "BI", -3.37, 29.92},
	{"Cambodia", "KH", 12.57, 104.99},
	{"Cameroon", "CM", 7.37, 12.35},
	{"Canada", "CA", 56.13, -106.35},
	{"Central African Republic", "CF", 6.61, 20.94},
	{"Chad", "TD", 15.45, 18.73},
	{"Chile", "CL", -35.68, -71.54},
	{"China", "CN", 35.86, 104.20},
	{"Colombia", "CO", 4.57, -74.30},
	{"Congo", "CD", -4.04, 21.76},
	{"Costa Rica", "CR", 9.75, -83.75},
	{"Croatia", "HR", 45.10, 15.20},
	{"Cuba", "CU", 21.52, -77.78},
	{"Cyprus", "CY", 35.13, 33.43},
	{"Czech Republic", "CZ", 49.82, 15.47},
	{"Denmark", "DK", 56.26, 9.50},
	{"Dominican Republic", "DO", 18.74, -70.16},
	{"Ecuador", "EC", -1.83, -78.18},
	{"Egypt", "EG", 26.82, 30.80},
	{"El Salvador", "SV", 13.79, -88.90},
	{"Eritrea", "ER", 15.18, 39.78},
	{"Estonia", "EE", 58.60, 25.01},
	{"Ethiopia", "ET", 9.14, 40.49},
	{"Finland", "FI", 61.92, 25.75},
	{"France", "FR", 46.23, 2.21},
	{"Gabon", "GA", -0.80, 11.61},
	{"Gambia", "GM", 13.44, -15.31},
	{"Gaza", "PS", 31.35, 34.31},
	{"Georgia", "GE", 42.32, 43.36},
	{"Germany", "DE", 51.17, 10.45},
	{"Ghana", "GH", 7.95, -1.02},
	{"Greece", "GR", 39.07, 21.82},
	{"Guatemala", "GT", 15.78, -90.23},
	{"Guinea", "GN", 9.95, -9.70},
	{"Haiti", "HT", 18.97, -72.29},
	{"Honduras", "HN", 15.20, -86.24},
	{"Hungary", "HU", 47.16, 19.50},
	{"India", "IN", 20.59, 78.96},
	{"Indonesia", "ID", -0.79, 113.92},
	{"Iran", "IR", 32.43, 53.69},
	{"Iraq", "IQ", 33.22, 43.68},
	{"Ireland", "IE", 53.14, -7.69},
	{"Israel", "IL", 31.05, 34.85},
	{"Italy", "IT", 41.87, 12.57},
	{"Ivory Coast", "CI", 7.54, -5.55},
	{"Jamaica", "JM", 18.11, -77.30},
	{"Japan", "JP", 36.20, 138.25},
	{"Jordan", "JO", 30.59, 36.24},
	{"Kazakhstan", "KZ", 48.02, 66.92},
	{"Kenya", "KE", -0.02, 37.91},
	{"Kosovo", "XK", 42.60, 20.90},
	{"Kuwait", "KW", 29.31, 47.48},
	{"Kyrgyzstan", "KG", 41.20, 74.77},
	{"Laos", "LA", 19.86, 102.50},
	{"Latvia", "LV", 56.88, 24.60},
	{"Lebanon", "LB", 33.85, 35.86},
	{"Libya", "LY", 26.34, 17.23},
	{"Lithuania", "LT", 55.17, 23.88},
	{"Madagascar", "MG", -18.77, 46.87},
	{"Malawi", "MW", -13.25, 34.30},
	{"Malaysia", "MY", 4.21, 101.98},
	{"Mali", "ML", 17.57, -4.00},
	{"Malta", "MT", 35.90, 14.51},
	{"Mauritania", "MR", 21.01, -10.94},
	{"Mexico", "MX", 23.63, -102.55},
	{"Moldova", "MD", 47.41, 28.37},
	{"Mongolia", "MN", 46.86, 103.85},
	{"Montenegro", "ME", 42.71, 19.37},
	{"Morocco", "MA", 31.79, -7.09},
	{"Mozambique", "MZ", -18.67, 35.53},
	{"Myanmar", "MM", 21.91, 95.96},
	{"Namibia", "NA", -22.96, 18.49},
	{"Nepal", "NP", 28.39, 84.12},
	{"Netherlands", "NL", 52.13, 5.29},
	{"New Zealand", "NZ", -40.90, 174.89},
	{"Nicaragua", "NI", 12.87, -85.21},
	{"Niger", "NE", 17.61, 8.08},
	{"Nigeria", "NG", 9.08, 8.68},
	{"North Korea", "KP", 40.34, 127.51},
	{"North Macedonia", "MK", 41.51, 21.75},
	{"Norway", "NO", 60.47, 8.47},
	{"Oman", "OM", 21.51, 55.92},
	{"Pakistan", "PK", 30.38, 69.35},
	{"Palestine", "PS", 31.95, 35.23},
	{"Panama", "PA", 8.54, -80.78},
	{"Papua New Guinea", "PG", -6.31, 143.96},
	{"Paraguay", "PY", -23.44, -58.44},
	{"Peru", "PE", -9.19, -75.02},
	{"Philippines", "PH", 12.88, 121.77},
	{"Poland", "PL", 51.92, 19.15},
	{"Portugal", "PT", 39.40, -8.22},
	{"Qatar", "QA", 25.35, 51.18},
	{"Romania", "RO", 45.94, 24.97},
	{"Russia", "RU", 61.52, 105.32},
	{"Rwanda", "RW", -1.94, 29.87},
	{"Saudi Arabia", "SA", 23.89, 45.08},
	{"Senegal", "SN", 14.50, -14.45},
	{"Serbia", "RS", 44.02, 21.01},
	{"Sierra Leone", "SL", 8.46, -11.78},
	{"Singapore", "SG", 1.35, 103.82},
	{"Slovakia", "SK", 48.67, 19.70},
	{"Slovenia", "SI", 46.15, 14.99},
	{"Somalia", "SO", 5.15, 46.20},
	{"South Africa", "ZA", -30.56, 22.94},
	{"South Korea", "KR", 35.91, 127.77},
	{"South Sudan", "SS", 6.88, 31.31},
	{"Spain", "ES", 40.46, -3.75},
	{"Sri Lanka", "LK", 7.87, 80.77},
	{"Sudan", "SD", 12.86, 30.22},
	{"Sweden", "SE", 60.13, 18.64},
	{"Switzerland", "CH", 46.82, 8.23},
	{"Syria", "SY", 34.80, 38.99},
	{"Taiwan", "TW", 23.70, 120.96},
	{"Tajikistan", "TJ", 38.86, 71.28},
	{"Tanzania", "TZ", -6.37, 34.89},
	{"Thailand", "TH", 15.87, 100.99},
	{"Togo", "TG", 8.62, 1.21},
	{"Tunisia", "TN", 33.89, 9.54},
	{"Turkey", "TR", 38.96, 35.24},
	{"Turkmenistan", "TM", 38.97, 59.56},
	{"Uganda", "UG", 1.37, 32.29},
	{"Ukraine", "UA", 48.38, 31.17},
	{"United Arab Emirates", "AE", 23.42, 53.85},
	{"United Kingdom", "GB", 55.38, -3.44},
	{"United States", "US", 37.09, -95.71},
	{"Uruguay", "UY", -32.52, -55.77},
	{"Uzbekistan", "UZ", 41.38, 64.59},
	{"Venezuela", "VE", 6.42, -66.59},
	{"Vietnam", "VN", 14.06, 108.28},
	{"Yemen", "YE", 15.55, 48.52},
	{"Zambia", "ZM", -13.13, 28.64},
	{"Zimbabwe", "ZW", -19.02, 29.15},
}

// geoAliases maps alternative names, adjectives, and region names to
// the canonical country name used in geoCountries.
var geoAliases = map[string]string{
	// Adjective forms
	"afghan":      "Afghanistan",
	"american":    "United States",
	"australian":  "Australia",
	"austrian":    "Austria",
	"algerian":    "Algeria",
	"angolan":     "Angola",
	"argentine":   "Argentina",
	"armenian":    "Armenia",
	"azerbaijani": "Azerbaijan",
	"bahraini":    "Bahrain",
	"bangladeshi": "Bangladesh",
	"belarusian":  "Belarus",
	"belgian":     "Belgium",
	"bolivian":    "Bolivia",
	"bosnian":     "Bosnia",
	"brazilian":   "Brazil",
	"burmese":     "Myanmar",
	"burundian":   "Burundi",
	"cambodian":   "Cambodia",
	"cameroonian": "Cameroon",
	"chadian":     "Chad",
	"chilean":     "Chile",
	"chinese":     "China",
	"cypriot":     "Cyprus",
	"czech":       "Czech Republic",
	"danish":      "Denmark",
	"dutch":       "Netherlands",
	"colombian":   "Colombia",
	"congolese":   "Congo",
	"cuban":       "Cuba",
	"ecuadorian":  "Ecuador",
	"egyptian":    "Egypt",
	"finnish":     "Finland",
	"french":      "France",
	"eritrean":    "Eritrea",
	"ethiopian":   "Ethiopia",
	"gambian":     "Gambia",
	"georgian":    "Georgia",
	"ghanaian":    "Ghana",
	"guatemalan":  "Guatemala",
	"german":      "Germany",
	"greek":       "Greece",
	"guinean":     "Guinea",
	"hungarian":   "Hungary",
	"haitian":     "Haiti",
	"honduran":    "Honduras",
	"indian":      "India",
	"indonesian":  "Indonesia",
	"iranian":     "Iran",
	"iraqi":       "Iraq",
	"irish":       "Ireland",
	"israeli":     "Israel",
	"italian":     "Italy",
	"jamaican":    "Jamaica",
	"japanese":    "Japan",
	"ivorian":     "Ivory Coast",
	"jordanian":   "Jordan",
	"kazakh":      "Kazakhstan",
	"kenyan":      "Kenya",
	"kosovar":     "Kosovo",
	"kuwaiti":     "Kuwait",
	"latvian":     "Latvia",
	"kyrgyz":      "Kyrgyzstan",
	"lebanese":    "Lebanon",
	"libyan":      "Libya",
	"lithuanian":  "Lithuania",
	"malagasy":    "Madagascar",
	"malawian":    "Malawi",
	"malaysian":   "Malaysia",
	"malian":      "Mali",
	"maltese":     "Malta",
	"mauritanian": "Mauritania",
	"mexican":     "Mexico",
	"moldovan":    "Moldova",
	"mongolian":   "Mongolia",
	"moroccan":    "Morocco",
	"mozambican":  "Mozambique",
	"namibian":    "Namibia",
	"nepalese":    "Nepal",
	"nicaraguan":  "Nicaragua",
	"nigerien":    "Niger",
	"nigerian":    "Nigeria",
	"norwegian":   "Norway",
	"pakistani":   "Pakistan",
	"palestinian": "Palestine",
	"panamanian":  "Panama",
	"paraguayan":  "Paraguay",
	"peruvian":    "Peru",
	"philippine":  "Philippines",
	"polish":      "Poland",
	"portuguese":  "Portugal",
	"omani":       "Oman",
	"qatari":      "Qatar",
	"romanian":    "Romania",
	"russian":     "Russia",
	"rwandan":     "Rwanda",
	"salvadoran":  "El Salvador",
	"saudi":       "Saudi Arabia",
	"senegalese":  "Senegal",
	"serbian":     "Serbia",
	"somali":      "Somalia",
	"spanish":     "Spain",
	"sri lankan":  "Sri Lanka",
	"sudanese":    "Sudan",
	"swedish":     "Sweden",
	"swiss":       "Switzerland",
	"syrian":      "Syria",
	"tajik":       "Tajikistan",
	"tanzanian":   "Tanzania",
	"thai":        "Thailand",
	"tunisian":    "Tunisia",
	"turkish":     "Turkey",
	"turkmen":     "Turkmenistan",
	"ugandan":     "Uganda",
	"ukrainian":   "Ukraine",
	"uzbek":       "Uzbekistan",
	"venezuelan":  "Venezuela",
	"vietnamese":  "Vietnam",
	"yemeni":      "Yemen",
	"zambian":     "Zambia",
	"zimbabwean":  "Zimbabwe",

	// Alternative / short names
	"drc":                          "Congo",
	"democratic republic of congo": "Congo",
	"cote d'ivoire":                "Ivory Coast",
	"côte d'ivoire":                "Ivory Coast",
	"rok":                          "South Korea",
	"dprk":                         "North Korea",
	"uae":                          "United Arab Emirates",
	"emirates":                     "United Arab Emirates",
	"uk":                           "United Kingdom",
	"britain":                      "United Kingdom",
	"british":                      "United Kingdom",

	// Conflict regions / sub-national areas → parent country
	"tigray":           "Ethiopia",
	"amhara":           "Ethiopia",
	"oromia":           "Ethiopia",
	"rakhine":          "Myanmar",
	"shan":             "Myanmar",
	"kachin":           "Myanmar",
	"darfur":           "Sudan",
	"kordofan":         "Sudan",
	"blue nile":        "Sudan",
	"donbas":           "Ukraine",
	"donbass":          "Ukraine",
	"donetsk":          "Ukraine",
	"luhansk":          "Ukraine",
	"crimea":           "Ukraine",
	"kherson":          "Ukraine",
	"zaporizhzhia":     "Ukraine",
	"idlib":            "Syria",
	"aleppo":           "Syria",
	"golan":            "Syria",
	"sinai":            "Egypt",
	"sahel":            "Mali",
	"cabo delgado":     "Mozambique",
	"kivu":             "Congo",
	"ituri":            "Congo",
	"kasai":            "Congo",
	"west bank":        "Palestine",
	"hebron":           "Palestine",
	"jenin":            "Palestine",
	"nablus":           "Palestine",
	"rafah":            "Gaza",
	"khan younis":      "Gaza",
	"balochistan":      "Pakistan",
	"waziristan":       "Pakistan",
	"kashmir":          "India",
	"nagorno-karabakh": "Azerbaijan",
	"karabakh":         "Azerbaijan",
	"mindanao":         "Philippines",
	"marawi":           "Philippines",
	"helmand":          "Afghanistan",
	"kandahar":         "Afghanistan",
	"kabul":            "Afghanistan",
	"mogadishu":        "Somalia",
	"benghazi":         "Libya",
	"tripoli":          "Libya",
	"mosul":            "Iraq",
	"kirkuk":           "Iraq",
	"basra":            "Iraq",
	"aden":             "Yemen",
	"sanaa":            "Yemen",
	"marib":            "Yemen",
	"hodeida":          "Yemen",
	"hodeidah":         "Yemen",
	"taipei":           "Taiwan",
	"valletta":         "Malta",
	"kyiv":             "Ukraine",
	"kharkiv":          "Ukraine",
}

// geoIndex maps lowercased country names to their centroid. Built once at init.
var geoIndex map[string]*countryGeo

var eventCountryPrefixRe = regexp.MustCompile(`(?i)(?:^|[\s,;:(\[])(?:in|near|at|off|outside|inside|across|around|within|throughout|into)\s+(?:the\s+)?$`)
var likelyNationalityAliasExceptions = map[string]struct{}{
	"uk":      {},
	"drc":     {},
	"rok":     {},
	"dprk":    {},
	"uae":     {},
	"britain": {},
}

func init() {
	geoIndex = make(map[string]*countryGeo, len(geoCountries)*2)
	for i := range geoCountries {
		g := &geoCountries[i]
		geoIndex[strings.ToLower(g.Name)] = g
	}
	// Wire aliases → canonical entries.
	for alias, canonical := range geoAliases {
		if g, ok := geoIndex[strings.ToLower(canonical)]; ok {
			geoIndex[strings.ToLower(alias)] = g
		}
	}
}

// geocodeCountryCode returns the capital city coordinates for a 2-letter
// country code, falling back to geographic centroid if no capital is known.
func geocodeCountryCode(code string) (lat, lng float64, name string, ok bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	// Prefer capital city coords (fixes islands-in-water problem).
	if capital, cok := capitalCoords[code]; cok {
		for i := range geoCountries {
			if geoCountries[i].Code == code {
				return capital[0], capital[1], geoCountries[i].Name, true
			}
		}
		return capital[0], capital[1], code, true
	}
	for i := range geoCountries {
		if geoCountries[i].Code == code {
			return geoCountries[i].Lat, geoCountries[i].Lng, geoCountries[i].Name, true
		}
	}
	return 0, 0, "", false
}

// geocodeText scans text for country/region mentions and returns the
// centroid of the best match. Prefers the rightmost mention in the text
// (headlines typically put the subject location last, e.g. "Israeli
// Strikes on Gaza" → Gaza). When two matches end at the same position,
// the longer match wins (e.g. "South Sudan" over "Sudan").
func geocodeText(text string) (lat, lng float64, code string, ok bool) {
	lower := strings.ToLower(text)
	titleCut := strings.Index(lower, "\n")
	if titleCut < 0 {
		titleCut = len(lower)
	}

	bestScore := -1 << 30
	bestPos := -1
	bestLen := 0
	var bestGeo *countryGeo

	for key, g := range geoIndex {
		searchEnd := len(lower)
		for {
			idx := strings.LastIndex(lower[:searchEnd], key)
			if idx < 0 {
				break
			}
			endPos := idx + len(key)
			// Require word boundaries to prevent substring false positives
			// (e.g. "oman" inside "Romania", "china" inside "Chinaware").
			if isWordBoundary(lower, idx, endPos) {
				score := endPos
				if idx < titleCut {
					score += 20000
				}
				if hasEventLocationPrefix(lower, idx) {
					score += 100000
				}
				if isLikelyNationalityAlias(key, g.Name) {
					score -= 30000
				}
				if score > bestScore || (score == bestScore && (endPos > bestPos || (endPos == bestPos && len(key) > bestLen))) {
					bestScore = score
					bestPos = endPos
					bestLen = len(key)
					bestGeo = g
				}
			}
			if idx == 0 {
				break
			}
			searchEnd = idx
		}
	}
	if bestGeo != nil {
		return bestGeo.Lat, bestGeo.Lng, bestGeo.Code, true
	}
	return 0, 0, "", false
}

func hasEventLocationPrefix(text string, idx int) bool {
	if idx <= 0 {
		return false
	}
	start := idx - 48
	if start < 0 {
		start = 0
	}
	prefix := text[start:idx]
	return eventCountryPrefixRe.MatchString(prefix)
}

func isLikelyNationalityAlias(key string, canonical string) bool {
	key = strings.TrimSpace(strings.ToLower(key))
	canonical = strings.TrimSpace(strings.ToLower(canonical))
	if key == "" || canonical == "" || key == canonical {
		return false
	}
	if _, ok := likelyNationalityAliasExceptions[key]; ok {
		return false
	}
	if strings.Contains(key, " ") {
		return false
	}
	return strings.HasSuffix(key, "ian") ||
		strings.HasSuffix(key, "ean") ||
		strings.HasSuffix(key, "ish") ||
		strings.HasSuffix(key, "ese") ||
		strings.HasSuffix(key, "ani") ||
		strings.HasSuffix(key, "i")
}

// isWordBoundary checks that the substring at text[start:end] is bounded by
// non-letter characters (or string edges). Prevents "oman" matching inside
// "Romania" or "iran" matching inside "Iranians".
func isWordBoundary(text string, start, end int) bool {
	if start > 0 {
		r := rune(text[start-1])
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return false
		}
	}
	if end < len(text) {
		r := rune(text[end])
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return false
		}
	}
	return true
}
