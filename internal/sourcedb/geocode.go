// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package sourcedb

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CityResult is a geocoded city from the GeoNames gazetteer.
type CityResult struct {
	Name        string
	CountryCode string
	Lat         float64
	Lng         float64
	Population  int
}

// ImportGeoNames loads a GeoNames cities500.txt (tab-separated) file into
// the cities table. It replaces all existing rows. The file format is
// documented at https://download.geonames.org/export/dump/readme.txt.
//
// Columns used: 0=geonameid, 1=name, 2=asciiname, 4=latitude, 5=longitude,
// 8=country_code, 14=population.
func (db *DB) ImportGeoNames(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open geonames file: %w", err)
	}
	defer f.Close()

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin geonames import tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM cities`); err != nil {
		return fmt.Errorf("clear cities table: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO cities (id, name, name_lower, ascii_name, ascii_lower, country_code, lat, lng, population)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare cities insert: %w", err)
	}
	defer stmt.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var count int
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 15 {
			continue
		}
		geoID, _ := strconv.Atoi(fields[0])
		name := strings.TrimSpace(fields[1])
		asciiName := strings.TrimSpace(fields[2])
		lat, _ := strconv.ParseFloat(fields[4], 64)
		lng, _ := strconv.ParseFloat(fields[5], 64)
		cc := strings.TrimSpace(fields[8])
		pop, _ := strconv.Atoi(fields[14])

		if name == "" || cc == "" {
			continue
		}

		if _, err := stmt.ExecContext(ctx, geoID, name, strings.ToLower(name), asciiName, strings.ToLower(asciiName), cc, lat, lng, pop); err != nil {
			return fmt.Errorf("insert city %q: %w", name, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan geonames file: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit geonames import: %w", err)
	}
	fmt.Printf("Imported %d cities from GeoNames\n", count)
	return nil
}

// HasCities returns true if the cities table is populated.
func (db *DB) HasCities(ctx context.Context) bool {
	var count int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM cities LIMIT 1`).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// LookupCity finds the largest city matching the given name. If countryCode
// is non-empty, results for that country are strongly preferred.
func (db *DB) LookupCity(ctx context.Context, name string, countryCode string) (CityResult, bool) {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))

	if nameLower == "" {
		return CityResult{}, false
	}

	// Try country-specific match first.
	if countryCode != "" {
		var r CityResult
		err := db.sql.QueryRowContext(ctx,
			`SELECT name, country_code, lat, lng, population FROM cities
			 WHERE (name_lower = ? OR ascii_lower = ?) AND country_code = ?
			 ORDER BY population DESC LIMIT 1`,
			nameLower, nameLower, countryCode).Scan(&r.Name, &r.CountryCode, &r.Lat, &r.Lng, &r.Population)
		if err == nil {
			return r, true
		}
	}

	// Fallback: largest city worldwide with that name.
	var r CityResult
	err := db.sql.QueryRowContext(ctx,
		`SELECT name, country_code, lat, lng, population FROM cities
		 WHERE name_lower = ? OR ascii_lower = ?
		 ORDER BY population DESC LIMIT 1`,
		nameLower, nameLower).Scan(&r.Name, &r.CountryCode, &r.Lat, &r.Lng, &r.Population)
	if err == nil {
		return r, true
	}
	return CityResult{}, false
}

// LookupCities finds all cities matching the given name, ordered by
// population descending. Used for batch scanning of text.
func (db *DB) LookupCities(ctx context.Context, name string) ([]CityResult, error) {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return nil, nil
	}

	rows, err := db.sql.QueryContext(ctx,
		`SELECT name, country_code, lat, lng, population FROM cities
		 WHERE name_lower = ? OR ascii_lower = ?
		 ORDER BY population DESC LIMIT 10`,
		nameLower, nameLower)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CityResult
	for rows.Next() {
		var r CityResult
		if err := rows.Scan(&r.Name, &r.CountryCode, &r.Lat, &r.Lng, &r.Population); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
