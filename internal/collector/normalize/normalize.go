// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"crypto/sha1"
	"encoding/hex"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
)

var (
	technicalSignalPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bcve-\d{4}-\d{4,7}\b`),
		regexp.MustCompile(`(?i)\b(?:ioc|iocs|indicator(?:s)? of compromise)\b`),
		regexp.MustCompile(`(?i)\b(?:tactic|technique|ttp|mitre)\b`),
		regexp.MustCompile(`(?i)\b(?:hash|sha-?256|sha-?1|md5|yara|sigma)\b`),
		regexp.MustCompile(`(?i)\b(?:ip(?:v4|v6)?|domain|url|hostname|command and control|c2)\b`),
		regexp.MustCompile(`(?i)\b(?:vulnerability|exploit(?:ation)?|zero-?day|patch|mitigation|workaround)\b`),
	}
	incidentDisclosurePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:breach|data leak|compromis(?:e|ed)|intrusion|unauthori[sz]ed access)\b`),
		regexp.MustCompile(`(?i)\b(?:ransomware|malware|botnet|ddos|phishing|credential theft)\b`),
		regexp.MustCompile(`(?i)\b(?:attack|attacked|target(?:ed|ing)|incident response|security incident)\b`),
		regexp.MustCompile(`(?i)\b(?:arrest(?:ed)?|charged|indicted|wanted|fugitive|missing person|kidnapp(?:ed|ing)|homicide)\b`),
	}
	actionablePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:report|submit (?:a )?tip|contact|hotline|phone|email)\b`),
		regexp.MustCompile(`(?i)\b(?:apply update|upgrade|disable|block|monitor|detect|investigate)\b`),
		regexp.MustCompile(`(?i)\b(?:advisory|alert|warning|incident notice|public appeal)\b`),
	}
	narrativePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:opinion|editorial|commentary|analysis|explainer|podcast|interview)\b`),
		regexp.MustCompile(`(?i)\b(?:what we know|live updates|behind the scenes|feature story)\b`),
		regexp.MustCompile(`(?i)\b(?:market reaction|share price|investor)\b`),
	}
	generalNewsPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:announces?|launche[sd]?|conference|summit|webinar|event|awareness month)\b`),
		regexp.MustCompile(`(?i)\b(?:ceremony|speech|statement|newsletter|weekly roundup)\b`),
		regexp.MustCompile(`(?i)\b(?:partnership|memorandum|mou|initiative|campaign)\b`),
	}
	securityContextPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:cyber|cybersecurity|infosec|information security|it security)\b`),
		regexp.MustCompile(`(?i)\b(?:security posture|security controls?|threat intelligence)\b`),
		regexp.MustCompile(`(?i)\b(?:vulnerability|exploit|patch|advisory|defend|defensive)\b`),
		regexp.MustCompile(`(?i)\b(?:soc|siem|incident response|malware analysis)\b`),
	}
	assistancePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:report(?:\s+a)?(?:\s+crime)?|submit (?:a )?tip|tip[-\s]?off)\b`),
		regexp.MustCompile(`(?i)\b(?:contact (?:police|authorities|law enforcement)|hotline|helpline)\b`),
		regexp.MustCompile(`(?i)\b(?:if you have information|seeking information|appeal for help)\b`),
		regexp.MustCompile(`(?i)\b(?:missing|wanted|fugitive|amber alert)\b`),
	}
	impactSpecificityPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:affected|impact(?:ed)?|disrupt(?:ed|ion)|outage|service interruption)\b`),
		regexp.MustCompile(`(?i)\b(?:records|accounts|systems|devices|endpoints|victims|organizations)\b`),
		regexp.MustCompile(`(?i)\b(?:on\s+\d{1,2}\s+\w+\s+\d{4}|timeline|between\s+\d{1,2}:\d{2})\b`),
		regexp.MustCompile(`(?i)\b\d{2,}\s+(?:records|users|systems|devices|victims|organizations)\b`),
	}
	newsMediaDomains = []string{
		"channelnewsasia.com",
		"yna.co.kr",
		"nhk.or.jp",
		"scmp.com",
		"jamaicaobserver.com",
		"straitstimes.com",
	}
	newsMediaIDs = map[string]struct{}{
		"cna-sg-crime":     {},
		"yonhap-kr":        {},
		"nhk-jp":           {},
		"scmp-hk":          {},
		"jamaica-observer": {},
		"straitstimes-sg":  {},
	}
	blogFilterExempt = map[string]struct{}{
		"bleepingcomputer": {},
		"krebsonsecurity":  {},
		"thehackernews":    {},
		"databreaches-net": {},
		"cbc-canada":       {},
		"globalnews-ca":    {},
	}
)

