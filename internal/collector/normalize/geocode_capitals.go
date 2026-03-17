// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

// capitalCoords maps ISO 3166-1 alpha-2 codes to capital city coordinates.
// Used instead of geographic centroids so island nations (Malta, Cyprus,
// Singapore, etc.) place pins on land rather than in the sea.
var capitalCoords = map[string][2]float64{
	"AF": {34.53, 69.17},   // Kabul
	"AL": {41.33, 19.82},   // Tirana
	"DZ": {36.75, 3.04},    // Algiers
	"AO": {-8.84, 13.23},   // Luanda
	"AR": {-34.60, -58.38}, // Buenos Aires
	"AM": {40.18, 44.51},   // Yerevan
	"AU": {-35.28, 149.13}, // Canberra
	"AT": {48.21, 16.37},   // Vienna
	"AZ": {40.41, 49.87},   // Baku
	"BH": {26.23, 50.59},   // Manama
	"BD": {23.81, 90.41},   // Dhaka
	"BY": {53.90, 27.57},   // Minsk
	"BE": {50.85, 4.35},    // Brussels
	"BJ": {6.50, 2.60},     // Porto-Novo
	"BO": {-16.50, -68.15}, // La Paz
	"BA": {43.86, 18.41},   // Sarajevo
	"BW": {-24.65, 25.91},  // Gaborone
	"BR": {-15.79, -47.88}, // Brasília
	"BG": {42.70, 23.32},   // Sofia
	"BF": {12.37, -1.52},   // Ouagadougou
	"BI": {-3.38, 29.36},   // Gitega
	"KH": {11.56, 104.92},  // Phnom Penh
	"CM": {3.87, 11.52},    // Yaoundé
	"CA": {45.42, -75.70},  // Ottawa
	"CF": {4.39, 18.56},    // Bangui
	"TD": {12.13, 15.05},   // N'Djamena
	"CL": {-33.45, -70.67}, // Santiago
	"CN": {39.90, 116.40},  // Beijing
	"CO": {4.71, -74.07},   // Bogotá
	"CD": {-4.32, 15.31},   // Kinshasa
	"CR": {9.93, -84.09},   // San José
	"HR": {45.81, 15.98},   // Zagreb
	"CU": {23.11, -82.37},  // Havana
	"CY": {35.17, 33.36},   // Nicosia
	"CZ": {50.08, 14.43},   // Prague
	"DK": {55.68, 12.57},   // Copenhagen
	"DO": {18.47, -69.90},  // Santo Domingo
	"EC": {-0.18, -78.47},  // Quito
	"EG": {30.04, 31.24},   // Cairo
	"SV": {13.69, -89.19},  // San Salvador
	"ER": {15.34, 38.93},   // Asmara
	"EE": {59.44, 24.75},   // Tallinn
	"ET": {9.02, 38.75},    // Addis Ababa
	"FI": {60.17, 24.94},   // Helsinki
	"FR": {48.86, 2.35},    // Paris
	"GA": {0.39, 9.45},     // Libreville
	"GM": {13.45, -16.58},  // Banjul
	"PS": {31.90, 35.20},   // Ramallah
	"GE": {41.72, 44.79},   // Tbilisi
	"DE": {52.52, 13.41},   // Berlin
	"GH": {5.56, -0.19},    // Accra
	"GR": {37.98, 23.73},   // Athens
	"GT": {14.63, -90.51},  // Guatemala City
	"GN": {9.64, -13.58},   // Conakry
	"HT": {18.54, -72.34},  // Port-au-Prince
	"HN": {14.07, -87.19},  // Tegucigalpa
	"HU": {47.50, 19.04},   // Budapest
	"IN": {28.61, 77.21},   // New Delhi
	"ID": {-6.21, 106.85},  // Jakarta
	"IR": {35.69, 51.39},   // Tehran
	"IQ": {33.34, 44.37},   // Baghdad
	"IE": {53.35, -6.26},   // Dublin
	"IL": {31.77, 35.22},   // Jerusalem
	"IT": {41.90, 12.50},   // Rome
	"CI": {6.83, -5.29},    // Yamoussoukro
	"JM": {18.00, -76.79},  // Kingston
	"JP": {35.68, 139.69},  // Tokyo
	"JO": {31.95, 35.93},   // Amman
	"KZ": {51.17, 71.43},   // Astana
	"KE": {-1.29, 36.82},   // Nairobi
	"XK": {42.66, 21.17},   // Pristina
	"KW": {29.37, 47.98},   // Kuwait City
	"KG": {42.87, 74.59},   // Bishkek
	"LA": {17.97, 102.63},  // Vientiane
	"LV": {56.95, 24.11},   // Riga
	"LB": {33.89, 35.50},   // Beirut
	"LY": {32.90, 13.18},   // Tripoli
	"LT": {54.69, 25.28},   // Vilnius
	"LU": {49.61, 6.13},    // Luxembourg City
	"MG": {-18.91, 47.54},  // Antananarivo
	"MW": {-13.97, 33.79},  // Lilongwe
	"MY": {3.14, 101.69},   // Kuala Lumpur
	"ML": {12.64, -8.00},   // Bamako
	"MT": {35.90, 14.51},   // Valletta
	"MR": {18.09, -15.98},  // Nouakchott
	"MX": {19.43, -99.13},  // Mexico City
	"MD": {47.01, 28.86},   // Chișinău
	"MN": {47.91, 106.91},  // Ulaanbaatar
	"ME": {42.44, 19.26},   // Podgorica
	"MA": {34.02, -6.84},   // Rabat
	"MZ": {-25.97, 32.57},  // Maputo
	"MM": {19.76, 96.07},   // Naypyidaw
	"NA": {-22.56, 17.08},  // Windhoek
	"NP": {27.72, 85.32},   // Kathmandu
	"NL": {52.37, 4.89},    // Amsterdam
	"NZ": {-41.29, 174.78}, // Wellington
	"NI": {12.11, -86.27},  // Managua
	"NE": {13.51, 2.11},    // Niamey
	"NG": {9.06, 7.49},     // Abuja
	"KP": {39.02, 125.75},  // Pyongyang
	"MK": {42.00, 21.43},   // Skopje
	"NO": {59.91, 10.75},   // Oslo
	"OM": {23.59, 58.54},   // Muscat
	"PK": {33.69, 73.04},   // Islamabad
	"PA": {8.98, -79.52},   // Panama City
	"PG": {-6.31, 147.15},  // Port Moresby
	"PY": {-25.26, -57.58}, // Asunción
	"PE": {-12.05, -77.04}, // Lima
	"PH": {14.60, 120.98},  // Manila
	"PL": {52.23, 21.01},   // Warsaw
	"PT": {38.72, -9.14},   // Lisbon
	"QA": {25.29, 51.53},   // Doha
	"RO": {44.43, 26.10},   // Bucharest
	"RU": {55.76, 37.62},   // Moscow
	"RW": {-1.94, 30.06},   // Kigali
	"SA": {24.69, 46.72},   // Riyadh
	"SN": {14.72, -17.47},  // Dakar
	"RS": {44.79, 20.47},   // Belgrade
	"SL": {8.48, -13.23},   // Freetown
	"SG": {1.29, 103.85},   // Singapore
	"SK": {48.15, 17.11},   // Bratislava
	"SI": {46.06, 14.51},   // Ljubljana
	"SO": {2.05, 45.32},    // Mogadishu
	"ZA": {-25.75, 28.19},  // Pretoria
	"KR": {37.57, 126.98},  // Seoul
	"SS": {4.85, 31.60},    // Juba
	"ES": {40.42, -3.70},   // Madrid
	"LK": {6.93, 79.85},    // Colombo
	"SD": {15.60, 32.53},   // Khartoum
	"SE": {59.33, 18.07},   // Stockholm
	"CH": {46.95, 7.45},    // Bern
	"SY": {33.51, 36.28},   // Damascus
	"TW": {25.03, 121.57},  // Taipei
	"TJ": {38.56, 68.77},   // Dushanbe
	"TZ": {-6.16, 35.75},   // Dodoma
	"TH": {13.76, 100.50},  // Bangkok
	"TG": {6.14, 1.21},     // Lomé
	"TN": {36.81, 10.17},   // Tunis
	"TR": {39.93, 32.87},   // Ankara
	"TM": {37.95, 58.38},   // Ashgabat
	"UG": {0.35, 32.58},    // Kampala
	"UA": {50.45, 30.52},   // Kyiv
	"AE": {24.45, 54.65},   // Abu Dhabi
	"GB": {51.51, -0.13},   // London
	"US": {38.90, -77.04},  // Washington D.C.
	"UY": {-34.88, -56.17}, // Montevideo
	"UZ": {41.30, 69.28},   // Tashkent
	"VE": {10.49, -66.88},  // Caracas
	"VN": {21.03, 105.85},  // Hanoi
	"YE": {15.37, 44.21},   // Sana'a
	"ZM": {-15.39, 28.32},  // Lusaka
	"ZW": {-17.83, 31.05},  // Harare
	"IS": {64.15, -21.94},  // Reykjavik
}