type Context struct {
	Config config.Config
	Now    time.Time
}

type FeedContext struct {
	Summary  string
	Author   string
	Tags     []string
	FeedType string
}

func RSSItem(ctx Context, meta model.RegistrySource, item parse.FeedItem) *model.Alert {
	publishedAt := parseDate(item.Published)
	if publishedAt.IsZero() {
		publishedAt = ctx.Now
	}
	if !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	alert := baseAlert(ctx, meta, item.Title, item.Link, publishedAt)
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Author:   item.Author,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	alert = normalizeInformational(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Author:   item.Author,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	return &alert
}

func HTMLItem(ctx Context, meta model.RegistrySource, item parse.FeedItem) *model.Alert {
	alert := baseAlert(ctx, meta, item.Title, item.Link, ctx.Now)
	triage := score(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	alert.Triage = triage
	alert = normalizeInformational(ctx.Config, alert, FeedContext{
		Summary:  item.Summary,
		Tags:     item.Tags,
		FeedType: meta.Type,
	})
	return &alert
}

func KEVAlert(ctx Context, meta model.RegistrySource, cveID string, vulnName string, description string, dateAdded string, knownRansomware bool) *model.Alert {
	publishedAt := parseDate(dateAdded)
	if publishedAt.IsZero() || !isFresh(ctx.Config, publishedAt, ctx.Now) {
		return nil
	}
	title := cveID + ": " + firstNonEmpty(vulnName, "Known Exploited Vulnerability")
	link := meta.Source.BaseURL
	if strings.TrimSpace(cveID) != "" {
		link = "https://nvd.nist.gov/vuln/detail/" + strings.TrimSpace(cveID)
	}
	alert := baseAlert(ctx, meta, title, link, publishedAt)
	if hoursBetween(ctx.Now, publishedAt) <= 72 {
		alert.Severity = "critical"
	} else if hoursBetween(ctx.Now, publishedAt) <= 168 {
		alert.Severity = "high"
	}
	tags := []string{}
	if knownRansomware {
		tags = append(tags, "known-ransomware-campaign")
	}
	alert.Triage = score(ctx.Config, alert, FeedContext{
		Summary:  strings.TrimSpace(vulnName + " " + description),
		Tags:     tags,
		FeedType: meta.Type,
	})
	return &alert
}

func InterpolAlert(ctx Context, meta model.RegistrySource, title string, link string, countryCode string, summary string, tags []string) *model.Alert {
	if strings.TrimSpace(title) == "" {
		return nil
	}
	alert := baseAlert(ctx, meta, title, firstNonEmpty(link, meta.Source.BaseURL), ctx.Now)
	alert.Severity = "critical"
	alert.RegionTag = firstNonEmpty(countryCode, alert.RegionTag)
	if strings.TrimSpace(countryCode) != "" {
		alert.Source.CountryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	}
	alert.Triage = score(ctx.Config, alert, FeedContext{
		Summary:  summary,
		Tags:     tags,
		FeedType: meta.Type,
	})
	return &alert
}

func StaticInterpolEntry(now time.Time) model.Alert {
	return model.Alert{
		AlertID:        "interpol-hub-static",
		SourceID:       "interpol-hub",
		Source:         model.SourceMetadata{SourceID: "interpol-hub", AuthorityName: "INTERPOL Notices Hub", Country: "France", CountryCode: "FR", Region: "International", AuthorityType: "police", BaseURL: "https://www.interpol.int"},
		Title:          "INTERPOL Red & Yellow Notices - Browse Wanted & Missing Persons",
		CanonicalURL:   "https://www.interpol.int/How-we-work/Notices/View-Red-Notices",
		FirstSeen:      now.UTC().Format(time.RFC3339),
		LastSeen:       now.UTC().Format(time.RFC3339),
		Status:         "active",
		Category:       "wanted_suspect",
		Severity:       "critical",
		RegionTag:      "INT",
		Lat:            45.764,
		Lng:            4.8357,
		FreshnessHours: 1,
		Reporting: model.ReportingMetadata{
			Label: "Browse INTERPOL Notices",
			URL:   "https://www.interpol.int/How-we-work/Notices/View-Red-Notices",
			Notes: "Red Notices: wanted persons. Yellow Notices: missing persons. Browse directly.",
		},
		Triage: &model.Triage{RelevanceScore: 1, Reasoning: "Permanent INTERPOL hub link"},
	}
}

func baseAlert(ctx Context, meta model.RegistrySource, title string, link string, publishedAt time.Time) model.Alert {
	lat, lng := jitter(meta.Lat, meta.Lng, meta.Source.SourceID+":"+link)
	return model.Alert{
		AlertID:        meta.Source.SourceID + "-" + hashID(link),
		SourceID:       meta.Source.SourceID,
		Source:         meta.Source,
		Title:          strings.TrimSpace(title),
		CanonicalURL:   strings.TrimSpace(link),
		FirstSeen:      publishedAt.UTC().Format(time.RFC3339),
		LastSeen:       ctx.Now.UTC().Format(time.RFC3339),
		Status:         "active",
		Category:       meta.Category,
		Severity:       inferSeverity(title, defaultSeverity(meta.Category)),
		RegionTag:      meta.RegionTag,
		Lat:            lat,
		Lng:            lng,
		FreshnessHours: hoursBetween(ctx.Now, publishedAt),
		Reporting:      meta.Reporting,
	}
}

func score(cfg config.Config, alert model.Alert, feed FeedContext) *model.Triage {
	text := strings.ToLower(strings.Join([]string{
		alert.Title,
		feed.Summary,
		feed.Author,
		strings.Join(feed.Tags, " "),
		alert.CanonicalURL,
	}, "\n"))
	publicationType := inferPublicationType(alert, feed.FeedType)
	score := 0.5
	signals := []string{}
	add := func(delta float64, reason string) {
		score += delta
		if delta >= 0 {
			signals = append(signals, "+"+formatDelta(delta)+" "+reason)
			return
		}
		signals = append(signals, formatDelta(delta)+" "+reason)
	}

	switch publicationType {
	case "news_media":
		add(-0.16, "publication type leans general-news")
	case "cert_advisory", "structured_incident_feed":
		add(0.08, "source metadata is incident-oriented")
	case "law_enforcement":
		add(0.06, "law-enforcement source metadata")
	}

	switch alert.Category {
	case "cyber_advisory":
		add(0.09, "cyber advisory category")
	case "wanted_suspect", "missing_person":
		add(0.09, "law-enforcement incident category")
	case "humanitarian_tasking", "conflict_monitoring", "humanitarian_security":
		add(0.08, "humanitarian incident/tasking category")
	case "education_digital_capacity":
		add(0.07, "education and digital capacity category")
	case "fraud_alert":
		add(0.07, "fraud incident category")
	}

	hasTechnical := hasAny(text, technicalSignalPatterns)
	hasIncident := hasAny(text, incidentDisclosurePatterns)
	hasActionable := hasAny(text, actionablePatterns)
	hasSpecificImpact := hasAny(text, impactSpecificityPatterns)
	hasNarrative := hasAny(text, narrativePatterns)
	hasGeneral := hasAny(text, generalNewsPatterns)
	looksLikeBlog := isBlog(alert)

	if hasTechnical {
		add(0.16, "technical indicators or tactics present")
	}
	if hasIncident {
		add(0.16, "incident/crime disclosure language")
	}
	if hasActionable {
		add(0.10, "contains response/reporting actions")
	}
	if hasSpecificImpact {
		add(0.08, "specific impact/timeline/system details")
	}
	if hasNarrative {
		add(-0.18, "opinion/commentary phrasing")
	}
	if hasGeneral {
		add(-0.12, "general institutional/news language")
	}
	if looksLikeBlog {
		add(-0.10, "blog-style structure")
	}
	if !hasTechnical && !hasIncident && (hasNarrative || hasGeneral) {
		add(-0.08, "weak incident evidence relative to narrative cues")
	}
	if alert.FreshnessHours > 0 && alert.FreshnessHours <= 24 && (hasIncident || hasTechnical) {
		add(0.04, "fresh post with potential early-warning signal")
	}

	threshold := clamp01(cfg.IncidentRelevanceThreshold)
	relevance := round3(clamp01(score))
	distance := math.Abs(relevance - threshold)
	confidence := "low"
	if distance >= 0.25 {
		confidence = "high"
	} else if distance >= 0.1 {
		confidence = "medium"
	}
	disposition := "filtered_review"
	if relevance >= threshold {
		disposition = "retained"
	}
	return &model.Triage{
		RelevanceScore:  relevance,
		Threshold:       threshold,
		Confidence:      confidence,
		Disposition:     disposition,
		PublicationType: publicationType,
		WeakSignals:     limitStrings(signals, 12),
		Metadata: &model.TriageMetadata{
			Author: strings.TrimSpace(feed.Author),
			Tags:   limitStrings(feed.Tags, 8),
		},
	}
}

func normalizeInformational(cfg config.Config, alert model.Alert, feed FeedContext) model.Alert {
	if !isSecurityInformational(alert, feed) || alert.Triage == nil {
		return alert
	}
	threshold := clamp01(cfg.IncidentRelevanceThreshold)
	score := math.Max(alert.Triage.RelevanceScore, threshold)
	alert.Category = "informational"
	alert.Severity = "info"
	alert.Triage.RelevanceScore = round3(score)
	alert.Triage.Threshold = threshold
	alert.Triage.Confidence = "medium"
	alert.Triage.Disposition = "retained"
	alert.Triage.WeakSignals = append([]string{"reclassified as informational security/cybersecurity update"}, limitStrings(alert.Triage.WeakSignals, 10)...)
	return alert
}

func thresholdForAlert(cfg config.Config, alert model.Alert) float64 {
	if strings.EqualFold(alert.Category, "missing_person") {
		return clamp01(cfg.MissingPersonRelevanceThreshold)
	}
	return clamp01(cfg.IncidentRelevanceThreshold)
}

func defaultSeverity(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "informational":
		return "info"
	case "cyber_advisory":
		return "high"
	case "wanted_suspect", "missing_person":
		return "critical"
	case "public_appeal", "humanitarian_tasking", "humanitarian_security", "private_sector":
		return "high"
	default:
		return "medium"
	}
}

func inferSeverity(title string, fallback string) string {
	t := strings.ToLower(title)
	switch {
	case containsAny(t, "critical", "emergency", "zero-day", "0-day", "ransomware", "actively exploited", "exploitation", "breach", "data leak", "crypto heist", "million stolen", "wanted", "fugitive", "murder", "homicide", "missing", "amber alert", "kidnap"):
		return "critical"
	case containsAny(t, "hack", "compromise", "vulnerability", "high", "severe", "urgent", "fatal", "death", "shooting", "fraud", "scam", "phishing"):
		return "high"
	case containsAny(t, "arrested", "charged", "sentenced", "medium", "moderate"):
		return "medium"
	case containsAny(t, "low", "informational"):
		return "info"
	default:
		return fallback
	}
}

func parseDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC3339, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC850, "2006-01-02"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func isFresh(cfg config.Config, date time.Time, now time.Time) bool {
	cutoff := now.Add(-time.Duration(cfg.MaxAgeDays) * 24 * time.Hour)
	return !date.Before(cutoff)
}

func hasAny(text string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func inferPublicationType(alert model.Alert, feedType string) string {
	if isNewsMedia(alert) {
		return "news_media"
	}
	switch strings.ToLower(alert.Source.AuthorityType) {
	case "cert":
		return "cert_advisory"
	case "police":
		return "law_enforcement"
	case "intelligence", "national_security":
		return "security_bulletin"
	case "public_safety_program":
		return "public_safety_bulletin"
	}
	if feedType == "kev-json" || feedType == "interpol-red-json" || feedType == "interpol-yellow-json" {
		return "structured_incident_feed"
	}
	return "official_update"
}

func isNewsMedia(alert model.Alert) bool {
	if _, ok := newsMediaIDs[strings.ToLower(alert.SourceID)]; ok {
		return true
	}
	host := extractDomain(alert.CanonicalURL)
	for _, domain := range newsMediaDomains {
		if strings.Contains(host, domain) {
			return true
		}
	}
	return false
}

func isBlog(alert model.Alert) bool {
	if _, ok := blogFilterExempt[strings.ToLower(alert.SourceID)]; ok {
		return false
	}
	title := strings.ToLower(alert.Title)
	link := strings.ToLower(alert.CanonicalURL)
	return strings.Contains(title, "blog") || strings.Contains(link, "/blog") || strings.Contains(link, "medium.com") || strings.Contains(link, "wordpress.com")
}

func isSecurityInformational(alert model.Alert, feed FeedContext) bool {
	text := strings.ToLower(strings.Join([]string{
		alert.Title,
		feed.Summary,
		feed.Author,
		strings.Join(feed.Tags, " "),
		alert.CanonicalURL,
	}, "\n"))
	publicationType := inferPublicationType(alert, feed.FeedType)
	authorityType := strings.ToLower(alert.Source.AuthorityType)
	sourceIsSecurityRelevant := alert.Category == "cyber_advisory" ||
		alert.Category == "private_sector" ||
		publicationType == "cert_advisory" ||
		authorityType == "cert" ||
		authorityType == "private_sector" ||
		authorityType == "regulatory"
	return sourceIsSecurityRelevant &&
		hasAny(text, securityContextPatterns) &&
		!hasAny(text, incidentDisclosurePatterns) &&
		!hasAny(text, assistancePatterns) &&
		!hasAny(text, impactSpecificityPatterns) &&
		(hasAny(text, generalNewsPatterns) || hasAny(text, narrativePatterns) || publicationType == "news_media")
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func hashID(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func jitter(lat float64, lng float64, seed string) (float64, float64) {
	sum := sha1.Sum([]byte(seed))
	angle := float64(sum[0])/255*math.Pi*2 + float64(sum[1])/255
	radius := 22 + float64(sum[2])/255*55
	dLat := (radius / 111.32) * math.Cos(angle)
	cosLat := math.Max(0.2, math.Cos((lat*math.Pi)/180))
	dLng := (radius / (111.32 * cosLat)) * math.Sin(angle)
	outLat := math.Max(-89.5, math.Min(89.5, lat+dLat))
	outLng := lng + dLng
	if outLng > 180 {
		outLng -= 360
	}
	if outLng < -180 {
		outLng += 360
	}
	return round5(outLat), round5(outLng)
}

func extractDomain(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func hoursBetween(now time.Time, publishedAt time.Time) int {
	if publishedAt.IsZero() {
		return 1
	}
	hours := int(math.Round(now.Sub(publishedAt).Hours()))
	if hours < 1 {
		return 1
	}
	return hours
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func round3(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func round5(value float64) float64 {
	return math.Round(value*100000) / 100000
}

func formatDelta(value float64) string {
	return strconvf(value, 2)
}

func strconvf(value float64, places int) string {
	format := math.Pow(10, float64(places))
	value = math.Round(value*format) / format
	return strings.TrimRight(strings.TrimRight(fmtFloat(value), "0"), ".")
}

func fmtFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func limitStrings(values []string, limit int) []string {
	out := make([]string, 0, limit)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
		if len(out) == limit {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func Deduplicate(alerts []model.Alert) ([]model.Alert, model.DuplicateAudit) {
	byKey := make(map[string]model.Alert, len(alerts))
	for _, alert := range alerts {
		key := strings.ToLower(alert.CanonicalURL + "|" + alert.Title)
		current, ok := byKey[key]
		if !ok || alertScore(alert) > alertScore(current) {
			byKey[key] = alert
		}
	}
	deduped := make([]model.Alert, 0, len(byKey))
	for _, alert := range byKey {
		deduped = append(deduped, alert)
	}
	sort.Slice(deduped, func(i, j int) bool { return deduped[i].Title < deduped[j].Title })
	kept, suppressed := collapseVariants(deduped)
	duplicates := summarizeTitleDuplicates(kept)
	return kept, model.DuplicateAudit{
		SuppressedVariantDuplicates: len(suppressed),
		RepeatedTitleGroupsInActive: len(duplicates),
		RepeatedTitleSamples:        duplicates,
	}
}

func FilterActive(cfg config.Config, alerts []model.Alert) (active []model.Alert, filtered []model.Alert) {
	for _, alert := range alerts {
		threshold := thresholdForAlert(cfg, alert)
		score := 0.0
		if alert.Triage != nil {
			score = alert.Triage.RelevanceScore
		}
		if score >= threshold {
			active = append(active, alert)
			continue
		}
		filtered = append(filtered, alert)
	}
	sortAlerts(active, true)
	sortAlerts(filtered, false)
	return active, filtered
}

func sortAlerts(alerts []model.Alert, active bool) {
	sort.Slice(alerts, func(i, j int) bool {
		if !active {
			scoreDelta := alertScore(alerts[j]) - alertScore(alerts[i])
			if scoreDelta != 0 {
				return scoreDelta > 0
			}
		}
		return alerts[i].FirstSeen > alerts[j].FirstSeen
	})
}

func alertScore(alert model.Alert) float64 {
	if alert.Triage == nil {
		return -1
	}
	return alert.Triage.RelevanceScore
}

func collapseVariants(alerts []model.Alert) ([]model.Alert, []model.Alert) {
	byVariant := make(map[string][]model.Alert)
	passthrough := make([]model.Alert, 0, len(alerts))
	for _, alert := range alerts {
		key := buildVariantKey(alert)
		if key == "" {
			passthrough = append(passthrough, alert)
			continue
		}
		byVariant[key] = append(byVariant[key], alert)
	}
	kept := append([]model.Alert{}, passthrough...)
	suppressed := []model.Alert{}
	for _, group := range byVariant {
		if len(group) == 1 {
			kept = append(kept, group[0])
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			return comparePreference(group[i], group[j]) < 0
		})
		kept = append(kept, group[0])
		suppressed = append(suppressed, group[1:]...)
	}
	return kept, suppressed
}

func buildVariantKey(alert model.Alert) string {
	titleNorm := normalizeHeadline(alert.Title)
	if len(titleNorm) < 24 {
		return ""
	}
	u, err := url.Parse(alert.CanonicalURL)
	if err != nil {
		return ""
	}
	path := strings.TrimRight(u.Path, "/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 {
		return ""
	}
	leaf := segments[len(segments)-1]
	re := regexp.MustCompile(`-\d+$`)
	if !re.MatchString(leaf) {
		return ""
	}
	segments[len(segments)-1] = re.ReplaceAllString(leaf, "")
	return strings.ToLower(alert.SourceID + "|" + strings.TrimPrefix(u.Hostname(), "www.") + "/" + strings.Join(segments, "/") + "|" + titleNorm)
}

func comparePreference(a model.Alert, b model.Alert) int {
	if alertScore(a) != alertScore(b) {
		if alertScore(a) > alertScore(b) {
			return -1
		}
		return 1
	}
	if a.FirstSeen != b.FirstSeen {
		if a.FirstSeen > b.FirstSeen {
			return -1
		}
		return 1
	}
	if len(a.CanonicalURL) < len(b.CanonicalURL) {
		return -1
	}
	if len(a.CanonicalURL) > len(b.CanonicalURL) {
		return 1
	}
	return 0
}

func summarizeTitleDuplicates(alerts []model.Alert) []model.DuplicateSample {
	counts := map[string]int{}
	for _, alert := range alerts {
		key := normalizeHeadline(alert.Title)
		if key == "" {
			continue
		}
		counts[key]++
	}
	out := []model.DuplicateSample{}
	for title, count := range counts {
		if count > 1 {
			out = append(out, model.DuplicateSample{Title: title, Count: count})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}

func normalizeHeadline(value string) string {
	value = strings.ToLower(value)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	return strings.TrimSpace(re.ReplaceAllString(value, " "))
}
